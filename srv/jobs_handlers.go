package srv

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"srv.exe.dev/srv/jobs"
)

// siteURL is the base for admin links in emails. Admin pages rely on exe.dev
// login, which only works reliably on the *.exe.xyz proxy host (the custom
// domain via www. redirect-loops on /__exe.dev/auth), so default there.
// Override with ADMIN_URL.
func (s *Server) siteURL() string {
	if u := strings.TrimRight(os.Getenv("ADMIN_URL"), "/"); u != "" {
		return u
	}
	return "https://kohlschwarz.exe.xyz"
}

type jobsPage struct {
	Hostname   string
	Rows       []jobs.Row
	Runs       []jobs.Run
	Cost       jobs.Cost
	CostLine   string
	Budget     string
	MinScore   int
	Sources    int
	Unranked   int
	Msg        string
	ShowHidden bool
	Model      string
	LastFetch  *jobs.Run
	LastEmail  *jobs.Run
	Activity   jobs.ActivityState
	Updated    string // last fetch finished, e.g. "2026-09-03 04:00 UTC"
	UpdatedAgo string
	Owner      bool // false for allowlisted viewers (read-only)
	Viewers    int
}

func runAgo(v any) string {
	var r *jobs.Run
	switch x := v.(type) {
	case *jobs.Run:
		r = x
	case jobs.Run:
		r = &x
	}
	if r == nil {
		return ""
	}
	if r.Finished != nil {
		return jobs.Ago(*r.Finished)
	}
	return jobs.Ago(r.Started)
}

func (s *Server) HandleAdminJobs(w http.ResponseWriter, r *http.Request) {
	ok, owner := s.requireViewer(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	showHidden := r.URL.Query().Get("hidden") == "1"
	rows, err := jobs.List(ctx, s.DB, showHidden, 300)
	if err != nil {
		slog.Warn("jobs list", "error", err)
	}
	runs, _ := jobs.Runs(ctx, s.DB, 12)
	unranked := 0
	for _, x := range rows {
		if x.Score == nil {
			unranked++
		}
	}
	rows = jobs.Dedupe(rows)
	cost := jobs.GetCost(ctx, s.DB)
	lf := jobs.LastRun(ctx, s.DB, "fetch")
	data := jobsPage{
		Hostname: s.Hostname, Rows: rows, Runs: runs, Cost: cost, CostLine: cost.CostLine(),
		Budget: "$" + strconv.FormatFloat(jobs.MaxMonthUSD(), 'f', 2, 64), MinScore: jobs.ReportMinScore,
		Sources: len(jobs.Sources), Unranked: unranked, Msg: r.URL.Query().Get("msg"), ShowHidden: showHidden, Model: jobs.Model,
		LastFetch: lf, LastEmail: jobs.LastRun(ctx, s.DB, "email"),
		Activity: jobs.Current.State(), UpdatedAgo: runAgo(lf),
		Owner: owner, Viewers: len(s.viewers(ctx)),
	}
	if lf != nil {
		ts := lf.Started
		if lf.Finished != nil {
			ts = *lf.Finished
		}
		if len(ts) >= 16 {
			ts = ts[:16]
		}
		data.Updated = ts + " UTC"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "jobs.html", data); err != nil {
		slog.Warn("render jobs", "error", err)
	}
}

func (s *Server) HandleAdminJobsReport(w http.ResponseWriter, r *http.Request) {
	if ok, _ := s.requireViewer(w, r); !ok {
		return
	}
	text, _ := jobs.BuildReport(r.Context(), s.DB, s.siteURL())
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(text))
}

// HandleAdminJobsStatus reports the running background job as JSON (polled by radar.js).
func (s *Server) HandleAdminJobsStatus(w http.ResponseWriter, r *http.Request) {
	if ok, _ := s.requireViewer(w, r); !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(jobs.Current.State())
}

// startBackground kicks off fn under the single activity slot and redirects back
// to the list, which shows a live "updating" indicator until fn finishes.
func (s *Server) startBackground(w http.ResponseWriter, r *http.Request, kind string, timeout time.Duration, fn func(ctx context.Context) string) {
	if !jobs.Current.Start(kind) {
		st := jobs.Current.State()
		http.Redirect(w, r, "/admin/jobs?msg="+url.QueryEscape("busy: "+st.Running+" already running"), http.StatusSeeOther)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		defer func() {
			if p := recover(); p != nil {
				slog.Error("jobs background panic", "kind", kind, "panic", p)
				jobs.Current.Finish(kind + " crashed")
			}
		}()
		jobs.Current.Finish(fn(ctx))
	}()
	http.Redirect(w, r, "/admin/jobs", http.StatusSeeOther)
}

func (s *Server) HandleAdminJobsFetch(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	s.startBackground(w, r, "fetch", 20*time.Minute, func(ctx context.Context) string {
		run := jobs.FetchAll(ctx, s.DB)
		jobs.Current.Switch("rank")
		rk := jobs.RankPending(ctx, s.DB, 200)
		jobs.Current.Switch("brief")
		br := jobs.BriefPending(ctx, s.DB, 60)
		msg := fmt.Sprintf("%d new, %d ranked, %d briefed", run.NewCount, rk.Ranked, br.Ranked)
		if run.SourcesErr > 0 {
			msg += fmt.Sprintf(", %d sources failed", run.SourcesErr)
		}
		return msg
	})
}

func (s *Server) HandleAdminJobsRank(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	s.startBackground(w, r, "rank", 10*time.Minute, func(ctx context.Context) string {
		run := jobs.RankPending(ctx, s.DB, 200)
		jobs.Current.Switch("brief")
		br := jobs.BriefPending(ctx, s.DB, 60)
		return fmt.Sprintf("ranked %d, briefed %d, cost $%.4f", run.Ranked, br.Ranked, run.CostUSD+br.CostUSD)
	})
}

func (s *Server) HandleAdminJobsEmail(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	s.startBackground(w, r, "email", 2*time.Minute, func(ctx context.Context) string {
		if _, err := jobs.WeeklyReport(ctx, s.DB, adminEmail(), s.siteURL(), true); err != nil {
			return "email failed: " + err.Error()
		}
		return "email sent to " + adminEmail()
	})
}

// HandleAdminJobsVoucher applies a one-time voucher that resets this month's budget usage to $0.
func (s *Server) HandleAdminJobsVoucher(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	amt, err := jobs.ApplyVoucher(r.Context(), s.DB)
	msg := fmt.Sprintf("voucher applied: %s credited, budget usage reset to $0", fmt.Sprintf("$%.3f", amt))
	if err != nil {
		msg = "voucher failed: " + err.Error()
	}
	http.Redirect(w, r, "/admin/jobs?msg="+url.QueryEscape(msg), http.StatusSeeOther)
}

func (s *Server) HandleAdminJobsHide(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	jobs.SetHidden(r.Context(), s.DB, id, r.FormValue("unhide") == "")
	http.Redirect(w, r, "/admin/jobs", http.StatusSeeOther)
}

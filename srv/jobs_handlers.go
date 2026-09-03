package srv

import (
	"context"
	"log/slog"
	"net/http"
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
}

func (s *Server) HandleAdminJobs(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
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
	data := jobsPage{
		Hostname: s.Hostname, Rows: rows, Runs: runs, Cost: cost, CostLine: cost.CostLine(),
		Budget: "$" + strconv.FormatFloat(jobs.MaxMonthUSD(), 'f', 2, 64), MinScore: jobs.ReportMinScore,
		Sources: len(jobs.Sources), Unranked: unranked, Msg: r.URL.Query().Get("msg"), ShowHidden: showHidden, Model: jobs.Model,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "jobs.html", data); err != nil {
		slog.Warn("render jobs", "error", err)
	}
}

func (s *Server) HandleAdminJobsReport(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	text, _ := jobs.BuildReport(r.Context(), s.DB, s.siteURL())
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(text))
}

func (s *Server) HandleAdminJobsFetch(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	run := jobs.FetchAll(ctx, s.DB)
	http.Redirect(w, r, "/admin/jobs?msg="+strconv.Quote(strings.TrimSpace("fetched: "+strconv.FormatInt(run.Found, 10)+" scanned, "+strconv.FormatInt(run.Matched, 10)+" matched, "+strconv.FormatInt(run.NewCount, 10)+" new, "+strconv.FormatInt(run.SourcesErr, 10)+" sources failed")), http.StatusSeeOther)
}

func (s *Server) HandleAdminJobsRank(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	run := jobs.RankPending(ctx, s.DB, 200)
	http.Redirect(w, r, "/admin/jobs?msg="+strconv.Quote("ranked "+strconv.FormatInt(run.Ranked, 10)+" postings, cost $"+strconv.FormatFloat(run.CostUSD, 'f', 4, 64)), http.StatusSeeOther)
}

func (s *Server) HandleAdminJobsEmail(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	_, err := jobs.WeeklyReport(ctx, s.DB, adminEmail(), s.siteURL(), true)
	msg := "email sent to " + adminEmail()
	if err != nil {
		msg = "email failed: " + err.Error()
	}
	http.Redirect(w, r, "/admin/jobs?msg="+strconv.Quote(msg), http.StatusSeeOther)
}

func (s *Server) HandleAdminJobsHide(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	jobs.SetHidden(r.Context(), s.DB, id, r.FormValue("unhide") == "")
	http.Redirect(w, r, "/admin/jobs", http.StatusSeeOther)
}

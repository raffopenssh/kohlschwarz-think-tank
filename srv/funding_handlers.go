package srv

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"srv.exe.dev/srv/feedback"
	"srv.exe.dev/srv/funding"
)

type fundingPage struct {
	Rows     []funding.Entry
	Upcoming []funding.Entry
	Msg      string
	Today    string
	Statuses []string
	Owner    bool // false for allowlisted viewers (read-only)
	Viewers  int
	Reasons  []feedback.Reason
	Feedback feedbackPanel
}

func (s *Server) HandleAdminFunding(w http.ResponseWriter, r *http.Request) {
	ok, owner := s.requireViewer(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	rows, err := funding.List(ctx, s.DB)
	if err != nil {
		slog.Warn("funding list", "error", err)
	}
	up, _ := funding.Upcoming(ctx, s.DB, 60, 40)
	data := fundingPage{Rows: rows, Upcoming: up, Msg: r.URL.Query().Get("msg"), Today: time.Now().UTC().Format("2006-01-02"), Statuses: []string{"open", "applied", "rejected", "won", "skip"}, Owner: owner, Viewers: len(s.viewers(ctx)), Reasons: feedback.Reasons, Feedback: s.feedbackPanel(ctx, "grant")}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "funding.html", data); err != nil {
		slog.Warn("render funding", "error", err)
	}
}

func (s *Server) HandleAdminFundingReseed(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	n, err := funding.Seed(r.Context(), s.DB)
	msg := "reseeded " + strconv.Itoa(n) + " entries"
	if err != nil {
		msg = "reseed error: " + err.Error()
	}
	http.Redirect(w, r, "/admin/funding?msg="+msg, http.StatusSeeOther)
}

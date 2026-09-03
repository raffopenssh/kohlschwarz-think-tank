package srv

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"srv.exe.dev/srv/funding"
)

type fundingPage struct {
	Rows     []funding.Entry
	Upcoming []funding.Entry
	Msg      string
	Today    string
	Statuses []string
}

func (s *Server) HandleAdminFunding(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	ctx := r.Context()
	rows, err := funding.List(ctx, s.DB)
	if err != nil {
		slog.Warn("funding list", "error", err)
	}
	up, _ := funding.Upcoming(ctx, s.DB, 60, 40)
	data := fundingPage{Rows: rows, Upcoming: up, Msg: r.URL.Query().Get("msg"), Today: time.Now().UTC().Format("2006-01-02"), Statuses: []string{"open", "applied", "rejected", "won", "skip"}}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "funding.html", data); err != nil {
		slog.Warn("render funding", "error", err)
	}
}

func (s *Server) HandleAdminFundingStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := funding.SetStatus(r.Context(), s.DB, id, r.FormValue("status"), r.FormValue("note")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/funding#f"+strconv.FormatInt(id, 10), http.StatusSeeOther)
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

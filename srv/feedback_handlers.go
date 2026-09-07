package srv

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"srv.exe.dev/srv/feedback"
	"srv.exe.dev/srv/funding"
	"srv.exe.dev/srv/jobs"
)

// wantsJSON: radar.js posts with fetch + Accept: application/json; plain forms
// (no-JS fallback) get a redirect.
func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

func (s *Server) feedbackDone(w http.ResponseWriter, r *http.Request, back string, payload map[string]any) {
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if payload == nil {
			payload = map[string]any{}
		}
		payload["ok"] = true
		json.NewEncoder(w).Encode(payload)
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

func pathID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

func voteVal(s string) int {
	switch s {
	case "1", "up":
		return 1
	case "-1", "down":
		return -1
	}
	return 0
}

func voteAction(v int) string {
	switch v {
	case 1:
		return "up"
	case -1:
		return "down"
	}
	return "clear"
}

// ---- jobs ----

// HandleAdminJobsHide trashes (or restores) a posting. Form: unhide=1, reason=<key>, detail=<text>.
// Reply tells the client whether to ask for a reason (only once per posting).
func (s *Server) HandleAdminJobsHide(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	ctx := r.Context()
	id := pathID(r)
	row, _ := jobs.Get(ctx, s.DB, id)
	if row == nil {
		http.NotFound(w, r)
		return
	}
	reason := r.FormValue("reason")
	if !feedback.ValidReason(reason) {
		reason = ""
	}
	detail := strings.TrimSpace(r.FormValue("detail"))
	unhide := r.FormValue("unhide") != ""
	if reason != "" && row.Hidden && !unhide {
		// second step: reason for an already-trashed item
		jobs.SetHidden(ctx, s.DB, id, true, reason)
		feedback.Log(ctx, s.DB, "job", id, "reason", reason, detail, row.Title, row.Org)
		s.feedbackDone(w, r, "/admin/jobs", nil)
		return
	}
	jobs.SetHidden(ctx, s.DB, id, !unhide, reason)
	action := "trash"
	if unhide {
		action = "restore"
	}
	feedback.Log(ctx, s.DB, "job", id, action, reason, detail, row.Title, row.Org)
	s.feedbackDone(w, r, "/admin/jobs", map[string]any{"hidden": !unhide, "ask_reason": !unhide && reason == "" && row.TrashReason == ""})
}

func (s *Server) HandleAdminJobsVote(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	ctx := r.Context()
	id := pathID(r)
	row, _ := jobs.Get(ctx, s.DB, id)
	if row == nil {
		http.NotFound(w, r)
		return
	}
	v := voteVal(r.FormValue("vote"))
	if v == row.Vote { // toggle off
		v = 0
	}
	jobs.SetVote(ctx, s.DB, id, v)
	feedback.Log(ctx, s.DB, "job", id, voteAction(v), "", strings.TrimSpace(r.FormValue("detail")), row.Title, row.Org)
	s.feedbackDone(w, r, "/admin/jobs", map[string]any{"vote": v})
}

func (s *Server) HandleAdminJobsNote(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	ctx := r.Context()
	id := pathID(r)
	row, _ := jobs.Get(ctx, s.DB, id)
	if row == nil {
		http.NotFound(w, r)
		return
	}
	note := strings.TrimSpace(r.FormValue("note"))
	if note != row.UserNote {
		jobs.SetNote(ctx, s.DB, id, note)
		feedback.Log(ctx, s.DB, "job", id, "note", "", note, row.Title, row.Org)
	}
	s.feedbackDone(w, r, "/admin/jobs", map[string]any{"note": note})
}

// ---- funding ----

// HandleAdminFundingStatus changes status (and optionally records why it was skipped/rejected).
// Legacy forms may also send note=; it is stored too.
func (s *Server) HandleAdminFundingStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	ctx := r.Context()
	id := pathID(r)
	e, _ := funding.Get(ctx, s.DB, id)
	if e == nil {
		http.NotFound(w, r)
		return
	}
	back := "/admin/funding#f" + strconv.FormatInt(id, 10)
	status := r.FormValue("status")
	if status == "" {
		status = e.Status
	}
	if !funding.ValidStatus(status) {
		http.Error(w, "bad status", http.StatusBadRequest)
		return
	}
	reason := r.FormValue("reason")
	if !feedback.ValidReason(reason) {
		reason = ""
	}
	detail := strings.TrimSpace(r.FormValue("detail"))
	if err := funding.SetStatus(ctx, s.DB, id, status, reason); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if n, has := r.Form["note"]; has && len(n) > 0 && strings.TrimSpace(n[0]) != e.UserNote {
		funding.SetNote(ctx, s.DB, id, n[0])
		feedback.Log(ctx, s.DB, "grant", id, "note", "", strings.TrimSpace(n[0]), e.Name, e.Track)
	}
	trashed := status == "skip" || status == "rejected"
	switch {
	case reason != "" && status == e.Status:
		feedback.Log(ctx, s.DB, "grant", id, "reason", reason, detail, e.Name, e.Track)
	case trashed && status != e.Status:
		feedback.Log(ctx, s.DB, "grant", id, "trash", reason, status, e.Name, e.Track)
	case status != e.Status:
		feedback.Log(ctx, s.DB, "grant", id, "status", "", status, e.Name, e.Track)
	}
	s.feedbackDone(w, r, back, map[string]any{"status": status, "ask_reason": trashed && status != e.Status && reason == "" && e.TrashReason == ""})
}

func (s *Server) HandleAdminFundingVote(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	ctx := r.Context()
	id := pathID(r)
	e, _ := funding.Get(ctx, s.DB, id)
	if e == nil {
		http.NotFound(w, r)
		return
	}
	v := voteVal(r.FormValue("vote"))
	if v == e.Vote {
		v = 0
	}
	funding.SetVote(ctx, s.DB, id, v)
	feedback.Log(ctx, s.DB, "grant", id, voteAction(v), "", "", e.Name, e.Track)
	s.feedbackDone(w, r, "/admin/funding#f"+strconv.FormatInt(id, 10), map[string]any{"vote": v})
}

func (s *Server) HandleAdminFundingNote(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	ctx := r.Context()
	id := pathID(r)
	e, _ := funding.Get(ctx, s.DB, id)
	if e == nil {
		http.NotFound(w, r)
		return
	}
	note := strings.TrimSpace(r.FormValue("note"))
	if note != e.UserNote {
		funding.SetNote(ctx, s.DB, id, note)
		feedback.Log(ctx, s.DB, "grant", id, "note", "", note, e.Name, e.Track)
	}
	s.feedbackDone(w, r, "/admin/funding#f"+strconv.FormatInt(id, 10), map[string]any{"note": note})
}

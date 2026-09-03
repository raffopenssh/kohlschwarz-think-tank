package srv

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"

	"srv.exe.dev/srv/jobs"
)

// Settings keys.
const settingViewers = "viewer_emails" // newline-separated exe.dev emails allowed to see the radars

func (s *Server) getSetting(ctx context.Context, key string) string {
	var v string
	err := s.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if err != nil && err != sql.ErrNoRows {
		slog.Warn("settings get", "key", key, "error", err)
	}
	return v
}

func (s *Server) setSetting(ctx context.Context, key, value string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES(?,?,strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, key, value)
	return err
}

// viewers returns the allowlisted viewer emails (lowercase, deduped, sorted by insertion).
func (s *Server) viewers(ctx context.Context) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range strings.Split(s.getSetting(ctx, settingViewers), "\n") {
		l = strings.ToLower(strings.TrimSpace(l))
		if l == "" || seen[l] || l == adminEmail() {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	return out
}

// isViewer is true for the owner or any allowlisted exe.dev account.
func (s *Server) isViewer(r *http.Request) bool {
	if s.isAdmin(r) {
		return true
	}
	e := strings.ToLower(strings.TrimSpace(r.Header.Get("X-ExeDev-Email")))
	if e == "" {
		return false
	}
	for _, v := range s.viewers(r.Context()) {
		if v == e {
			return true
		}
	}
	return false
}

// requireViewer gates read access to the radars: owner (incl. basic-auth
// fallback) or an allowlisted exe.dev login. Returns (ok, isOwner).
func (s *Server) requireViewer(w http.ResponseWriter, r *http.Request) (bool, bool) {
	if s.isAdmin(r) {
		return true, true
	}
	if s.isViewer(r) {
		return true, false
	}
	return s.requireAuth(w, r), true
}

type configPage struct {
	Viewers    []string
	Msg        string
	Err        string
	AdminEmail string
	AdminURL   string
	Budget     string
	Model      string
	Password   bool
}

func (s *Server) HandleAdminConfig(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	data := configPage{
		Viewers: s.viewers(r.Context()), Msg: r.URL.Query().Get("msg"), Err: r.URL.Query().Get("err"),
		AdminEmail: adminEmail(), AdminURL: s.siteURL(),
		Budget: "$" + strconv.FormatFloat(jobs.MaxMonthUSD(), 'f', 2, 64), Model: jobs.Model,
		Password: os.Getenv("ADMIN_PASSWORD") != "",
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "config.html", data); err != nil {
		slog.Warn("render config", "error", err)
	}
}

// HandleAdminConfigViewers adds (email=) or removes (remove=) a viewer.
func (s *Server) HandleAdminConfigViewers(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	ctx := r.Context()
	cur := s.viewers(ctx)
	var msg, errMsg string
	if rm := strings.ToLower(strings.TrimSpace(r.FormValue("remove"))); rm != "" {
		var next []string
		for _, v := range cur {
			if v != rm {
				next = append(next, v)
			}
		}
		cur, msg = next, "removed "+rm
	} else if add := strings.ToLower(strings.TrimSpace(r.FormValue("email"))); add != "" {
		a, err := mail.ParseAddress(add)
		switch {
		case err != nil || a.Address != add:
			errMsg = "not a valid email: " + add
		case add == adminEmail():
			errMsg = "that's you — the owner always has access"
		default:
			dup := false
			for _, v := range cur {
				dup = dup || v == add
			}
			if dup {
				errMsg = add + " is already on the list"
			} else {
				cur, msg = append(cur, add), "added "+add
			}
		}
	}
	if errMsg == "" {
		if err := s.setSetting(ctx, settingViewers, strings.Join(cur, "\n")); err != nil {
			errMsg = "save failed: " + err.Error()
		}
	}
	q := url.Values{}
	if errMsg != "" {
		q.Set("err", errMsg)
	} else if msg != "" {
		q.Set("msg", msg)
	}
	http.Redirect(w, r, "/admin/config?"+q.Encode(), http.StatusSeeOther)
}

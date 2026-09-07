// Package feedback stores the owner's explicit reactions to radar items
// (thumbs up/down, notes, trash reasons) so the ranker can learn from them.
package feedback

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// Reasons offered when an item is trashed. Keys are stored, labels shown.
type Reason struct{ Key, Label string }

var Reasons = []Reason{
	{"role", "wrong role"},
	{"level", "too junior"},
	{"place", "wrong place"},
	{"org", "wrong org"},
	{"terms", "pay / terms"},
	{"eligible", "not eligible"},
	{"expired", "expired"},
	{"dupe", "duplicate"},
	{"other", "other"},
}

func ValidReason(k string) bool {
	for _, r := range Reasons {
		if r.Key == k {
			return true
		}
	}
	return false
}

type Item struct {
	ID     int64
	At     string
	Radar  string // job | grant
	ItemID int64
	Action string // up | down | clear | trash | restore | note | status
	Reason string
	Detail string
	Title  string
	Org    string
}

// Log appends one feedback event.
func Log(ctx context.Context, db *sql.DB, radar string, itemID int64, action, reason, detail, title, org string) {
	if len(detail) > 500 {
		detail = detail[:500]
	}
	db.ExecContext(ctx, `INSERT INTO feedback_log (radar, item_id, action, reason, detail, title, org) VALUES (?,?,?,?,?,?,?)`,
		radar, itemID, action, reason, detail, title, org)
}

// Recent returns the newest n events (all radars when radar == "").
func Recent(ctx context.Context, db *sql.DB, radar string, n int) ([]Item, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, at, radar, item_id, action, reason, detail, title, org FROM feedback_log
		WHERE (? = '' OR radar = ?) ORDER BY id DESC LIMIT ?`, radar, radar, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.At, &it.Radar, &it.ItemID, &it.Action, &it.Reason, &it.Detail, &it.Title, &it.Org); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

type Count struct {
	Key, Label string
	N          int
}

// ReasonCounts tallies trash reasons for a radar, most frequent first.
func ReasonCounts(ctx context.Context, db *sql.DB, radar string) []Count {
	rows, err := db.QueryContext(ctx, `SELECT reason, COUNT(*) FROM feedback_log WHERE radar = ? AND reason <> '' GROUP BY reason`, radar)
	if err != nil {
		return nil
	}
	defer rows.Close()
	m := map[string]int{}
	for rows.Next() {
		var k string
		var n int
		rows.Scan(&k, &n)
		m[k] = n
	}
	var out []Count
	for _, r := range Reasons {
		if m[r.Key] > 0 {
			out = append(out, Count{r.Key, r.Label, m[r.Key]})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].N > out[j].N })
	return out
}

// PromptHints renders the owner's latest job verdicts as plain text for the
// ranking prompt: what they trashed (with reason), down- and up-voted. Empty
// when there is no feedback yet. Bounded to keep the prompt cheap.
func PromptHints(ctx context.Context, db *sql.DB, radar string, max int) string {
	rows, err := db.QueryContext(ctx, `SELECT action, reason, detail, title, org FROM feedback_log
		WHERE radar = ? AND action IN ('up','down','trash') ORDER BY id DESC LIMIT ?`, radar, max)
	if err != nil {
		return ""
	}
	defer rows.Close()
	var bad, good []string
	seen := map[string]bool{}
	for rows.Next() {
		var action, reason, detail, title, org string
		rows.Scan(&action, &reason, &detail, &title, &org)
		k := strings.ToLower(title + "|" + org)
		if seen[k] || title == "" {
			continue
		}
		seen[k] = true
		line := title
		if org != "" {
			line += " (" + org + ")"
		}
		switch action {
		case "up":
			good = append(good, line)
		default:
			if reason != "" {
				line += " — " + reasonLabel(reason)
			}
			if detail != "" {
				line += ": " + detail
			}
			bad = append(bad, line)
		}
	}
	if len(bad)+len(good) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\nOWNER FEEDBACK (learn from it; the same pattern should score lower/higher next time):\n")
	if len(bad) > 0 {
		sb.WriteString("Rejected:\n")
		for _, l := range bad {
			fmt.Fprintf(&sb, "- %s\n", l)
		}
	}
	if len(good) > 0 {
		sb.WriteString("Liked:\n")
		for _, l := range good {
			fmt.Fprintf(&sb, "+ %s\n", l)
		}
	}
	return sb.String()
}

func reasonLabel(k string) string {
	for _, r := range Reasons {
		if r.Key == k {
			return r.Label
		}
	}
	return k
}

// Totals counts up/down votes currently set and trashed items for a radar.
func Totals(ctx context.Context, db *sql.DB, radar string) (up, down, trashed int) {
	switch radar {
	case "job":
		db.QueryRowContext(ctx, `SELECT COALESCE(SUM(vote=1),0), COALESCE(SUM(vote=-1),0), COALESCE(SUM(hidden=1),0) FROM job_postings`).Scan(&up, &down, &trashed)
	case "grant":
		db.QueryRowContext(ctx, `SELECT COALESCE(SUM(vote=1),0), COALESCE(SUM(vote=-1),0), COALESCE(SUM(status IN ('skip','rejected')),0) FROM funding`).Scan(&up, &down, &trashed)
	}
	return
}

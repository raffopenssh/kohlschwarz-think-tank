// Package funding is a hand-curated radar of grants, prizes, accelerators and
// investors for Veridical Earth ("Palantir for land use") and the kohlschwarz.at
// apps. Entries are seeded from code; suitability is scored once by hand (no LLM,
// zero running cost). Status/notes are editable in /admin/funding and deadlines
// within the next weeks are appended to the weekly radar email.
package funding

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Entry struct {
	ID           int64
	Key          string
	Name         string
	URL          string
	Kind         string
	Track        string
	Amount       string
	Deadline     string
	DeadlineNote string
	Eligibility  string
	Note         string
	Score        int
	Why          string
	Status       string
	UserNote     string
}

// DaysLeft returns days until deadline, or -1 when unknown.
func (e Entry) DaysLeft() int {
	if e.Deadline == "" {
		return -1
	}
	t, err := time.Parse("2006-01-02", e.Deadline)
	if err != nil {
		return -1
	}
	return int(time.Until(t).Hours()/24) + 1
}

func (e Entry) Expired() bool { d := e.DaysLeft(); return e.Deadline != "" && d < 0 }

// DL renders the deadline column.
func (e Entry) DL() string {
	switch {
	case e.Deadline != "" && e.DeadlineNote != "":
		return e.Deadline + " (" + e.DeadlineNote + ")"
	case e.Deadline != "":
		return e.Deadline
	case e.DeadlineNote != "":
		return e.DeadlineNote
	}
	return "—"
}

const cols = `id, key, name, url, kind, track, amount, deadline, deadline_note, eligibility, note, score, why, status, user_note`

func scan(rows *sql.Rows) ([]Entry, error) {
	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.Key, &e.Name, &e.URL, &e.Kind, &e.Track, &e.Amount, &e.Deadline, &e.DeadlineNote, &e.Eligibility, &e.Note, &e.Score, &e.Why, &e.Status, &e.UserNote); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// List returns all entries, best score first; expired deadlines sink to the bottom.
func List(ctx context.Context, db *sql.DB) ([]Entry, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+cols+` FROM funding ORDER BY
		CASE WHEN status IN ('skip','rejected','won') THEN 1 ELSE 0 END,
		CASE WHEN deadline <> '' AND deadline < date('now') THEN 1 ELSE 0 END,
		score DESC, deadline`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scan(rows)
}

// Upcoming returns open entries with a deadline within the next n days (score >= minScore).
func Upcoming(ctx context.Context, db *sql.DB, days, minScore int) ([]Entry, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+cols+` FROM funding WHERE status IN ('open','applied') AND deadline <> ''
		AND deadline >= date('now') AND deadline <= date('now', ?) AND score >= ? ORDER BY deadline`, fmt.Sprintf("+%d days", days), minScore)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scan(rows)
}

// Seed inserts new curated entries and refreshes curated columns of existing ones
// (status and user_note are preserved). Returns number of new rows.
func Seed(ctx context.Context, db *sql.DB) (int, error) {
	n := 0
	for _, e := range Seeds {
		e = applyVerified(e)
		res, err := db.ExecContext(ctx, `INSERT INTO funding (key, name, url, kind, track, amount, deadline, deadline_note, eligibility, note, score, why)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(key) DO UPDATE SET name=excluded.name, url=excluded.url, kind=excluded.kind, track=excluded.track,
			amount=excluded.amount, deadline=excluded.deadline, deadline_note=excluded.deadline_note, eligibility=excluded.eligibility,
			note=excluded.note, score=excluded.score, why=excluded.why, updated_at=datetime('now')`,
			e.Key, e.Name, e.URL, e.Kind, e.Track, e.Amount, e.Deadline, e.DeadlineNote, e.Eligibility, e.Note, e.Score, e.Why)
		if err != nil {
			return n, fmt.Errorf("seed %s: %w", e.Key, err)
		}
		_ = res
		n++
	}
	return n, nil
}

var validStatus = map[string]bool{"open": true, "applied": true, "rejected": true, "won": true, "skip": true}

func SetStatus(ctx context.Context, db *sql.DB, id int64, status, note string) error {
	if !validStatus[status] {
		return fmt.Errorf("bad status %q", status)
	}
	_, err := db.ExecContext(ctx, `UPDATE funding SET status=?, user_note=?, updated_at=datetime('now') WHERE id=?`, status, strings.TrimSpace(note), id)
	return err
}

// ReportSection renders the block appended to the weekly email: deadlines in the
// next `days` days for entries scoring >= minScore. Empty when nothing is due.
// urgent is true when a deadline falls within the next 14 days (forces the email).
func ReportSection(ctx context.Context, db *sql.DB, siteURL string, days, minScore int) (text string, urgent bool, lines []string) {
	up, _ := Upcoming(ctx, db, days, minScore)
	if len(up) == 0 {
		return fmt.Sprintf("FUNDING DEADLINES · none in the next %d days\n\n", days), false, nil
	}
	for _, e := range up {
		if e.Status == "open" && e.DaysLeft() <= 14 {
			urgent = true
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "FUNDING DEADLINES · next %d days (%d)\n%s\n", days, len(up), strings.Repeat("-", 60))
	for _, e := range up {
		flag := ""
		if e.Status == "applied" {
			flag = " (applied)"
		} else if e.DaysLeft() <= 14 {
			flag = " ‼"
		}
		fmt.Fprintf(&b, "  %s · in %2dd · [%d] %s%s — %s\n        %s\n", e.Deadline, e.DaysLeft(), e.Score, e.Name, flag, e.Amount, e.URL)
		lines = append(lines, fmt.Sprintf("[%d] %s — %s, deadline %s (in %d days, status %s)", e.Score, e.Name, e.Amount, e.Deadline, e.DaysLeft(), e.Status))
	}
	fmt.Fprintf(&b, "  full list: %s/admin/funding\n", siteURL)
	return b.String(), urgent, lines
}

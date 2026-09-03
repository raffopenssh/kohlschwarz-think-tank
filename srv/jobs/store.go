package jobs

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Row is a stored posting.
type Row struct {
	ID         int64
	URL        string
	Source     string
	Title      string
	Org        string
	Location   string
	Snippet    string
	Lang       string
	Region     string
	Kind       string
	Posted     string
	Deadline   string
	FirstSeen  string
	LastSeen   string
	Score      *int64
	Why        string
	ReportedAt *string
	Hidden     bool
}

// ScoreVal returns the score or -1.
func (r Row) ScoreVal() int64 {
	if r.Score == nil {
		return -1
	}
	return *r.Score
}

// IsNew reports whether the posting was first seen within the last 8 days.
func (r Row) IsNew() bool {
	t, err := time.Parse("2006-01-02 15:04:05", r.FirstSeen)
	return err == nil && time.Since(t) < 8*24*time.Hour
}

const rowCols = `id, url, source, title, org, location, snippet, lang, region, kind, posted, deadline, first_seen, last_seen, score, why, reported_at, hidden`

func scanRows(rows *sql.Rows) ([]Row, error) {
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		var hidden int64
		if err := rows.Scan(&r.ID, &r.URL, &r.Source, &r.Title, &r.Org, &r.Location, &r.Snippet, &r.Lang, &r.Region, &r.Kind, &r.Posted, &r.Deadline, &r.FirstSeen, &r.LastSeen, &r.Score, &r.Why, &r.ReportedAt, &hidden); err != nil {
			return nil, err
		}
		r.Hidden = hidden == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

// Upsert inserts a posting or refreshes last_seen. Returns true if new.
func Upsert(ctx context.Context, db *sql.DB, p Posting) (bool, error) {
	res, err := db.ExecContext(ctx, `INSERT INTO job_postings (url, source, title, org, location, snippet, lang, region, posted, deadline)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(url) DO UPDATE SET last_seen = datetime('now'),
			deadline = CASE WHEN excluded.deadline != '' THEN excluded.deadline ELSE job_postings.deadline END,
			snippet = CASE WHEN length(excluded.snippet) > length(job_postings.snippet) THEN excluded.snippet ELSE job_postings.snippet END`,
		p.URL, p.Source, p.Title, p.Org, p.Location, p.Snippet, p.Lang, p.Region, p.Posted, p.Deadline)
	if err != nil {
		return false, err
	}
	id, _ := res.LastInsertId()
	var firstSeen, lastSeen string
	if err := db.QueryRowContext(ctx, `SELECT first_seen, last_seen FROM job_postings WHERE url = ?`, p.URL).Scan(&firstSeen, &lastSeen); err != nil {
		return false, err
	}
	return id > 0 && firstSeen == lastSeen, nil
}

// List returns postings ordered by score, newest first; unranked at the end.
func List(ctx context.Context, db *sql.DB, includeHidden bool, limit int) ([]Row, error) {
	q := `SELECT ` + rowCols + ` FROM job_postings WHERE (? OR hidden = 0)
		ORDER BY score IS NULL, score DESC, first_seen DESC LIMIT ?`
	rows, err := db.QueryContext(ctx, q, includeHidden, limit)
	if err != nil {
		return nil, err
	}
	return scanRows(rows)
}

// Unranked returns postings not yet scored.
func Unranked(ctx context.Context, db *sql.DB, limit int) ([]Row, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+rowCols+` FROM job_postings WHERE score IS NULL AND hidden = 0 ORDER BY first_seen DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	return scanRows(rows)
}

// Unreported returns ranked postings with score >= min that were never emailed.
func Unreported(ctx context.Context, db *sql.DB, minScore int) ([]Row, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+rowCols+` FROM job_postings WHERE score >= ? AND reported_at IS NULL AND hidden = 0 ORDER BY score DESC, first_seen DESC`, minScore)
	if err != nil {
		return nil, err
	}
	return scanRows(rows)
}

func SetHidden(ctx context.Context, db *sql.DB, id int64, hidden bool) error {
	_, err := db.ExecContext(ctx, `UPDATE job_postings SET hidden = ? WHERE id = ?`, hidden, id)
	return err
}

func MarkReported(ctx context.Context, db *sql.DB, ids []int64) error {
	for _, id := range ids {
		if _, err := db.ExecContext(ctx, `UPDATE job_postings SET reported_at = datetime('now') WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

// Purge removes stale postings not seen for 90 days and never scored well.
func Purge(ctx context.Context, db *sql.DB) {
	db.ExecContext(ctx, `DELETE FROM job_postings WHERE last_seen < datetime('now','-90 days') AND (score IS NULL OR score < 50)`)
}

// --- Runs ---------------------------------------------------------------------

type Run struct {
	ID         int64
	Started    string
	Finished   *string
	Kind       string
	SourcesOK  int64
	SourcesErr int64
	Found      int64
	Matched    int64
	NewCount   int64
	Ranked     int64
	Model      string
	InTokens   int64
	OutTokens  int64
	CostUSD    float64
	Log        string
}

func insertRun(ctx context.Context, db *sql.DB, r Run) error {
	_, err := db.ExecContext(ctx, `INSERT INTO job_runs (started, finished, kind, sources_ok, sources_err, found, matched, new_count, ranked, llm_model, llm_in_tokens, llm_out_tokens, llm_cost_usd, log)
		VALUES (?, datetime('now'), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Started, r.Kind, r.SourcesOK, r.SourcesErr, r.Found, r.Matched, r.NewCount, r.Ranked, r.Model, r.InTokens, r.OutTokens, r.CostUSD, r.Log)
	return err
}

func Runs(ctx context.Context, db *sql.DB, limit int) ([]Run, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, started, finished, kind, sources_ok, sources_err, found, matched, new_count, ranked, llm_model, llm_in_tokens, llm_out_tokens, llm_cost_usd, log FROM job_runs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.Started, &r.Finished, &r.Kind, &r.SourcesOK, &r.SourcesErr, &r.Found, &r.Matched, &r.NewCount, &r.Ranked, &r.Model, &r.InTokens, &r.OutTokens, &r.CostUSD, &r.Log); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LastRun returns the most recent run of a kind, or nil.
func LastRun(ctx context.Context, db *sql.DB, kind string) *Run {
	rows, err := db.QueryContext(ctx, `SELECT id, started, finished, kind, sources_ok, sources_err, found, matched, new_count, ranked, llm_model, llm_in_tokens, llm_out_tokens, llm_cost_usd, log FROM job_runs WHERE kind = ? ORDER BY id DESC LIMIT 1`, kind)
	if err != nil {
		return nil
	}
	defer rows.Close()
	if rows.Next() {
		var r Run
		if rows.Scan(&r.ID, &r.Started, &r.Finished, &r.Kind, &r.SourcesOK, &r.SourcesErr, &r.Found, &r.Matched, &r.NewCount, &r.Ranked, &r.Model, &r.InTokens, &r.OutTokens, &r.CostUSD, &r.Log) == nil {
			return &r
		}
	}
	return nil
}

// Cost summarises LLM spend.
type Cost struct {
	MonthUSD  float64
	TotalUSD  float64
	MonthIn   int64
	MonthOut  int64
	TotalIn   int64
	TotalOut  int64
	MonthName string
}

func GetCost(ctx context.Context, db *sql.DB) Cost {
	var c Cost
	c.MonthName = time.Now().UTC().Format("January 2006")
	db.QueryRowContext(ctx, `SELECT COALESCE(SUM(llm_cost_usd),0), COALESCE(SUM(llm_in_tokens),0), COALESCE(SUM(llm_out_tokens),0) FROM job_runs WHERE strftime('%Y-%m', started) = strftime('%Y-%m','now')`).Scan(&c.MonthUSD, &c.MonthIn, &c.MonthOut)
	db.QueryRowContext(ctx, `SELECT COALESCE(SUM(llm_cost_usd),0), COALESCE(SUM(llm_in_tokens),0), COALESCE(SUM(llm_out_tokens),0) FROM job_runs`).Scan(&c.TotalUSD, &c.TotalIn, &c.TotalOut)
	return c
}

// CostLine renders the mandatory cost line for reports.
func (c Cost) CostLine() string {
	return fmt.Sprintf("LLM cost (%s via exe.dev gateway): %s this month (%s, %d in / %d out tokens) · %s cumulative total (%d in / %d out tokens)",
		Model, usd(c.MonthUSD), c.MonthName, c.MonthIn, c.MonthOut, usd(c.TotalUSD), c.TotalIn, c.TotalOut)
}

func usd(v float64) string {
	if v < 0.01 {
		return fmt.Sprintf("$%.4f", v)
	}
	return fmt.Sprintf("$%.3f", v)
}

// --- Fetch pipeline -----------------------------------------------------------

// FetchAll pulls every source, filters by Match, stores matches. Returns the run record.
func FetchAll(ctx context.Context, db *sql.DB) Run {
	run := Run{Started: time.Now().UTC().Format("2006-01-02 15:04:05"), Kind: "fetch"}
	var logb strings.Builder
	for _, s := range Sources {
		ps, err := Fetch(ctx, s)
		if err != nil {
			run.SourcesErr++
			fmt.Fprintf(&logb, "✗ %s: %v\n", s.Name, err)
			slog.Warn("jobs fetch", "source", s.Name, "error", err)
			continue
		}
		run.SourcesOK++
		run.Found += int64(len(ps))
		m, n := 0, 0
		for _, p := range ps {
			if !Match(p) {
				continue
			}
			m++
			isNew, err := Upsert(ctx, db, p)
			if err != nil {
				slog.Warn("jobs upsert", "url", p.URL, "error", err)
				continue
			}
			if isNew {
				n++
			}
		}
		run.Matched += int64(m)
		run.NewCount += int64(n)
		fmt.Fprintf(&logb, "✓ %s: %d found, %d matched, %d new\n", s.Name, len(ps), m, n)
		// Be polite; LinkedIn guest search rate-limits after ~15 quick requests.
		if s.Kind == "linkedin" {
			time.Sleep(6 * time.Second)
		} else {
			time.Sleep(800 * time.Millisecond)
		}
	}
	Purge(ctx, db)
	run.Log = logb.String()
	if err := insertRun(ctx, db, run); err != nil {
		slog.Warn("jobs insert run", "error", err)
	}
	return run
}

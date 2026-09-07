package jobs

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
)

// Row is a stored posting.
type Row struct {
	ID              int64
	retried         bool // rank: already requeued once after being dropped from a reply
	URL             string
	Source          string
	Title           string
	Org             string
	Location        string
	Snippet         string
	Lang            string
	Region          string
	Kind            string
	Posted          string
	Deadline        string
	FirstSeen       string
	LastSeen        string
	Score           *int64
	Why             string
	Brief           string
	ReportedAt      *string
	Hidden          bool
	Salary          string  // salary / grade text as scraped from the page ('' = unknown)
	Applicants      *int64  // LinkedIn public applicant count, nil = unknown
	CheckedAt       *string // last page re-check (signals.go)
	ClosedAt        *string // page says closed / gone, nil = live
	Reposted        bool    // source shows it re-advertised
	Vote            int     // owner thumbs: 1 up, -1 down, 0 none
	UserNote        string  // owner's free-text note
	TrashReason     string  // why it was hidden ('' = not given)
	Events          []Event // change history (loaded by AttachEvents)
	latestFirstSeen string  // newest first_seen among merged copies (Dedupe)
	Dupes           int     // extra copies collapsed by Dedupe (not stored)
}

// Since renders FirstSeen as a date.
func (r Row) Since() string {
	if len(r.FirstSeen) >= 10 {
		return r.FirstSeen[:10]
	}
	return r.FirstSeen
}

// LastSeenDate renders LastSeen as a date.
func (r Row) LastSeenDate() string {
	if len(r.LastSeen) >= 10 {
		return r.LastSeen[:10]
	}
	return r.LastSeen
}

// SeenAgo renders LastSeen relative to now.
func (r Row) SeenAgo() string { return Ago(r.LastSeen) }

var snippetPrefixRe = regexp.MustCompile(`^\[[^\]]{1,40}\]\s*`)

// HasBrief reports whether Context is the LLM brief rather than the raw snippet.
func (r Row) HasBrief() bool { return r.Brief != "" }

// Context is what the details panel shows under the 'why': the LLM brief when
// one exists, otherwise the raw fetched snippet with source tag, title echo and
// boilerplate stripped.
func (r Row) Context() string {
	if r.Brief != "" {
		return strings.Join(strings.Split(r.Brief, "\n"), " · ")
	}
	s := snippetPrefixRe.ReplaceAllString(strings.TrimSpace(r.Snippet), "")
	s = strings.TrimPrefix(s, r.Title)
	s = strings.TrimLeft(s, " ·")
	s = strings.TrimSpace(s)
	if s == "" || s == r.Title {
		return ""
	}
	const max = 600
	if len(s) > max {
		if i := strings.LastIndex(s[:max], " "); i > max/2 {
			s = s[:i]
		} else {
			s = s[:max]
		}
		s += " …"
	}
	return s
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

const rowCols = `id, url, source, title, org, location, snippet, lang, region, kind, posted, deadline, first_seen, last_seen, score, why, brief, reported_at, hidden, salary, applicants, checked_at, closed_at, reposted, vote, user_note, trash_reason`

func scanRows(rows *sql.Rows) ([]Row, error) {
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		var hidden, reposted int64
		if err := rows.Scan(&r.ID, &r.URL, &r.Source, &r.Title, &r.Org, &r.Location, &r.Snippet, &r.Lang, &r.Region, &r.Kind, &r.Posted, &r.Deadline, &r.FirstSeen, &r.LastSeen, &r.Score, &r.Why, &r.Brief, &r.ReportedAt, &hidden, &r.Salary, &r.Applicants, &r.CheckedAt, &r.ClosedAt, &reposted, &r.Vote, &r.UserNote, &r.TrashReason); err != nil {
			return nil, err
		}
		r.Hidden = hidden == 1
		r.Reposted = reposted == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

// Upsert inserts a posting or refreshes last_seen. Returns true if new.
//
// On refresh it also records hiring signals as job_events: a deadline that
// moved (extended / shortened), a later posted date (re-advertised), a posting
// that reappears after being gone from every source for 10+ days, and a
// posting that was marked closed but is listed again (reopened).
func Upsert(ctx context.Context, db *sql.DB, p Posting) (bool, error) {
	var id int64
	var oldDeadline, oldPosted, lastSeen string
	var closedAt *string
	err := db.QueryRowContext(ctx, `SELECT id, deadline, posted, last_seen, closed_at FROM job_postings WHERE url = ?`, p.URL).Scan(&id, &oldDeadline, &oldPosted, &lastSeen, &closedAt)
	if err == sql.ErrNoRows {
		_, err = db.ExecContext(ctx, `INSERT INTO job_postings (url, source, title, org, location, snippet, lang, region, posted, deadline)
			VALUES (?,?,?,?,?,?,?,?,?,?)`, p.URL, p.Source, p.Title, p.Org, p.Location, p.Snippet, p.Lang, p.Region, p.Posted, p.Deadline)
		return err == nil, err
	}
	if err != nil {
		return false, err
	}
	// Signals from the listing itself (free, no page fetch).
	if p.Deadline != "" && oldDeadline != "" && p.Deadline != oldDeadline {
		kind := "deadline_extended"
		if p.Deadline < oldDeadline {
			kind = "deadline_shortened"
		}
		AddEvent(ctx, db, id, kind, oldDeadline, p.Deadline)
	}
	if p.Posted != "" && oldPosted != "" && p.Posted > oldPosted {
		AddEvent(ctx, db, id, "reposted", oldPosted, p.Posted)
	}
	if t, e := time.Parse("2006-01-02 15:04:05", lastSeen); e == nil && time.Since(t) > 10*24*time.Hour {
		AddEvent(ctx, db, id, "reappeared", lastSeen[:10], time.Now().UTC().Format("2006-01-02"))
	}
	if closedAt != nil {
		AddEvent(ctx, db, id, "reopened", (*closedAt)[:10], "")
	}
	_, err = db.ExecContext(ctx, `UPDATE job_postings SET last_seen = datetime('now'), closed_at = NULL,
			deadline = CASE WHEN ? != '' THEN ? ELSE deadline END,
			posted = CASE WHEN ? > posted THEN ? ELSE posted END,
			reposted = CASE WHEN ? > posted AND posted != '' THEN 1 ELSE reposted END,
			snippet = CASE WHEN length(?) > length(snippet) THEN ? ELSE snippet END
		WHERE id = ?`,
		p.Deadline, p.Deadline, p.Posted, p.Posted, p.Posted, p.Snippet, p.Snippet, id)
	return false, err
}

// Event is one recorded change on a posting.
type Event struct {
	At   string
	Kind string
	Old  string
	New  string
}

// AddEvent appends a change record unless the identical event was already
// recorded within the last day (fetch runs may see the same source twice).
func AddEvent(ctx context.Context, db *sql.DB, postingID int64, kind, old, new string) {
	var n int
	db.QueryRowContext(ctx, `SELECT count(*) FROM job_events WHERE posting_id = ? AND kind = ? AND old = ? AND new = ? AND at > datetime('now','-1 day')`, postingID, kind, old, new).Scan(&n)
	if n > 0 {
		return
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO job_events (posting_id, kind, old, new) VALUES (?,?,?,?)`, postingID, kind, old, new); err != nil {
		slog.Warn("jobs event", "error", err)
	}
}

// AttachEvents loads the change history for every row (one query).
func AttachEvents(ctx context.Context, db *sql.DB, rows []Row) {
	if len(rows) == 0 {
		return
	}
	idx := map[int64]int{}
	for i, r := range rows {
		idx[r.ID] = i
	}
	q, err := db.QueryContext(ctx, `SELECT posting_id, at, kind, old, new FROM job_events ORDER BY at`)
	if err != nil {
		return
	}
	defer q.Close()
	for q.Next() {
		var pid int64
		var e Event
		if q.Scan(&pid, &e.At, &e.Kind, &e.Old, &e.New) != nil {
			continue
		}
		if i, ok := idx[pid]; ok {
			rows[i].Events = append(rows[i].Events, e)
		}
	}
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

// Unbriefed returns ranked, visible postings scoring >= min that have no brief yet.
func Unbriefed(ctx context.Context, db *sql.DB, min, limit int) ([]Row, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+rowCols+` FROM job_postings WHERE score >= ? AND hidden = 0 AND briefed_at IS NULL ORDER BY score DESC, first_seen DESC LIMIT ?`, min, limit)
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

// SetHidden trashes/restores a posting; reason is kept (only overwritten when non-empty).
func SetHidden(ctx context.Context, db *sql.DB, id int64, hidden bool, reason string) error {
	_, err := db.ExecContext(ctx, `UPDATE job_postings SET hidden = ?, trash_reason = CASE WHEN ? <> '' THEN ? ELSE trash_reason END WHERE id = ?`, hidden, reason, reason, id)
	return err
}

// SetVote stores the owner's thumbs (1, -1 or 0).
func SetVote(ctx context.Context, db *sql.DB, id int64, vote int) error {
	if vote < -1 || vote > 1 {
		vote = 0
	}
	_, err := db.ExecContext(ctx, `UPDATE job_postings SET vote = ? WHERE id = ?`, vote, id)
	return err
}

// SetNote stores the owner's free-text note.
func SetNote(ctx context.Context, db *sql.DB, id int64, note string) error {
	note = strings.TrimSpace(note)
	if len(note) > 2000 {
		note = note[:2000]
	}
	_, err := db.ExecContext(ctx, `UPDATE job_postings SET user_note = ? WHERE id = ?`, note, id)
	return err
}

// Get returns one posting.
func Get(ctx context.Context, db *sql.DB, id int64) (*Row, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+rowCols+` FROM job_postings WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	out, err := scanRows(rows)
	if err != nil || len(out) == 0 {
		return nil, err
	}
	return &out[0], nil
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
	Credit    float64 // one-time voucher applied to this month (settings llm_credit_YYYY-MM)
	SpentUSD  float64 // raw spend this month before credit
}

func GetCost(ctx context.Context, db *sql.DB) Cost {
	var c Cost
	c.MonthName = time.Now().UTC().Format("January 2006")
	db.QueryRowContext(ctx, `SELECT COALESCE(SUM(llm_cost_usd),0), COALESCE(SUM(llm_in_tokens),0), COALESCE(SUM(llm_out_tokens),0) FROM job_runs WHERE strftime('%Y-%m', started) = strftime('%Y-%m','now')`).Scan(&c.MonthUSD, &c.MonthIn, &c.MonthOut)
	db.QueryRowContext(ctx, `SELECT COALESCE(SUM(llm_cost_usd),0), COALESCE(SUM(llm_in_tokens),0), COALESCE(SUM(llm_out_tokens),0) FROM job_runs`).Scan(&c.TotalUSD, &c.TotalIn, &c.TotalOut)
	c.SpentUSD = c.MonthUSD
	db.QueryRowContext(ctx, `SELECT CAST(value AS REAL) FROM settings WHERE key = ?`, CreditKey()).Scan(&c.Credit)
	c.MonthUSD = max(0, c.SpentUSD-c.Credit)
	return c
}

// CreditKey is the settings key of this month's voucher.
func CreditKey() string { return "llm_credit_" + time.Now().UTC().Format("2006-01") }

// ApplyVoucher zeroes this month's budget usage by crediting what has been spent so far.
func ApplyVoucher(ctx context.Context, db *sql.DB) (float64, error) {
	c := GetCost(ctx, db)
	_, err := db.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES(?,?,strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, CreditKey(), fmt.Sprintf("%.6f", c.SpentUSD))
	return c.SpentUSD, err
}

// CostLine renders the mandatory cost line for reports.
func (c Cost) CostLine() string {
	credit := ""
	if c.Credit > 0 {
		credit = fmt.Sprintf(", %s spent − %s voucher", usd(c.SpentUSD), usd(c.Credit))
	}
	return fmt.Sprintf("LLM cost (%s via exe.dev gateway): %s this month (%s, %d in / %d out tokens%s) · %s cumulative total (%d in / %d out tokens)",
		Model, usd(c.MonthUSD), c.MonthName, c.MonthIn, c.MonthOut, credit, usd(c.TotalUSD), c.TotalIn, c.TotalOut)
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

package jobs

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const emailEndpoint = "http://169.254.169.254/gateway/email/send"

// ReportMinScore is the threshold for "really interesting".
const ReportMinScore = 70

// BuildReport renders the plain-text weekly report. Returns text and the ids included.
func BuildReport(ctx context.Context, db *sql.DB, siteURL string) (string, []int64) {
	fresh, _ := Unreported(ctx, db, ReportMinScore)
	all, _ := List(ctx, db, false, 400)
	cost := GetCost(ctx, db)
	lastFetch := LastRun(ctx, db, "fetch")

	var b strings.Builder
	fmt.Fprintf(&b, "NATIONAL PARK LEADERSHIP RADAR — week of %s\n", time.Now().UTC().Format("2 Jan 2006"))
	b.WriteString("Senior park director roles & PA consultancies · Austria / Sub-Saharan Africa / global · EN/DE/FR\n")
	b.WriteString(strings.Repeat("=", 72) + "\n\n")

	var ids []int64
	if len(fresh) == 0 {
		b.WriteString("No new postings scoring ≥ 70 this week.\n\n")
	} else {
		fmt.Fprintf(&b, "NEW & INTERESTING (%d, score ≥ %d)\n\n", len(fresh), ReportMinScore)
		for _, r := range fresh {
			ids = append(ids, r.ID)
			writeRow(&b, r)
		}
	}

	// Still-open reminders: previously reported, deadline in the future or seen in the last week.
	var open []Row
	for _, r := range all {
		if r.ReportedAt == nil || r.ScoreVal() < ReportMinScore {
			continue
		}
		if (r.Deadline != "" && r.Deadline >= time.Now().Format("2006-01-02")) || recentlySeen(r) {
			open = append(open, r)
		}
	}
	if len(open) > 0 {
		fmt.Fprintf(&b, "\nSTILL OPEN (reported earlier, %d)\n\n", len(open))
		for i, r := range open {
			if i >= 15 {
				fmt.Fprintf(&b, "  … and %d more at %s/admin/jobs\n", len(open)-15, siteURL)
				break
			}
			fmt.Fprintf(&b, "  [%d] %s — %s%s\n      %s\n", r.ScoreVal(), r.Title, orgLoc(r), dl(r), r.URL)
		}
	}

	b.WriteString("\n" + strings.Repeat("-", 72) + "\n")
	if lastFetch != nil {
		fmt.Fprintf(&b, "Sources: %d ok, %d failed · %d postings scanned, %d keyword matches, %d new this run\n",
			lastFetch.SourcesOK, lastFetch.SourcesErr, lastFetch.Found, lastFetch.Matched, lastFetch.NewCount)
	}
	b.WriteString(cost.CostLine() + "\n")
	fmt.Fprintf(&b, "Full list & controls: %s/admin/jobs\n", siteURL)
	return b.String(), ids
}

func recentlySeen(r Row) bool {
	t, err := time.Parse("2006-01-02 15:04:05", r.LastSeen)
	return err == nil && time.Since(t) < 8*24*time.Hour
}

func orgLoc(r Row) string {
	parts := []string{}
	if r.Org != "" {
		parts = append(parts, r.Org)
	}
	if r.Location != "" {
		parts = append(parts, r.Location)
	}
	return strings.Join(parts, ", ")
}

func dl(r Row) string {
	if r.Deadline != "" {
		return " · deadline " + r.Deadline
	}
	return ""
}

func writeRow(b *strings.Builder, r Row) {
	tags := []string{strings.ToUpper(r.Region)}
	if r.Kind != "" {
		tags = append(tags, r.Kind)
	}
	if r.Lang != "" {
		tags = append(tags, r.Lang)
	}
	fmt.Fprintf(b, "[%d] %s\n", r.ScoreVal(), r.Title)
	if ol := orgLoc(r); ol != "" {
		fmt.Fprintf(b, "     %s\n", ol)
	}
	meta := strings.Join(tags, " · ")
	if r.Posted != "" {
		meta += " · posted " + r.Posted
	}
	if r.Deadline != "" {
		meta += " · deadline " + r.Deadline
	}
	fmt.Fprintf(b, "     %s\n", meta)
	if r.Why != "" {
		fmt.Fprintf(b, "     → %s\n", r.Why)
	}
	fmt.Fprintf(b, "     %s\n     via %s\n\n", r.URL, r.Source)
}

// SendEmail posts a plain-text email via the exe.dev gateway.
func SendEmail(ctx context.Context, to, subject, body string) error {
	payload, _ := json.Marshal(map[string]string{"to": to, "subject": subject, "body": body})
	req, _ := http.NewRequestWithContext(ctx, "POST", emailEndpoint, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var r struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	json.Unmarshal(b, &r)
	if !r.Success {
		return fmt.Errorf("email gateway: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// WeeklyReport builds and emails the report; marks postings as reported. Set force to send even when nothing new.
func WeeklyReport(ctx context.Context, db *sql.DB, to, siteURL string, force bool) (string, error) {
	text, ids := BuildReport(ctx, db, siteURL)
	run := Run{Started: time.Now().UTC().Format("2006-01-02 15:04:05"), Kind: "email", NewCount: int64(len(ids))}
	if len(ids) == 0 && !force {
		run.Log = "nothing new ≥ " + fmt.Sprint(ReportMinScore) + "; email skipped"
		insertRun(ctx, db, run)
		return text, nil
	}
	subj := fmt.Sprintf("Park leadership radar: %d new · %s", len(ids), time.Now().UTC().Format("2 Jan"))
	if err := SendEmail(ctx, to, subj, text); err != nil {
		run.Log = "send failed: " + err.Error()
		insertRun(ctx, db, run)
		return text, err
	}
	MarkReported(ctx, db, ids)
	run.Log = "sent to " + to
	insertRun(ctx, db, run)
	slog.Info("jobs weekly report sent", "to", to, "new", len(ids))
	return text, nil
}

// Scheduler runs fetch daily-ish and fetch+rank+email once a week.
// Weekly: Monday 06:00 UTC. Daily fetch: 04:00 UTC (keeps postings fresh, no LLM cost).
func Scheduler(ctx context.Context, db *sql.DB, to, siteURL string) {
	for {
		now := time.Now().UTC()
		nextDaily := time.Date(now.Year(), now.Month(), now.Day(), 4, 0, 0, 0, time.UTC)
		if !nextDaily.After(now) {
			nextDaily = nextDaily.Add(24 * time.Hour)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(nextDaily)):
		}
		FetchAll(ctx, db)
		if time.Now().UTC().Weekday() == time.Monday {
			RankPending(ctx, db, 200)
			if _, err := WeeklyReport(ctx, db, to, siteURL, false); err != nil {
				slog.Warn("jobs weekly report", "error", err)
			}
		}
	}
}

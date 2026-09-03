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

// ReportExtra, when set, returns an extra section appended before the footer
// (used for funding deadlines). Return "" for nothing; urgent=true forces the
// weekly email even when there are no new job picks.
var ReportExtra func(ctx context.Context) (text string, urgent bool)

// ReportMax caps the number of new items per weekly email; the best-scoring go first.
const ReportMax = 10

// BuildReport renders the plain-text weekly digest: the best new matches (deduped,
// score ≥ ReportMinScore, at most ReportMax) each with a one-sentence fit note,
// then a compact list of earlier picks that are still open. Returns text and the
// ids covered (including collapsed duplicates) so they are marked as reported.
func BuildReport(ctx context.Context, db *sql.DB, siteURL string) (string, []int64) {
	freshAll, _ := Unreported(ctx, db, ReportMinScore)
	all, _ := List(ctx, db, false, 400)
	cost := GetCost(ctx, db)
	lastFetch := LastRun(ctx, db, "fetch")

	fresh := Dedupe(freshAll)
	if len(fresh) > ReportMax {
		fresh = fresh[:ReportMax]
	}
	// Mark every copy of a reported item (so dupes don't resurface next week).
	var ids []int64
	if len(fresh) > 0 {
		picked := map[string]bool{}
		for _, r := range fresh {
			picked[DedupeKey(r)] = true
		}
		for _, r := range freshAll {
			if picked[DedupeKey(r)] {
				ids = append(ids, r.ID)
			}
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "PARK LEADERSHIP RADAR · %s\n", time.Now().UTC().Format("Mon 2 Jan 2006"))
	b.WriteString("Park director roles & senior PA consultancies · Austria / Sub-Saharan Africa / global\n")
	b.WriteString(strings.Repeat("=", 64) + "\n\n")

	if len(fresh) == 0 {
		b.WriteString("Nothing new worth your time this week (no new posting scored ≥ 70).\n\n")
	} else {
		fmt.Fprintf(&b, "THIS WEEK'S PICKS (%d)\n\n", len(fresh))
		for i, r := range fresh {
			writePick(&b, i+1, r)
		}
	}

	// Still open: reported earlier, deadline in future or still listed at source.
	today := time.Now().Format("2006-01-02")
	var open []Row
	for _, r := range Dedupe(all) {
		if r.ReportedAt == nil || r.ScoreVal() < ReportMinScore {
			continue
		}
		if (r.Deadline != "" && r.Deadline >= today) || (r.Deadline == "" && recentlySeen(r)) {
			open = append(open, r)
		}
	}
	if len(open) > 0 {
		fmt.Fprintf(&b, "STILL OPEN FROM EARLIER WEEKS (%d)\n", len(open))
		for i, r := range open {
			if i >= 12 {
				fmt.Fprintf(&b, "  … %d more: %s/admin/jobs\n", len(open)-12, siteURL)
				break
			}
			fmt.Fprintf(&b, "  · %s — %s%s\n    %s\n", r.Title, orgLoc(r), dl(r), r.URL)
		}
		b.WriteString("\n")
	}

	if ReportExtra != nil {
		if extra, _ := ReportExtra(ctx); extra != "" {
			b.WriteString(extra + "\n")
		}
	}

	b.WriteString(strings.Repeat("-", 64) + "\n")
	if lastFetch != nil {
		fmt.Fprintf(&b, "Scan: %d sources (%d failed) · %d postings · %d keyword matches · %d new\n",
			lastFetch.SourcesOK+lastFetch.SourcesErr, lastFetch.SourcesErr, lastFetch.Found, lastFetch.Matched, lastFetch.NewCount)
	}
	b.WriteString(cost.CostLine() + "\n")
	fmt.Fprintf(&b, "All entries, hide/unhide, run log: %s/admin/jobs\n", siteURL)
	return b.String(), ids
}

// writePick renders one digest entry:
//
//  1. Title — Org, Location
//     Why it fits (one sentence). Score 95 · SSA · director · deadline 2026-09-30 · listed since 2026-09-01
//     https://…
func writePick(b *strings.Builder, n int, r Row) {
	fmt.Fprintf(b, "%d. %s", n, r.Title)
	if ol := orgLoc(r); ol != "" {
		fmt.Fprintf(b, " — %s", ol)
	}
	b.WriteString("\n")
	if r.Why != "" {
		fmt.Fprintf(b, "   %s\n", strings.TrimRight(r.Why, ".")+".")
	}
	meta := []string{fmt.Sprintf("score %d", r.ScoreVal()), strings.ToUpper(r.Region)}
	if r.Kind != "" && r.Kind != "other" {
		meta = append(meta, r.Kind)
	}
	if r.Deadline != "" {
		meta = append(meta, "deadline "+r.Deadline)
	}
	meta = append(meta, "listed since "+r.Since())
	if r.Dupes > 0 {
		meta = append(meta, fmt.Sprintf("%d further copies", r.Dupes))
	}
	fmt.Fprintf(b, "   %s\n   %s\n\n", strings.Join(meta, " · "), r.URL)
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
	picks := strings.Count(text, "\n   score ") // one meta line per pick
	run := Run{Started: time.Now().UTC().Format("2006-01-02 15:04:05"), Kind: "email", NewCount: int64(picks)}
	urgent := false
	if ReportExtra != nil {
		_, urgent = ReportExtra(ctx)
	}
	if len(ids) == 0 && !force && !urgent {
		run.Log = "nothing new ≥ " + fmt.Sprint(ReportMinScore) + "; email skipped"
		insertRun(ctx, db, run)
		return text, nil
	}
	subj := fmt.Sprintf("Park radar · %d pick(s) · %s", picks, time.Now().UTC().Format("2 Jan"))
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

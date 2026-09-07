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

// ReportExtra, when set, returns the funding-deadline section appended after the
// picks. Return "" for nothing; urgent=true forces the weekly email even when
// there are no new job picks. lines is a compact plain list handed to the LLM
// summary so it can mention deadlines.
var ReportExtra func(ctx context.Context) (text string, urgent bool, lines []string)

// ReportMax caps the number of new items per weekly email; the best-scoring go first.
const ReportMax = 10

// AlsoMin/AlsoMax bound the "also new" glimpse of runner-ups (titles only).
const (
	AlsoMin = 40
	AlsoMax = 8
)

// BuildReport renders the plain-text weekly digest:
//
//	header · LLM intro · this week's picks · also new (titles) · funding deadlines
//	· still open from earlier weeks · footer (scan stats, cost, admin link)
//
// Returns text and the ids covered (including collapsed duplicates and the
// runner-ups) so they are marked as reported and don't resurface.
func BuildReport(ctx context.Context, db *sql.DB, siteURL string) (string, []int64) {
	freshAll, _ := Unreported(ctx, db, AlsoMin)
	all, _ := List(ctx, db, false, 400)
	AttachEvents(ctx, db, all)
	AttachEvents(ctx, db, freshAll)
	cost := GetCost(ctx, db)
	lastFetch := LastRun(ctx, db, "fetch")

	var fresh, also []Row
	for _, r := range Dedupe(freshAll) {
		if r.ScoreVal() >= ReportMinScore && len(fresh) < ReportMax {
			fresh = append(fresh, r)
		} else if len(also) < AlsoMax {
			also = append(also, r)
		}
	}
	// Mark every copy of a reported item (so dupes don't resurface next week).
	var ids []int64
	picked := map[string]bool{}
	for _, r := range fresh {
		picked[DedupeKey(r)] = true
	}
	for _, r := range also {
		picked[DedupeKey(r)] = true
	}
	for _, r := range freshAll {
		if picked[DedupeKey(r)] {
			ids = append(ids, r.ID)
		}
	}

	extra, fundingLines := "", []string(nil)
	if ReportExtra != nil {
		extra, _, fundingLines = ReportExtra(ctx)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "PARK LEADERSHIP RADAR · %s\n", time.Now().UTC().Format("Mon 2 Jan 2006"))
	b.WriteString("Park director roles & senior PA consultancies · Austria / SSA / global\n")
	b.WriteString(strings.Repeat("=", 60) + "\n\n")

	if intro := Summarize(ctx, db, fresh, also, fundingLines); intro != "" {
		b.WriteString(wrap(intro, 72) + "\n\n")
	}

	if len(fresh) == 0 {
		b.WriteString("No new posting scored ≥ 70 this week.\n\n")
	} else {
		fmt.Fprintf(&b, "THIS WEEK'S PICKS (%d)\n%s\n", len(fresh), strings.Repeat("-", 60))
		for i, r := range fresh {
			writePick(&b, i+1, r)
		}
	}

	if len(also) > 0 {
		fmt.Fprintf(&b, "ALSO NEW · lower fit, titles only (%d)\n%s\n", len(also), strings.Repeat("-", 60))
		for _, r := range also {
			fmt.Fprintf(&b, "  [%d] %s", r.ScoreVal(), r.Title)
			if ol := orgLoc(r); ol != "" {
				fmt.Fprintf(&b, " — %s", ol)
			}
			b.WriteString(dl(r) + "\n")
			fmt.Fprintf(&b, "       %s\n", r.URL)
		}
		b.WriteString("\n")
	}

	if extra != "" {
		b.WriteString(extra + "\n")
	}

	// Hiring signals: anything ranked whose recruiter is visibly struggling
	// (deadline extended, re-advertised, overdue, thin field, pay raised) or
	// that closed since the last report. Only rows with a change this week.
	today := time.Now().Format("2006-01-02")
	weekAgo := time.Now().UTC().Add(-7 * 24 * time.Hour).Format("2006-01-02 15:04:05")
	deduped := Dedupe(all)
	var struggling, closed []Row
	for _, r := range deduped {
		if r.ScoreVal() < AlsoMin {
			continue
		}
		v := r.Verdict()
		changed := false
		for _, e := range r.Events {
			if e.At >= weekAgo {
				changed = true
				break
			}
		}
		switch {
		case v == "hard to fill" && changed && len(struggling) < 8:
			struggling = append(struggling, r)
		case v == "closed" && changed && r.ScoreVal() >= ReportMinScore && len(closed) < 6:
			closed = append(closed, r)
		}
	}
	if len(struggling) > 0 {
		fmt.Fprintf(&b, "STILL UNFILLED — RECRUITER SIGNALS (%d)\n%s\n", len(struggling), strings.Repeat("-", 60))
		b.WriteString("Posts where the hiring side visibly has not found anyone yet.\n")
		for _, r := range struggling {
			fmt.Fprintf(&b, "  [%d] %s — %s%s\n", r.ScoreVal(), r.Title, orgLoc(r), dl(r))
			var tags []string
			for _, sg := range r.Signals() {
				if sg.Hard || sg.Key == "long" || sg.Key == "fewapps" {
					tags = append(tags, sg.Label)
				}
			}
			if len(tags) > 0 {
				fmt.Fprintf(&b, "       %s\n", strings.Join(tags, " · "))
			}
			fmt.Fprintf(&b, "       %s\n", r.URL)
		}
		b.WriteString("\n")
	}
	if len(closed) > 0 {
		fmt.Fprintf(&b, "CLOSED THIS WEEK (%d)\n%s\n", len(closed), strings.Repeat("-", 60))
		for _, r := range closed {
			fmt.Fprintf(&b, "  · %s — %s (open %d days)\n", r.Title, orgLoc(r), r.AgeDays())
		}
		b.WriteString("\n")
	}

	// Still open: reported earlier, deadline in future or still listed at source.
	var open []Row
	for _, r := range deduped {
		if r.ReportedAt == nil || r.ScoreVal() < ReportMinScore || picked[DedupeKey(r)] || r.ClosedAt != nil {
			continue
		}
		if (r.Deadline != "" && r.Deadline >= today) || (r.Deadline == "" && recentlySeen(r)) {
			open = append(open, r)
		}
	}
	if len(open) > 0 {
		fmt.Fprintf(&b, "STILL OPEN FROM EARLIER WEEKS (%d)\n%s\n", len(open), strings.Repeat("-", 60))
		for i, r := range open {
			if i >= 12 {
				fmt.Fprintf(&b, "  … %d more: %s/admin/jobs\n", len(open)-12, siteURL)
				break
			}
			fmt.Fprintf(&b, "  · %s — %s%s\n    %s\n", r.Title, orgLoc(r), dl(r), r.URL)
		}
		b.WriteString("\n")
	}

	b.WriteString(strings.Repeat("=", 60) + "\n")
	if lastFetch != nil {
		fmt.Fprintf(&b, "Scan: %d sources (%d failed) · %d postings · %d keyword matches · %d new\n",
			lastFetch.SourcesOK+lastFetch.SourcesErr, lastFetch.SourcesErr, lastFetch.Found, lastFetch.Matched, lastFetch.NewCount)
	}
	b.WriteString(cost.CostLine() + "\n")
	fmt.Fprintf(&b, "Jobs (all entries, hide, run log): %s/admin/jobs\nFunding tracker: %s/admin/funding\n", siteURL, siteURL)
	return b.String(), ids
}

// wrap re-flows a paragraph to at most width columns (soft wrap for plain-text mail).
func wrap(s string, width int) string {
	var out, line strings.Builder
	for _, w := range strings.Fields(s) {
		if line.Len() > 0 && line.Len()+1+len(w) > width {
			out.WriteString(line.String() + "\n")
			line.Reset()
		}
		if line.Len() > 0 {
			line.WriteByte(' ')
		}
		line.WriteString(w)
	}
	out.WriteString(line.String())
	return out.String()
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
	if items := r.BriefItems(); len(items) > 0 {
		for _, it := range items {
			for i, ln := range strings.Split(wrap(it.Text, 60), "\n") {
				if i == 0 {
					fmt.Fprintf(b, "   %-7s %s\n", it.Label+":", ln)
				} else {
					fmt.Fprintf(b, "           %s\n", ln)
				}
			}
		}
	} else if r.Brief != "" {
		for _, ln := range strings.Split(wrap(r.Brief, 69), "\n") {
			fmt.Fprintf(b, "   %s\n", ln)
		}
	}
	meta := []string{fmt.Sprintf("fit %d/100", r.ScoreVal()), strings.ToUpper(r.Region)}
	if r.Kind != "" && r.Kind != "other" {
		meta = append(meta, r.Kind)
	}
	for _, sg := range r.Signals() {
		if sg.Hard {
			meta = append(meta, "⚑ "+sg.Label)
		}
	}
	if r.Deadline != "" {
		meta = append(meta, "deadline "+r.Deadline)
	}
	meta = append(meta, "listed since "+r.Since())
	if r.Dupes > 0 {
		meta = append(meta, fmt.Sprintf("%d further copies", r.Dupes))
	}
	fmt.Fprintf(b, "   %s\n   → %s\n\n", strings.Join(meta, " · "), r.URL)
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
	picks := strings.Count(text, "\n   fit ") // one meta line per pick
	run := Run{Started: time.Now().UTC().Format("2006-01-02 15:04:05"), Kind: "email", NewCount: int64(picks)}
	urgent := false
	if ReportExtra != nil {
		_, urgent, _ = ReportExtra(ctx)
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
		if !Current.Start("fetch") {
			continue
		}
		r := FetchAll(ctx, db)
		Current.Switch("rank")
		rk := RankPending(ctx, db, 200)
		Current.Switch("brief")
		br := BriefPending(ctx, db, 60)
		Current.Switch("check")
		ck := CheckPending(ctx, db, 40)
		Current.Finish(fmt.Sprintf("scheduled fetch: %d new · ranked %d · briefed %d · re-checked %d", r.NewCount, rk.Ranked, br.Ranked, ck.Found))
		if time.Now().UTC().Weekday() == time.Monday {
			if Current.Start("email") {
				if _, err := WeeklyReport(ctx, db, to, siteURL, false); err != nil {
					slog.Warn("jobs weekly report", "error", err)
				}
				Current.Finish("scheduled email")
			}
		}
	}
}

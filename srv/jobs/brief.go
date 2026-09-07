package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"regexp"
	"strings"
	"time"
)

var liJSONDescRe = regexp.MustCompile(`"description":"((?:[^"\\]|\\.)*)"`)

// BriefMinScore: only postings that made the list get a fetched brief.
const BriefMinScore = 35

const briefPrompt = `You brief a former national park director (fluent EN/DE/FR) on ONE job posting or tender. He wants (A) to lead a national park / protected area, (B) senior consultancies on protected-area management, governance, finance or evaluation, or (C) a substantive post inside an Austrian Land/Bund authority that governs a national park, as a step towards directing it.

You get the title, metadata and the fetched page text (may contain navigation noise; ignore it). Answer in English with EXACTLY these four lines, each "Label: text", telegraphic style (drop articles and filler), no markdown, no preamble, do not repeat the title, only what the text supports:
What: type (permanent post / fixed-term / consultancy ToR / tender lot / news item), employer and unit/department as named in the text
Terms: grade or salary or contract value, duration, location, deadline if stated
Duties: core responsibilities, <=18 words
Fit: one clause on why it does or does not match A, B or C (for C name the park the authority governs, or 'no park link')
If the page text is missing or a login wall, set What to 'Page not readable; from metadata:' followed by what you can infer, and keep the other lines short.`

// BriefPending fetches the page of each ranked posting (score >= BriefMinScore)
// without a brief and asks the LLM for a short, structured summary. Respects
// the monthly budget like RankPending.
func BriefPending(ctx context.Context, db *sql.DB, maxItems int) Run {
	run := Run{Started: time.Now().UTC().Format("2006-01-02 15:04:05"), Kind: "brief", Model: Model}
	var logb strings.Builder
	cost := GetCost(ctx, db)
	budget := MaxMonthUSD()
	rows, err := Unbriefed(ctx, db, BriefMinScore, maxItems)
	if err != nil {
		run.Log = "unbriefed: " + err.Error()
		insertRun(ctx, db, run)
		return run
	}
	if len(rows) == 0 {
		run.Log = "nothing to brief"
		insertRun(ctx, db, run)
		return run
	}
	spent := 0.0
	for _, r := range rows {
		if ctx.Err() != nil {
			break
		}
		if cost.MonthUSD+spent >= budget {
			fmt.Fprintf(&logb, "budget reached (%s >= %s), %d left unbriefed\n", usd(cost.MonthUSD+spent), usd(budget), len(rows)-int(run.Ranked))
			break
		}
		text, src := pageText(ctx, r)
		var sb strings.Builder
		fmt.Fprintf(&sb, "TITLE: %s\nORG: %s\nLOCATION: %s\nPOSTED: %s\nDEADLINE: %s\nSOURCE: %s\nURL: %s\nRANKER VERDICT: score %d, %s\n\nPAGE TEXT (%s):\n%s\n",
			r.Title, r.Org, r.Location, r.Posted, r.Deadline, r.Source, r.URL, r.ScoreVal(), r.Why, src, text)
		out, in, nOut, err := chat(ctx, briefPrompt, sb.String(), 1400)
		c := costUSD(in, nOut)
		run.InTokens += in
		run.OutTokens += nOut
		run.CostUSD += c
		spent += c
		if err != nil {
			slog.Warn("jobs brief", "id", r.ID, "error", err)
			fmt.Fprintf(&logb, "✗ #%d %.60s: %v\n", r.ID, r.Title, err)
			continue
		}
		brief := normalizeBrief(out)
		if len(brief) > 900 {
			brief = truncate(brief, 900)
		}
		if _, err := db.ExecContext(ctx, `UPDATE job_postings SET brief = ?, briefed_at = datetime('now') WHERE id = ?`, brief, r.ID); err != nil {
			fmt.Fprintf(&logb, "✗ #%d store: %v\n", r.ID, err)
			continue
		}
		run.Ranked++
		fmt.Fprintf(&logb, "✓ #%d %.60s (%s, %d in / %d out, %s)\n", r.ID, r.Title, src, in, nOut, usd(c))
	}
	run.Log = logb.String()
	if err := insertRun(ctx, db, run); err != nil {
		slog.Warn("jobs insert brief run", "error", err)
	}
	return run
}

// pageText fetches the posting page as readable text: r.jina.ai first (renders
// JS, strips chrome), raw HTML stripped as fallback, stored snippet last.
func pageText(ctx context.Context, r Row) (text, src string) {
	const max = 6000
	if r.URL != "" {
		fctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		if strings.Contains(r.URL, "linkedin.com/jobs/view/") {
			// Public job pages embed the full description as JSON-LD; the
			// reader proxy only sees the logged-out shell.
			if b, err := get(fctx, r.URL); err == nil {
				if m := liJSONDescRe.FindSubmatch(b); m != nil {
					var d string
					if json.Unmarshal(append(append([]byte{'"'}, m[1]...), '"'), &d) == nil {
						if t := clean(html.UnescapeString(d)); len(t) > 100 {
							return truncate(t, max), "fetched linkedin"
						}
					}
				}
			}
			time.Sleep(6 * time.Second)
		}
		if b, err := get(fctx, "https://r.jina.ai/"+r.URL); err == nil && len(b) > 200 {
			return truncate(compactText(string(b)), max), "fetched via reader"
		}
		if b, err := get(fctx, r.URL); err == nil {
			if t := clean(string(b)); len(t) > 200 {
				return truncate(t, max), "fetched html"
			}
		}
	}
	if s := strings.TrimSpace(r.Snippet); s != "" {
		return truncate(s, max), "stored snippet only"
	}
	return "(none)", "missing"
}

// compactText collapses the markdown-ish reader output: drops link targets,
// images and blank runs so the token budget goes to actual content.
func compactText(s string) string {
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "![") || strings.HasPrefix(ln, "URL Source:") || strings.HasPrefix(ln, "Markdown Content:") {
			continue
		}
		ln = mdLinkRe.ReplaceAllString(ln, "$1")
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}

var briefLabels = []string{"What", "Terms", "Duties", "Fit"}

// normalizeBrief keeps only the four labelled lines, one per line, in order.
func normalizeBrief(out string) string {
	got := map[string]string{}
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(strings.TrimLeft(ln, "-*• "))
		for _, l := range briefLabels {
			if len(ln) > len(l)+1 && strings.EqualFold(ln[:len(l)], l) && ln[len(l)] == ':' {
				got[l] = strings.TrimSpace(ln[len(l)+1:])
			}
		}
	}
	if len(got) == 0 {
		return strings.TrimSpace(strings.Trim(strings.TrimSpace(out), `"`))
	}
	var lines []string
	for _, l := range briefLabels {
		if v := got[l]; v != "" {
			lines = append(lines, l+": "+v)
		}
	}
	return strings.Join(lines, "\n")
}

// BriefItem is one labelled line of a brief.
type BriefItem struct{ Label, Text string }

// BriefItems splits a stored brief into labelled items; empty when the brief
// is a plain paragraph (old format) or missing.
func (r Row) BriefItems() []BriefItem {
	var out []BriefItem
	for _, ln := range strings.Split(r.Brief, "\n") {
		if i := strings.IndexByte(ln, ':'); i > 0 && i < 12 {
			out = append(out, BriefItem{strings.TrimSpace(ln[:i]), strings.TrimSpace(ln[i+1:])})
		}
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

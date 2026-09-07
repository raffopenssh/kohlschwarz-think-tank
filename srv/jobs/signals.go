package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Hiring-difficulty signals — "has the recruiter found anyone yet?"
//
// Everything here is heuristic and LLM-free. Inputs:
//   - listing metadata across daily fetches (Upsert → job_events):
//     deadline moved, posted date moved, reappeared after absence, reopened;
//   - a periodic page re-check (CheckPending): closed markers / 404, LinkedIn
//     applicant count, salary text, JSON-LD validThrough / datePosted;
//   - the dedupe group: how many boards carry it, whether a fresh copy
//     appeared weeks after the first one.
//
// Output is Row.Signals(): a small set of tags with a plain-English reason,
// and a one-word verdict (hard to fill / long open / closed / gone / fresh).

// Signal is one hiring-difficulty indicator shown as a tag.
type Signal struct {
	Key   string // css/filter key: long | extended | readvertised | overdue | fewapps | salaryup | closed | gone | widespread | reopened
	Label string // short tag text
	Title string // tooltip / email explanation
	Hard  bool   // counts towards the "hard to fill" verdict
}

// Signals derives the indicators for a (deduped) row.
func (r Row) Signals() []Signal {
	var out []Signal
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	add := func(key, label, title string, hard bool) {
		out = append(out, Signal{key, label, title, hard})
	}

	// Terminal states first.
	if r.ClosedAt != nil {
		add("closed", "closed", "Page says applications are closed or the ad is gone (checked "+(*r.ClosedAt)[:10]+") — filled or withdrawn.", false)
		return out
	}
	if t, err := time.Parse("2006-01-02 15:04:05", r.LastSeen); err == nil && now.Sub(t) > 5*24*time.Hour {
		add("gone", "not listed since "+r.LastSeen[:10], "Dropped from every source it was found on — most likely filled or withdrawn.", false)
		return out
	}

	// Hiring signals only make sense for actual vacancies that made the list.
	if r.ScoreVal() < BriefMinScore || r.Kind == "other" || r.Kind == "news" {
		return out
	}
	age := r.AgeDays()
	// Change history.
	ext, short, rep, reapp, reopen, salUp, salChg := 0, 0, 0, 0, 0, 0, 0
	var lastExt Event
	for _, e := range r.Events {
		switch e.Kind {
		case "deadline_extended":
			ext++
			lastExt = e
		case "deadline_shortened":
			short++
		case "reposted", "readvertised":
			// LinkedIn shifts datePosted by a few days when it re-indexes an
			// ad; only a jump of 14+ days is a real re-advertisement.
			if daysBetween(e.Old, e.New) >= 14 || e.Old == "" {
				rep++
			}
		case "reappeared":
			reapp++
		case "reopened":
			reopen++
		case "salary_up":
			salUp++
		case "salary":
			salChg++
		}
	}
	if ext > 0 {
		lbl := "deadline extended"
		if d := daysBetween(lastExt.Old, lastExt.New); d > 0 {
			lbl += fmt.Sprintf(" +%dd", d)
		}
		if ext > 1 {
			lbl += fmt.Sprintf(" ×%d", ext)
		}
		add("extended", lbl, fmt.Sprintf("Closing date moved from %s to %s — the recruiter did not fill it in the first round.", lastExt.Old, lastExt.New), true)
	}
	if rep > 0 || reapp > 0 || reopen > 0 {
		title := "The source shows a newer posted date, or the ad came back after disappearing — re-advertised."
		if reopen > 0 {
			title = "Listed again after the page had said closed — re-opened."
		}
		add("readvertised", "re-advertised", title, true)
	} else if gap := r.RepostGapDays(); gap >= 21 {
		add("readvertised", "re-advertised", fmt.Sprintf("A fresh copy of this job appeared %d days after the first one — re-advertised on another board or with a new id.", gap), true)
	}
	if d := daysBetween(r.Deadline, today); r.Deadline != "" && d >= 3 {
		add("overdue", fmt.Sprintf("deadline passed %dd ago, still listed", d), "The stated closing date is in the past but the ad is still online — often means no appointment yet.", true)
	}
	if r.Applicants != nil && age >= 10 {
		n := *r.Applicants
		lbl, title := "", ""
		switch {
		case n == 0:
			lbl, title = "<25 applicants", "LinkedIn shows “be among the first 25 applicants”"
		case n < 40:
			lbl, title = fmt.Sprintf("%d applicants", n), fmt.Sprintf("LinkedIn shows %d applicants", n)
		}
		if lbl != "" {
			add("fewapps", lbl, title+fmt.Sprintf(" after %d days — thin field.", age), n < 25)
		} else if n >= 150 {
			add("manyapps", fmt.Sprintf("%d applicants", n), fmt.Sprintf("LinkedIn shows %d applicants — crowded field.", n), false)
		}
	}
	if salUp > 0 {
		add("salaryup", "salary raised", "The advertised pay went up between checks — sweetening the offer.", true)
	} else if salChg > 0 {
		add("salaryup", "terms changed", "The advertised pay/grade text changed between checks.", false)
	}
	switch {
	case age >= 60:
		add("long", fmt.Sprintf("open %dd", age), fmt.Sprintf("Advertised for %d days and still live — well beyond a normal 4–6 week round.", age), true)
	case age >= 40:
		add("long", fmt.Sprintf("open %dd", age), fmt.Sprintf("Advertised for %d days and still live.", age), false)
	}
	if r.Dupes >= 3 {
		add("widespread", fmt.Sprintf("on %d boards", r.Dupes+1), "Pushed to many boards at once — recruiter is casting a wide net.", false)
	}
	return out
}

// Verdict summarises Signals as one word for the card ribbon / filter.
// "" means nothing notable.
func (r Row) Verdict() string {
	hard := 0
	for _, s := range r.Signals() {
		switch s.Key {
		case "closed":
			return "closed"
		case "gone":
			return "gone"
		}
		if s.Hard {
			hard++
		}
	}
	if hard >= 1 {
		return "hard to fill"
	}
	return ""
}

// HardReasons joins the hard-signal tooltips (for the verdict title / email).
func (r Row) HardReasons() string {
	var parts []string
	for _, s := range r.Signals() {
		if s.Hard {
			parts = append(parts, s.Title)
		}
	}
	return strings.Join(parts, " ")
}

// AgeDays is days since the posted date (or first sighting).
func (r Row) AgeDays() int {
	from := r.Posted
	if from == "" && len(r.FirstSeen) >= 10 {
		from = r.FirstSeen[:10]
	}
	if t, err := time.Parse("2006-01-02", from); err == nil {
		return int(time.Since(t).Hours() / 24)
	}
	return 0
}

// DeadlineIn renders days until the deadline ("in 12 d", "today", "" if none/past).
func (r Row) DeadlineIn() string {
	if r.Deadline == "" {
		return ""
	}
	d := daysBetween(time.Now().UTC().Format("2006-01-02"), r.Deadline)
	switch {
	case d < 0:
		return ""
	case d == 0:
		return "today"
	default:
		return fmt.Sprintf("in %d d", d)
	}
}

// RepostGapDays: days between the earliest and the latest first_seen inside a
// dedupe group (0 for a single row). A large gap means a new copy surfaced long
// after the original — re-advertised.
func (r Row) RepostGapDays() int {
	if r.latestFirstSeen == "" || r.FirstSeen == "" {
		return 0
	}
	return daysBetween(r.FirstSeen[:10], r.latestFirstSeen[:10])
}

func daysBetween(a, b string) int {
	ta, e1 := time.Parse("2006-01-02", a)
	tb, e2 := time.Parse("2006-01-02", b)
	if e1 != nil || e2 != nil {
		return 0
	}
	return int(tb.Sub(ta).Hours() / 24)
}

// --- page re-check --------------------------------------------------------------

var (
	closedRe     = regexp.MustCompile(`(?i)(no longer accepting applications|this job is no longer available|job (has )?expired|position has been filled|vacancy (is )?closed|applications? (are |is )?closed|this (position|vacancy|job) (has been|is) (closed|filled|withdrawn)|Bewerbungsfrist (ist )?abgelaufen|Stelle (ist |wurde )?(bereits )?(besetzt|vergeben)|nicht mehr verfügbar|Diese Stellenanzeige ist (nicht mehr|abgelaufen)|offre (n'est plus|expirée)|poste (a été )?pourvu|candidatures? clos)`)
	applicantsRe = regexp.MustCompile(`(?i)(\d{1,4})\s+applicants|be among the first (\d{1,3}) applicants`)
	salaryRe     = regexp.MustCompile(`(?i)(?:€|EUR|USD|US\$|\$|£|GBP|CHF|ZAR|XAF|XOF|KES|TZS|UGX)\s?\d{1,3}(?:[.,\s]\d{3})+(?:[.,]\d{1,2})?(?:\s?(?:-|–|to|bis|à)\s?(?:€|EUR|USD|\$|£)?\s?\d{1,3}(?:[.,\s]\d{3})+)?|\d{1,3}(?:[.,]\d{3})+(?:[.,]\d{2})?\s?(?:€|EUR|USD|£|GBP|CHF|ZAR)|(?:Gehalt|Entlohnung|Bruttogehalt|Mindestgehalt|Entgelt|salary|salaire|rémunération|grade|Gehaltsstufe|Verwendungsgruppe|Entlohnungsgruppe|Dienstklasse)[^.\n]{0,60}?\d[\d.,]{2,}`)
	jsonLDRe     = regexp.MustCompile(`"(validThrough|datePosted)":"(\d{4}-\d{2}-\d{2})`)
	baseSalaryRe = regexp.MustCompile(`"baseSalary":\{.*?"minValue":([\d.]+).*?"maxValue":([\d.]+).*?"unitText":"(\w+)"`)
	numRe        = regexp.MustCompile(`\d[\d.,]*`)
	thousandsSp  = regexp.MustCompile(`(\d)\s(\d{3})\b`)
)

// CheckInterval: how often a live, ranked posting's page is re-read.
const CheckInterval = 3 * 24 * time.Hour

// CheckPending re-reads the pages of ranked (score ≥ BriefMinScore), visible,
// live postings that were not checked for CheckInterval and records what it
// finds (closed marker, applicant count, salary text, JSON-LD dates). No LLM.
func CheckPending(ctx context.Context, db *sql.DB, maxItems int) Run {
	run := Run{Started: time.Now().UTC().Format("2006-01-02 15:04:05"), Kind: "check"}
	var logb strings.Builder
	rows, err := db.QueryContext(ctx, `SELECT `+rowCols+` FROM job_postings
		WHERE score >= ? AND hidden = 0 AND closed_at IS NULL
		  AND (checked_at IS NULL OR checked_at < datetime('now', ?))
		ORDER BY score DESC, first_seen DESC LIMIT ?`, BriefMinScore, fmt.Sprintf("-%d hours", int(CheckInterval.Hours())), maxItems)
	if err != nil {
		run.Log = "select: " + err.Error()
		insertRun(ctx, db, run)
		return run
	}
	list, err := scanRows(rows)
	if err != nil {
		run.Log = "scan: " + err.Error()
		insertRun(ctx, db, run)
		return run
	}
	if len(list) == 0 {
		run.Log = "nothing to check"
		insertRun(ctx, db, run)
		return run
	}
	for _, r := range list {
		if ctx.Err() != nil {
			break
		}
		res := checkPage(ctx, r)
		run.Found++
		applySignals(ctx, db, r, res, &logb)
		if strings.Contains(r.URL, "linkedin.com") {
			time.Sleep(6 * time.Second)
		} else {
			time.Sleep(800 * time.Millisecond)
		}
	}
	run.Log = logb.String()
	if run.Log == "" {
		run.Log = fmt.Sprintf("%d pages re-checked, nothing changed", run.Found)
	}
	insertRun(ctx, db, run)
	return run
}

type pageCheck struct {
	err        error
	gone       bool // HTTP 404/410 or closed marker
	applicants *int64
	salary     string
	validThru  string
	datePosted string
}

func checkPage(ctx context.Context, r Row) pageCheck {
	var pc pageCheck
	fctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	b, err := get(fctx, r.URL)
	if err != nil {
		if strings.Contains(err.Error(), "HTTP 404") || strings.Contains(err.Error(), "HTTP 410") {
			pc.gone = true
			return pc
		}
		pc.err = err
		return pc
	}
	raw := string(b)
	// LinkedIn embeds the description as JSON-escaped HTML: decode, then strip.
	text := clean(html.UnescapeString(clean(strings.NewReplacer(`\u003c`, "<", `\u003e`, ">", `\"`, `"`, `\n`, " ").Replace(raw))))
	if strings.Contains(r.URL, "linkedin.com/jobs/view/") {
		if m := applicantsRe.FindStringSubmatch(text); m != nil {
			n := int64(0)
			if m[1] != "" {
				n, _ = strconv.ParseInt(m[1], 10, 64)
			}
			pc.applicants = &n
		}
		if m := baseSalaryRe.FindStringSubmatch(raw); m != nil {
			pc.salary = fmt.Sprintf("%s–%s / %s", m[1], m[2], strings.ToLower(m[3]))
		}
	}
	if closedRe.MatchString(text) {
		pc.gone = true
	}
	li := strings.Contains(r.URL, "linkedin.com")
	for _, m := range jsonLDRe.FindAllStringSubmatch(raw, -1) {
		if m[1] == "validThrough" {
			if li {
				continue // LinkedIn's validThrough is a synthetic posted+180d, not a closing date
			}
			pc.validThru = m[2]
		} else {
			pc.datePosted = m[2]
		}
	}
	if pc.salary == "" {
		if m := salaryRe.FindString(text); m != "" {
			pc.salary = truncate(strings.Join(strings.Fields(m), " "), 90)
		}
	}
	return pc
}

// applySignals stores the check result and emits events for what changed.
func applySignals(ctx context.Context, db *sql.DB, r Row, pc pageCheck, logb *strings.Builder) {
	if pc.err != nil {
		// Unreachable page is not evidence of anything; try again next round.
		db.ExecContext(ctx, `UPDATE job_postings SET checked_at = datetime('now') WHERE id = ?`, r.ID)
		fmt.Fprintf(logb, "? #%d %.50s: %v\n", r.ID, r.Title, pc.err)
		return
	}
	if pc.gone {
		db.ExecContext(ctx, `UPDATE job_postings SET checked_at = datetime('now'), closed_at = datetime('now') WHERE id = ?`, r.ID)
		AddEvent(ctx, db, r.ID, "closed", "", "")
		fmt.Fprintf(logb, "✗ #%d %.50s: closed / gone\n", r.ID, r.Title)
		return
	}
	sets := []string{"checked_at = datetime('now')"}
	var args []any
	if pc.applicants != nil {
		if r.Applicants == nil || *r.Applicants != *pc.applicants {
			old := ""
			if r.Applicants != nil {
				old = strconv.FormatInt(*r.Applicants, 10)
			}
			AddEvent(ctx, db, r.ID, "applicants", old, strconv.FormatInt(*pc.applicants, 10))
		}
		sets = append(sets, "applicants = ?")
		args = append(args, *pc.applicants)
	}
	if pc.salary != "" && pc.salary != r.Salary {
		if r.Salary != "" {
			kind := "salary"
			if maxNum(pc.salary) > maxNum(r.Salary)*1.02 {
				kind = "salary_up"
			}
			AddEvent(ctx, db, r.ID, kind, r.Salary, pc.salary)
			fmt.Fprintf(logb, "€ #%d %.50s: %s → %s\n", r.ID, r.Title, r.Salary, pc.salary)
		}
		sets = append(sets, "salary = ?")
		args = append(args, pc.salary)
	}
	if pc.validThru != "" && pc.validThru != r.Deadline {
		if r.Deadline != "" {
			kind := "deadline_extended"
			if pc.validThru < r.Deadline {
				kind = "deadline_shortened"
			}
			AddEvent(ctx, db, r.ID, kind, r.Deadline, pc.validThru)
			fmt.Fprintf(logb, "⏱ #%d %.50s: deadline %s → %s\n", r.ID, r.Title, r.Deadline, pc.validThru)
		}
		sets = append(sets, "deadline = ?")
		args = append(args, pc.validThru)
	}
	if pc.datePosted != "" && r.Posted != "" && pc.datePosted > r.Posted {
		AddEvent(ctx, db, r.ID, "reposted", r.Posted, pc.datePosted)
		sets = append(sets, "posted = ?", "reposted = 1")
		args = append(args, pc.datePosted)
		fmt.Fprintf(logb, "↻ #%d %.50s: reposted %s → %s\n", r.ID, r.Title, r.Posted, pc.datePosted)
	}
	args = append(args, r.ID)
	if _, err := db.ExecContext(ctx, `UPDATE job_postings SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		slog.Warn("jobs check store", "id", r.ID, "error", err)
	}
}

// maxNum extracts the largest number in a salary string (handles 1.234,56 / 1,234.56 / 1 234).
func maxNum(s string) float64 {
	best := 0.0
	s = thousandsSp.ReplaceAllString(thousandsSp.ReplaceAllString(s, "$1$2"), "$1$2")
	for _, m := range numRe.FindAllString(s, -1) {
		m = strings.TrimRight(m, ".,")
		// Decide decimal separator: the last separator followed by exactly 2 digits.
		if i := strings.LastIndexAny(m, ".,"); i >= 0 && len(m)-i-1 == 2 {
			m = strings.NewReplacer(".", "", ",", "").Replace(m[:i]) + "." + m[i+1:]
		} else {
			m = strings.NewReplacer(".", "", ",", "").Replace(m)
		}
		if v, err := strconv.ParseFloat(m, 64); err == nil && v > best {
			best = v
		}
	}
	return best
}

// EventLabel renders an event for the details panel / email.
func (e Event) Label() string {
	d := e.At
	if len(d) >= 10 {
		d = d[:10]
	}
	switch e.Kind {
	case "deadline_extended":
		return d + " deadline extended " + e.Old + " → " + e.New
	case "deadline_shortened":
		return d + " deadline moved earlier " + e.Old + " → " + e.New
	case "reposted":
		return d + " re-posted (dated " + e.New + ", was " + e.Old + ")"
	case "reappeared":
		return d + " back in listings (last seen " + e.Old + ")"
	case "reopened":
		return d + " listed again after closing"
	case "closed":
		return d + " page reports closed / removed"
	case "applicants":
		if e.Old == "" {
			return d + " " + appsText(e.New) + " applicants"
		}
		return d + " applicants " + appsText(e.Old) + " → " + appsText(e.New)
	case "salary_up":
		return d + " pay raised: " + e.Old + " → " + e.New
	case "salary":
		return d + " terms changed: " + e.Old + " → " + e.New
	}
	return d + " " + e.Kind
}

func appsText(s string) string {
	if s == "0" {
		return "<25"
	}
	return s
}

// marshal helper kept for status.json consumers that want signals.
func (r Row) SignalsJSON() string {
	b, _ := json.Marshal(r.Signals())
	return string(b)
}

// CheckedDate renders CheckedAt as a date ("" if never checked).
func (r Row) CheckedDate() string {
	if r.CheckedAt != nil && len(*r.CheckedAt) >= 10 {
		return (*r.CheckedAt)[:10]
	}
	return ""
}

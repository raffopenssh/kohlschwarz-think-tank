package jobs

import (
	"testing"
	"time"
)

func ago(d int) string { return time.Now().UTC().AddDate(0, 0, -d).Format("2006-01-02 15:04:05") }
func day(d int) string { return time.Now().UTC().AddDate(0, 0, d).Format("2006-01-02") }

func keys(r Row) map[string]bool {
	m := map[string]bool{}
	for _, s := range r.Signals() {
		m[s.Key] = true
	}
	return m
}

func TestSignals(t *testing.T) {
	// fresh posting: nothing
	if k := keys(Row{FirstSeen: ago(3), LastSeen: ago(0)}); len(k) != 0 {
		t.Errorf("fresh: %v", k)
	}
	// extended deadline + overdue → hard to fill
	r := Row{FirstSeen: ago(50), LastSeen: ago(0), Deadline: day(-4), Events: []Event{{Kind: "deadline_extended", Old: day(-30), New: day(-4)}}}
	k := keys(r)
	if !k["extended"] || !k["overdue"] || !k["long"] || r.Verdict() != "hard to fill" {
		t.Errorf("extended: %v verdict=%q", k, r.Verdict())
	}
	// gone from sources
	r = Row{FirstSeen: ago(30), LastSeen: ago(9)}
	if r.Verdict() != "gone" {
		t.Errorf("gone: %q", r.Verdict())
	}
	// closed wins over everything
	c := ago(1)
	r = Row{FirstSeen: ago(90), LastSeen: ago(0), ClosedAt: &c, Deadline: day(-10)}
	if r.Verdict() != "closed" || len(r.Signals()) != 1 {
		t.Errorf("closed: %v", r.Signals())
	}
	// thin field on linkedin after 10 days
	n := int64(0)
	r = Row{FirstSeen: ago(12), LastSeen: ago(0), Applicants: &n}
	if k := keys(r); !k["fewapps"] || r.Verdict() != "hard to fill" {
		t.Errorf("fewapps: %v", k)
	}
	// but not on day 2
	r = Row{FirstSeen: ago(2), LastSeen: ago(0), Applicants: &n}
	if k := keys(r); k["fewapps"] {
		t.Errorf("fewapps too early: %v", k)
	}
	// repost gap via dedupe: copy surfaced 25 days after original
	rows := Dedupe([]Row{{URL: "https://a/1", Org: "X", Title: "Park Director", FirstSeen: ago(30), LastSeen: ago(0)}, {URL: "https://b/2", Org: "X", Title: "Park Director", FirstSeen: ago(5), LastSeen: ago(0)}})
	if len(rows) != 1 || rows[0].RepostGapDays() != 25 || !keys(rows[0])["readvertised"] {
		t.Errorf("repost gap: %d %v", rows[0].RepostGapDays(), keys(rows[0]))
	}
}

func TestMaxNum(t *testing.T) {
	for in, want := range map[string]float64{"€ 4.500,50 brutto": 4500.5, "$85,000 - $95,000": 95000, "EUR 60 000": 60000, "grade P5": 5} {
		if got := maxNum(in); got != want {
			t.Errorf("maxNum(%q)=%v want %v", in, got, want)
		}
	}
}

func TestClosedRe(t *testing.T) {
	for _, s := range []string{"No longer accepting applications", "Die Bewerbungsfrist ist abgelaufen", "Cette offre n'est plus disponible", "This position has been filled"} {
		if !closedRe.MatchString(s) {
			t.Errorf("closedRe miss: %q", s)
		}
	}
	if closedRe.MatchString("Applications close 30 September") {
		t.Error("closedRe false positive")
	}
}

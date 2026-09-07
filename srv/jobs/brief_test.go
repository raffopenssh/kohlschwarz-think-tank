package jobs

import (
	"strings"
	"testing"
)

func TestNormalizeBrief(t *testing.T) {
	out := normalizeBrief("Sure.\n- What: permanent post, Noé\n* Terms: no deadline\nDuties: run the park\nFit: matches A\nExtra: junk")
	want := "What: permanent post, Noé\nTerms: no deadline\nDuties: run the park\nFit: matches A"
	if out != want {
		t.Fatalf("got %q", out)
	}
	r := Row{Brief: out}
	if it := r.BriefItems(); len(it) != 4 || it[3].Label != "Fit" {
		t.Fatalf("items %+v", it)
	}
	if got := normalizeBrief("plain paragraph"); got != "plain paragraph" {
		t.Fatalf("fallback %q", got)
	}
}

func TestWritePickBrief(t *testing.T) {
	sc := int64(95)
	var b strings.Builder
	writePick(&b, 1, Row{Title: "Directeur PNCD", Org: "Noé", Score: &sc, Region: "ssa", Kind: "director", Why: "SSA park director", URL: "https://x", Brief: "What: permanent post\nFit: matches A"})
	s := b.String()
	for _, want := range []string{"   What:   permanent post", "   Fit:    matches A", "fit 95/100"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in\n%s", want, s)
		}
	}
}

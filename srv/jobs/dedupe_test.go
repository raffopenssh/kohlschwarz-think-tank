package jobs

import "testing"

func TestDedupe(t *testing.T) {
	s := func(v int64) *int64 { return &v }
	rows := []Row{
		{ID: 1, Source: "Noé · nous rejoindre", Title: "Directeur.trice du Parc National de Conkouati-Douli (PNCD)", URL: "https://noe.org/a", Score: s(95), FirstSeen: "2026-09-01 00:00:00"},
		{ID: 2, Source: "Noé · nous rejoindre", Title: "Conkouati-Douli Park Manager", URL: "https://noe.org/b", Score: s(95), FirstSeen: "2026-08-20 00:00:00", Deadline: "2026-10-01"},
		{ID: 3, Source: "LinkedIn · CI", Org: "Conservation International", Title: "Senior Director, KPA Resource Mobilization", Location: "Kenya", URL: "https://ke.linkedin.com/jobs/view/x-4454403176", Score: s(15)},
		{ID: 4, Source: "LinkedIn · CI", Org: "Conservation International", Title: "Senior Director, KPA Resource Mobilization", Location: "Nairobi", URL: "https://ke.linkedin.com/jobs/view/x-4455895309", Score: s(15)},
		{ID: 5, Source: "LinkedIn · CI", Org: "Conservation International", Title: "Senior Manager, Ocean Policy", URL: "https://cr.linkedin.com/jobs/view/y-4439153466", Score: s(15)},
		{ID: 6, Source: "LinkedIn · X", Org: "Conservation International", Title: "Senior Manager, Ocean Policy", URL: "https://gy.linkedin.com/jobs/view/y-4439153466?trk=1", Score: s(10)},
		{ID: 7, Source: "LinkedIn · Ramboll", Org: "Ramboll", Title: "Senior Consultant (m/w/d) Corporate Biodiversity Strategy", URL: "https://de.linkedin.com/jobs/view/1", Score: s(10)},
		{ID: 8, Source: "LinkedIn · Ramboll", Org: "Ramboll", Title: "Senior Consultant Corporate Biodiversity Strategy", URL: "https://de.linkedin.com/jobs/view/2", Score: s(10)},
		{ID: 9, Source: "Noé · nous rejoindre", Title: "Expertise scientifique sur la conservation des espèces", URL: "https://noe.org/c", Score: s(10)},
		{ID: 10, Source: "TNC", Org: "The Nature Conservancy", Title: "Utah State Director", URL: "https://l/1", Score: s(40)},
		{ID: 11, Source: "TNC", Org: "The Nature Conservancy", Title: "New Mexico State Director", URL: "https://l/2", Score: s(40)},
	}
	out := Dedupe(rows)
	ids := map[int64]Row{}
	for _, r := range out {
		ids[r.ID] = r
	}
	if len(out) != 7 {
		for _, r := range out {
			t.Logf("%d %q dupes=%d", r.ID, r.Title, r.Dupes)
		}
		t.Fatalf("want 7 groups, got %d", len(out))
	}
	if r := ids[1]; r.Dupes != 1 || r.FirstSeen != "2026-08-20 00:00:00" || r.Deadline != "2026-10-01" {
		t.Errorf("noe merge wrong: %+v", r)
	}
	if ids[3].Dupes != 1 || ids[5].Dupes != 1 || ids[7].Dupes != 1 {
		t.Errorf("linkedin merges wrong: %+v %+v %+v", ids[3], ids[5], ids[7])
	}
	if _, ok := ids[10]; !ok {
		t.Error("utah lost")
	}
	if _, ok := ids[11]; !ok {
		t.Error("new mexico wrongly merged")
	}
}

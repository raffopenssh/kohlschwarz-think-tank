package jobs

import "testing"

func TestAustrianPathwayMatch(t *testing.T) {
	at := func(title, org, snippet string) Posting {
		return Posting{Title: title, Org: org, Snippet: snippet, Region: "austria", Source: "Land X"}
	}
	yes := []Posting{
		at("Referent/in Naturschutz", "Amt der Oö. Landesregierung · Abteilung Naturschutz", "[Land OÖ] Referent/in · Direktion Umwelt und Wasserwirtschaft"),
		at("Amtssachverständige/r für Naturschutz", "Amt der Steiermärkischen Landesregierung", "[Land Steiermark] Dienstort: Graz · Abteilung 13 Umwelt und Raumordnung"),
		at("Juristin bzw. Jurist", "Amt der NÖ Landesregierung", "[Land NÖ] Juristin bzw. Jurist · Abteilung Naturschutz (RU5) · Naturschutzrecht, Natura 2000 Verfahren"),
		at("Fachbereichsleitung Alm und Weide", "Amt der NÖ Landesregierung", "[Land NÖ] Fachbereichsleitung Alm und Weide"),
		at("Leiter:in der Abteilung Agrar-, Umwelt- und Ernährungssysteme", "Bundesministerium für Land- und Forstwirtschaft, Klima- und Umweltschutz", "[Bund] BML · Natur/Landwirtschaft/Technik"),
		at("Eine Planstelle im „Höheren Dienst“ im Bereich Naturschutz", "Amt der Kärntner Landesregierung", "[Land Kärnten] Abteilung 8 – Umwelt, Energie und Naturschutz"),
		at("Natura 2000-Gebietsbetreuung", "Land Burgenland", "[Land Burgenland] Natura 2000-Gebietsbetreuung"),
		at("Revierleiter (w/m/d)", "Österreichische Bundesforste", "[Österreichische Bundesforste] Forstbetrieb Steyrtal · Nationalpark Kalkalpen"),
	}
	no := []Posting{
		at("Reinigungskraft", "Amt der Oö. Landesregierung", "[Land OÖ] Reinigungskraft · Direktion Umwelt und Wasserwirtschaft"),
		at("Verwaltungspraktikum in der Abteilung IV/7 – Siedlungswasserwirtschaft", "BML", "[Bund] BML · Natur/Landwirtschaft/Technik"),
		at("Nationalpark-Ranger/in (Saison)", "Nationalpark Gesäuse GmbH", "[Nationalpark] Ranger Saison 2027"),
		at("Sachbearbeiter/in Preisauszeichnung und Produktsicherheit", "Amt der Oö. Landesregierung", "[Land OÖ] Sachbearbeitung · Direktion für Landesplanung, wirtschaftliche und ländliche Entwicklung"),
		at("Luftmesstechniker*in im Bereich Luftreinhaltung", "Stadt Wien · MA 22 - Umweltschutz", "[Stadt Wien] MA 22 - Umweltschutz · Aufgaben: Leitung von Messkampagnen"),
		at("Leiter/in der Gruppe Grafik- und Webservice", "Amt der Oö. Landesregierung", "[Land OÖ] Gruppenleiter/in · Direktion Präsidium · Kommunikation"),
		at("Amtsärztin/Amtsarzt", "Amt der Oö. Landesregierung", "[Land OÖ] Bezirkshauptmannschaften · Medizin & Gesundheit · Umweltmedizin"),
		at("Sekretariatskraft (Karenzvertretung)", "BML", "[Bund] BML · Forstliche Ausbildungsstätte · Wald"),
		at("Bautechniker/in", "Amt der Oö. Landesregierung", "[Land OÖ] Bautechniker/in · Direktion Umwelt und Wasserwirtschaft"),
	}
	for _, p := range yes {
		if !Match(p) {
			t.Errorf("should match: %q (%s)", p.Title, p.Org)
		}
	}
	for _, p := range no {
		if Match(p) {
			t.Errorf("should NOT match: %q (%s)", p.Title, p.Org)
		}
	}
	// Non-Austrian region must not get the relaxed treatment.
	if Match(Posting{Title: "Referent Naturschutz", Org: "Landesregierung", Snippet: "Naturschutz", Region: "global"}) {
		t.Error("relaxed pathway leaked outside austria")
	}
}

func TestISODate(t *testing.T) {
	for in, want := range map[string]string{"27.08.2026": "2026-08-27", "2026-09-11": "2026-09-11", "2026-09-11T00:00:00": "2026-09-11", "": "", "n/a": ""} {
		if got := isoDate(in); got != want {
			t.Errorf("isoDate(%q)=%q want %q", in, got, want)
		}
	}
	if got := sapDate("/Date(1788739200000)/"); got != "2026-09-07" {
		t.Errorf("sapDate=%q", got)
	}
}

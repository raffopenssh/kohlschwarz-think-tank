package funding

// Verified overrides (manual web check 2026-09-03 against official pages).
// Applied on top of Seeds at seed time. Score 0 = drop (ineligible / dead / not worth it).
type override struct {
	Score        int
	Deadline     string // ISO or "" (keep/clear)
	DeadlineNote string
	Why          string
}

var Verified = map[string]override{
	// ---- KEEP: act now ----
	"wa-gruendungsstipendium": {95, "2026-09-15", "window 15 Jul–15 Sep 2026; mandatory Founders Lab + 4 coaching hrs; Hauptwohnsitz Wien ≥6 mo, Meldebestätigung ≤1 mo old", "Exact stage fit, €8k, natural person. SUBMIT NOW."},
	"aws-preseed-innovative":  {90, "", "rolling; jury ~quarterly; info hours 23 Sep & 21 Oct 2026; needs 25-page business plan + pitch deck + work-package plan", "Main pre-company grant (€89k, 80%). Attend 23 Sep info hour, submit Oct/Nov."},
	"inits":                   {80, "2026-11-16", "SCALEup registration open until 16 Nov (one banner says 25 Nov – use 16); intro interview within 7 days; camp spring 2027", "Free Vienna incubator, STARTKapital €100k, wants academic background – fits."},
	"yc":                      {60, "2026-11-02", "W27 on-time deadline 2 Nov 2026 8pm PT; decisions 11 Dec (confirmed)", "Re-apply with traction (users on 5MP, entity, co-founder). Solo + no users = long shot."},
	"mit-solve":               {75, "", "Global Challenges round opens Sep 2026 (official FAQ) with early-stage eligibility; deadline TBA – check weekly", "No entity needed; early-stage OK; conservation/climate tracks."},
	"esa-bic-austria":         {80, "", "permanent call; next cut-off 'to be announced' (expect Oct/Nov 2026)", "Natural persons eligible, €50k zero-equity. Blocker: must locate in Styria or Lower Austria (Vienna not listed) → accent/Wr. Neustadt route."},
	"mbz-species-fund":        {60, "2026-10-15", "window 15 Sep–15 Oct 2026; then 4–31 Jan 2027, 1–31 May 2027", "Individuals eligible, ≤$25k. Only if framed around a specific threatened species (e.g. Chinko lion/leopard monitoring), not a platform."},
	"nasa-space-apps":         {60, "2026-11-14", "hackathon 14–15 Nov 2026 (NOT Oct); Vienna in-person event confirmed; register now", "Free; best venue to find a tech co-founder."},
	"cassini-hackathon":       {60, "2026-11-20", "12th hackathon 'EU Space for Peace & Resilience' 27–29 Nov 2026; registration 28 Sep–20 Nov; check for AT location late Sep", "€5k/3k/1k + 6 months mentoring; individuals OK; theme fits."},
	"african-parks-pilot":     {85, "", "no call – relationship-driven; AP is core partner of Rob Walton Foundation", "Realistic paid multi-park pilot via existing park-manager contacts. Needs invoicing entity eventually."},
	"antler":                  {55, "", "Berlin residency 2×/yr (Mar & Sep); Sep 2026 cohort started 14 Sep → apply for Mar 2027; ~2% acceptance", "Solo founders accepted (co-founder matching). €100k for 8.5% + €100k MFN note. 10 weeks in Berlin."},
	"ms-founders-hub":         {50, "", "rolling", "Most realistic credits programme pre-incorporation."},
	"gcp-startups":            {45, "", "rolling", "$2k pre-funded tier only; Earth Engine covered; $200k needs seed round."},
	"aws-activate":            {40, "", "rolling", "$1k bootstrapped tier only without Activate Provider."},
	"esa-osip":                {40, "", "rolling; ideas from anyone in ESA state; contract needs esa-star entity (e.U. suffices)", "Cheap shot; wants novel R&D not product."},

	// ---- WATCH: not open now, fits ----
	"aws-first-incubator":   {75, "", "2026 call #2 closed 23 Jun; next ~Q1–Q2 2027; company must be <6 months old", "Great fit for 2027 – do NOT found a company too early."},
	"netidee":               {70, "", "call 21 closed 7 Jul 2026; next 'noch nicht fixiert' → expect ~Mar 2027; results must be open source", "€60k for open-source kohlschwarz.at tools."},
	"ffg-impact-innovation": {55, "", "NOT in FFG open list (3 Sep 2026); 2026 closed 12 Mar; next ~Q1 2027", "Individuals eligible; co-design with a Land/park authority."},
	"climatelaunchpad-at":   {60, "", "2026 closed 30 Mar; 2027 intake ~Feb 2027 (Thinkubator Vienna)", "Free pitch training, no entity."},
	"greenstart":            {50, "", "no 2026/27 call published; not in Klimafonds 2026 programme press release – may be paused; if it runs: opens ~Dec, deadline ~Feb", "Only if programme continues."},
	"wildlabs-awards":       {65, "", "2026 EOI closed 18 Mar; 2027 EOI ~Jan–Mar 2027; two-stage; Arm-based hardware/tech mandatory", "$10k tier can go to individuals; need an Arm angle (edge devices in parks)."},
	"oxford-catalyst-grant": {65, "", "2026 closed 15 May; expect 2027 call ~Mar–May (only 2 cycles so far)", "Equity-free £10k+; PGDip alumni should qualify – confirm with OSEC."},
	"oxford-seed-fund":      {50, "", "rolling Oct–Jun; pre-incorporation apply OK, must incorporate before funds; historically UK/US entity", "Solo pre-revenue = weak; after co-founder."},
	"oxford-open-seed-fund": {35, "", "2026-27 rounds ~Nov 2026 / Feb 2027; Oxford researcher must apply, he'd be Policy Partner", "Needs a WildCRU PI to front it."},
	"oui-startup-incubator": {35, "", "rolling; cohorts Oct–Mar; Phase 2 £10k for 5%; must plan UK/US incorporation + Oxford presence", "Marginal."},
	"esa-bass-kickstart":    {55, "2026-10-30", "open call batch cut-off 30 Oct 2026 (needs legal entity + FFG/ALR authorisation); now under ESA ACCESS", "€75k at 75% – reachable only if e.U./GmbH exists by Oct."},
	"ffg-asap":              {50, "", "NOT in FFG open list (3 Sep 2026) – no ASAP/Weltraum call open; check FFG 'geplant' list / newsletter", "Best thematic fit once a call opens; needs entity."},
	"cassini-accelerator":   {30, "2026-10-02", "Batch 8 closes 2 Oct 2026 – ineligible (SME with sales); Batch 9 opens ~mid-Jan 2027", "Only after entity + first revenue."},
	"eic-accelerator":       {25, "2026-11-04", "full-app cut-off 4 Nov 2026; natural persons 'willing to set up an SME' may apply", "TRL6+ and team required; not now."},
	"echoing-green":         {45, "", "typically opens ~Oct/Nov, closes ~Dec/Jan; no entity needed at application", "Possible but not core fit (Vienna founder serving CAR)."},
	"cdl-climate":           {35, "", "2026/27 intake closing/closed; next ~spring–summer 2027; Climate stream in Paris/Vancouver", "2027/28 cycle."},
	"ef":                    {40, "", "rolling; London/Bangalore/SF only (no Paris); 9 months in London", "Only if willing to move to London."},
	"superorganism":         {55, "", "rolling", "Best VC thesis fit; contact once entity + co-founder."},
	"rolex-awards":          {50, "", "biennial; check rolex.org Q1 2027", "Strong personal fit (Chinko), needs active project."},
	"gee-noncommercial":     {50, "", "rolling; quotas enforced since Apr 2026", "OK for pre-commercial R&D only; startup → GCP for Startups."},
	"planet-tfo":            {30, "", "self-serve paid: $180/mo or $1,800/yr non-commercial (NICFI free access ended Jan 2025)", "Not free; cheap data if needed."},
	"rufford":               {25, "", "rolling", "Early-career criterion (≤3 yrs post-degree) – poor fit; paid to org."},
	"ucpm":                  {25, "", "2026 KAPP closed 21 May; 2027 ~Feb–May", "Consortium of authorities; only as partner."},
	"horizon-euspa-2027":    {15, "2027-02-17", "real topic HORIZON-2027-EUSPA-SPACE-51 opens 14 Oct 2026, closes 17 Feb 2027; €0.98m, 1 project, practitioner consortium", "Not a fit."},
	"life-2026":             {10, "2026-09-22", "SAP deadline 22 Sep 2026 – unreachable; 2027 call ~Apr 2027", "Entity + €1m+ project."},
	"life-ngo-consortia":    {40, "", "2027 call opens ~Apr 2027, deadline ~Sep 2027", "Ask WWF/ELC contacts to be SME partner in 2027 proposals."},
	"horizon-cl6-biodiv":    {10, "2026-09-17", "17 Sep 2026 – unreachable", "2027 topics only, as partner."},
	"horizon-cl3-drs":       {10, "2026-11-05", "5 Nov 2026 – consortia already formed", "Not realistic."},
	"eu-mission-adaptation": {10, "2026-09-23", "23 Sep 2026 – unreachable", "2027 WP only."},
	"natgeo-grants":         {20, "", "RFP-only now; current RFP (Okavango, 23 Sep) irrelevant", "Monitor for a tech RFP."},
	"whitley":               {25, "2026-10-31", "31 Oct 2026 – nominate a CAR partner, not yourself", "Indirect."},

	"ffg-eureka-resilienz": {35, "2026-10-26", "FFG open list: Eureka 'Katastrophenresilienz und Wiederaufbau 2026', 10 Jun–26 Oct 2026; ≥2 companies from 2 Eureka countries; max €3m AT share", "Direct thematic hit (flood/fire/drought) – needs entity + foreign partner; watch for 2027 edition."},
	"ffg-oeko-scheck":      {30, "2026-10-05", "FFG open list: Öko-Scheck 2026, 1 Sep–5 Oct 2026; €12k voucher; KMU or gemeinnützige Org", "Only if an entity/Verein exists by early Oct."},
	"ffg-markt-einstieg":   {25, "2026-12-31", "FFG open list: Markt.Einstieg 2026, rolling; max €120k commercialisation; typically after a completed FFG project", "Sequence after Kleinprojekt."},
	"he-cl6-governance":    {20, "2026-11-26", "FFG open list: HORIZON-CL6-2026-04 GOVERNANCE single stage 25 Aug–26 Nov 2026 (two-stage Call 03 closes 30 Sep); EO + digital governance", "Closest Horizon topic – partner-only via TU Wien GEO/BOKU/GeoSphere."},

	// ---- DROP ----
	"biopama-ac":                {0, "", "programme closed at IUCN Congress 2025 – dead", "Dead."},
	"public-io":                 {0, "", "consultancy, not an investor", "Not a funder."},
	"gfw-small-grants":          {0, "", "requires registered non-profit with >$50k budget", "Ineligible."},
	"prince-bernhard":           {0, "", "individuals ineligible; calls only via Conservation Connect", "Ineligible."},
	"clp-award":                 {0, "", "early-career (<5 yrs) + LMIC applicants + team of 3", "Ineligible."},
	"afox":                      {0, "", "Oxford postdoc + African institution researcher required", "Ineligible."},
	"developpp-ventures":        {35, "2026-11-15", "15 Nov 2026 is call OPENING; startup must be registered in CI/GH/NG/ZA/KE/TZ/RW (not CAR) – so pick Kenya or Rwanda for the African entity; needs PoC + first revenue + €100k matching", "Re-opened 7 Sep 2026: African entity is feasible – choose jurisdiction from this list."},
	"climate-change-ai":         {0, "", "invitation-only 2025; PI at OECD university required", "Ineligible."},
	"digital-africa-fuze":       {60, "", "rolling; €20k (idea/MVP) → €30k → €50–100k tiers; company <36 months, ≥1 African national co-founder; francophone tilt (AFD/Proparco) – CAR qualifies; online eligibility check → IC", "Re-opened 7 Sep 2026: African entity is feasible. €20k MVP ticket is reachable within months; francophone CAR is in their sweet spot."},
	"cafi":                      {0, "", "government/UN implementers only", "Only as subcontractor."},
	"legacy-landscapes":         {0, "", "NGO managing whole PA with co-finance", "Not for individuals."},
	"gef-sgp-car":               {0, "", "local registered CSOs only", "Via partner CSO only."},
	"cxl-grand-challenges":      {0, "", "no open challenge (Fire GC concluded Jan 2026)", "Watch conservationxlabs.com/open-innovation."},
	"global-conservation":       {0, "", "no mechanism for tech vendors", "Drop."},
	"rob-walton-foundation":     {0, "", "invitation-only; funds AP/WCS/CI", "Use as intel only – route via African Parks."},
	"ada-business-partnerships": {0, "", "requires EEA legal entity with own funds + creditworthiness", "After GmbH + revenue."},
	"eit-climaccelerator":       {0, "", "no Austrian programme; incorporated startups only", "Drop."},
	"digital-europe-genai-pa":   {0, "", "closed 3 Mar 2026; no follow-up topic", "Dead."},
	"seraphim-space":            {0, "", "applications closed; TRL5+ incorporated spacetech", "Too early."},
	"esa-incubed":               {0, "", "companies with matching funds", "Too early."},
	"ffg-kiras":                 {0, "", "NOT in FFG open list (3 Sep 2026); consortium + Bedarfsträger + entity", "Not now."},
	"ffg-innovationsscheck":     {45, "2026-11-02", "FFG open list: open until 2 Nov 2026; €10k voucher (20% Selbstbehalt) for work bought from TU Wien GEO/BOKU/GeoSphere", "Cheap way to formalise a research partner – needs KMU entity."},
	"ffg-ai-for-green":          {15, "2026-10-06", "no 'AI for Green' call; nearest: FFG 'AI Ökosysteme 2026 – Hybrid AI & Green AI', 18 May–6 Oct 2026, max €1m, consortia", "Partner-only in a TU Wien/BOKU consortium."},
	"ams-ugp":                   {0, "", "only for registered unemployed; excludes Gründungsstipendium; URL dead", "N/A."},
	"uniqa-ventures":            {0, "", "redirects to uniqagroup.com; insurance-strategic", "Drop."},
	"vig":                       {0, "", "URL 404; insurer strategic", "Drop."},
	"drk-foundation":            {0, "", "post-pilot orgs with impact evidence", "Too early."},
	"2150":                      {0, "", "urban-tech, later stage", "Drop."},
	"aws-gruendungsfonds":       {0, "", "GmbH + traction", "Too early."},
	"nvidia-inception":          {20, "", "needs incorporated company + website", "After founding."},
	"esri-startup":              {20, "", "incorporated software company <5 yrs", "After founding."},
	"wa-innovation":             {30, "2026-12-31", "cut-off 31 Dec 2026; needs SME + 55% own financing", "After GmbH."},
	"wa-lebensqualitaet":        {20, "2026-12-31", "needs entity + co-financing", "After GmbH."},
	"ffg-kleinprojekt":          {55, "2026-12-31", "FFG open list 3 Sep 2026: rolling to 31 Dec 2026; max €88,500; startups 'in Gründung' explicitly eligible (entity by contract)", "First FFG step once e.U./GmbH exists – fund the Veridical Earth MVP as R&D."},
	"ffg-basisprogramm":         {25, "2026-12-31", "FFG open list: rolling to 31 Dec 2026; max €3m grant+loan", "Follow-on after Kleinprojekt."},
	"accent":                    {45, "2026-09-29", "next selection ~29 Sep 2026 (unverified); Lower Austria base required – but that is also the ESA BIC route", "Consider together with ESA BIC Austria."},
	"aws-preseed-deeptech":      {35, "", "rolling; needs IP-protectable tech leap + ≥€5m financing plausibility", "aws will steer this to Innovative Solutions."},
}

func applyVerified(e Entry) Entry {
	o, ok := Verified[e.Key]
	if !ok {
		return e
	}
	e.Score = o.Score
	e.Deadline = o.Deadline
	if o.DeadlineNote != "" {
		e.DeadlineNote = o.DeadlineNote
	}
	if o.Why != "" {
		e.Why = o.Why
	}
	e.Note = "[verified 2026-09-03] " + e.Note
	return e
}

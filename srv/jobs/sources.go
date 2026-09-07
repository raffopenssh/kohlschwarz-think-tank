package jobs

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Source describes one place to look for postings.
type Source struct {
	Name   string
	Kind   string // linkedin | rss | ted | undp | page
	URL    string
	Region string // default region hint: austria | ssa | global | eu
	Lang   string // en | de | fr
}

func li(name, keywords, location, region, lang string) Source {
	q := url.Values{}
	q.Set("keywords", keywords)
	if location != "" {
		q.Set("location", location)
	}
	q.Set("f_TPR", "r2592000") // last 30 days
	q.Set("start", "0")
	return Source{Name: name, Kind: "linkedin", Region: region, Lang: lang,
		URL: "https://www.linkedin.com/jobs-guest/jobs/api/seeMoreJobPostings/search?" + q.Encode()}
}

// LinkedInCompanies maps LinkedIn organisation IDs (f_C filter) to names.
// Harvested from https://www.linkedin.com/company/<slug> (urn:li:organization:<id>).
var LinkedInCompanies = map[int]string{
	1341065:  "African Parks Network",
	14344:    "Wildlife Conservation Society",
	11210818: "WCS Europe",
	805644:   "WWF International",
	157338:   "The Nature Conservancy",
	12533:    "Conservation International",
	14035404: "Peace Parks Foundation",
	116377:   "Frankfurt Zoological Society",
	368678:   "African Wildlife Foundation",
	125250:   "Zoological Society of London",
	15210561: "ZSL",
	51247:    "BirdLife International",
	74034991: "Re:Wild",
	105968:   "Panthera",
	13143:    "IUCN",
	124245:   "Wetlands International",
	96471:    "Wildlife Alliance",
	10383711: "Conservation Capital",
	87982825: "Enduring Earth",
	76149649: "Legacy Landscapes Fund",
	3495681:  "Noé",
	1664949:  "GIZ",
	14378:    "KfW",
	162332:   "AFD",
	1861:     "UNDP",
	10451:    "UNEP",
	102249:   "Global Environment Facility",
	53244:    "SANParks",
	9035518:  "Kenya Wildlife Service",
	7041675:  "Tanzania National Parks",
	7087275:  "Uganda Wildlife Authority",
	760192:   "Rwanda Development Board",
	82366060: "Nationalparks Austria",
	1584138:  "Gorongosa National Park",
	35578107: "Grumeti Fund",
	1730525:  "Big Life Foundation",
	55069873: "Tusk Trust",
	6220464:  "Save the Elephants",
	21746:    "IFAW",
	71654:    "WildAid",
	532897:   "Blue Ventures",
	82657629: "Mara Elephant Project",
	17947053: "Lion Landscapes",
	14657246: "Conservation South Africa",
}

// linkedInCompanySources builds one guest-search source per group of ~6 companies
// (all their postings, last 30 days; the keyword prefilter keeps only senior roles).
// bigOrgs post far more than LinkedIn's guest search will page through, so they are
// queried individually with a topic keyword filter.
var bigOrgs = map[int]bool{1861: true, 10451: true, 1664949: true, 14378: true, 162332: true, 157338: true, 805644: true, 12533: true, 14344: true, 13143: true, 102249: true, 21746: true, 125250: true, 15210561: true}

const bigOrgKeywords = `director OR park OR "protected area" OR "team leader" OR "chief of party" OR Nationalpark OR "parc national" OR conservation`

func linkedInCompanySources() []Source {
	ids := make([]int, 0, len(LinkedInCompanies))
	var out []Source
	for id := range LinkedInCompanies {
		if bigOrgs[id] {
			q := url.Values{}
			q.Set("f_C", strconv.Itoa(id))
			q.Set("keywords", bigOrgKeywords)
			q.Set("f_TPR", "r2592000")
			q.Set("location", "Worldwide")
			q.Set("start", "0")
			out = append(out, Source{Name: "LinkedIn · " + LinkedInCompanies[id], Kind: "linkedin", Region: "global", Lang: "en",
				URL: "https://www.linkedin.com/jobs-guest/jobs/api/seeMoreJobPostings/search?" + q.Encode()})
			continue
		}
		ids = append(ids, id)
	}
	sort.Ints(ids)
	const per = 6
	for i := 0; i < len(ids); i += per {
		end := min(i+per, len(ids))
		var idStrs, names []string
		for _, id := range ids[i:end] {
			idStrs = append(idStrs, strconv.Itoa(id))
			names = append(names, LinkedInCompanies[id])
		}
		q := url.Values{}
		q.Set("f_C", strings.Join(idStrs, ","))
		q.Set("f_TPR", "r2592000")
		q.Set("location", "Worldwide")
		q.Set("start", "0")
		out = append(out, Source{Name: "LinkedIn · " + strings.Join(names, ", "), Kind: "linkedin", Region: "global", Lang: "en",
			URL: "https://www.linkedin.com/jobs-guest/jobs/api/seeMoreJobPostings/search?" + q.Encode()})
	}
	return out
}

// Sources is the curated, no-API-key list. Everything here is fetchable with plain HTTP.
var Sources = []Source{
	// --- LinkedIn guest search (no login; EN/DE/FR) ---
	li("LinkedIn · park manager Africa (EN)", "\"park manager\" OR \"park director\" OR \"protected area\" manager", "Africa", "ssa", "en"),
	li("LinkedIn · national park director (EN, Africa)", "\"national park\" director OR \"park warden\" OR \"chief park warden\" OR \"conservation manager\"", "Africa", "ssa", "en"),
	li("LinkedIn · national park CEO / director (EN, Europe)", "\"national park\" director OR \"national park\" CEO OR \"protected area\" director", "European Union", "global", "en"),
	li("LinkedIn · protected area consultant (EN, Africa)", "\"protected area\" consultant OR \"protected areas\" expert OR \"protected area\" team leader", "Africa", "ssa", "en"),
	li("LinkedIn · protected area senior expert (EN, Europe)", "\"protected areas\" senior expert OR \"protected areas\" team leader OR \"protected area\" consultant", "European Union", "global", "en"),
	li("LinkedIn · conservation area / landscape manager Africa", "\"conservation area\" manager OR \"landscape manager\" OR \"reserve manager\" OR \"concession manager\"", "Africa", "ssa", "en"),
	li("LinkedIn · Nationalpark Österreich (DE)", "Nationalpark OR Naturpark OR Biosphärenpark OR Schutzgebiet", "Österreich", "austria", "de"),
	li("LinkedIn · Nationalpark Direktor DACH (DE)", "Nationalpark Direktor OR Nationalparkleitung OR Geschäftsführung Nationalpark OR Schutzgebietsmanagement", "Deutschland", "global", "de"),
	li("LinkedIn · parc national Afrique (FR)", "\"parc national\" OR \"aire protégée\" OR \"aires protégées\" OR conservateur parc", "Afrique", "ssa", "fr"),
	li("LinkedIn · directeur parc / aires protégées (FR, monde)", "directeur \"parc national\" OR \"aires protégées\" expert OR \"aires protégées\" consultant", "", "global", "fr"),

	li("LinkedIn · African Parks (EN/FR)", "\"African Parks\"", "", "ssa", "en"),
	li("LinkedIn · park PPP review / evaluation (EN)", "\"national park\" evaluation OR \"protected area\" \"mid-term review\" OR \"collaborative management\" park OR \"co-management\" protected area", "", "global", "en"),
	li("LinkedIn · évaluation parc / gestion déléguée (FR)", "\"parc national\" évaluation OR \"gestion déléguée\" parc OR \"aires protégées\" \"mi-parcours\"", "", "ssa", "fr"),

	// --- Career pages of key NGOs (generic anchor scraper) ---
	{Name: "Re:Wild careers (Dover)", Kind: "page", Region: "global", Lang: "en", URL: "https://app.dover.com/jobs/rewild"},
	{Name: "Panthera careers", Kind: "page", Region: "global", Lang: "en", URL: "https://www.panthera.org/careers"},
	{Name: "Wetlands International vacancies", Kind: "page", Region: "global", Lang: "en", URL: "https://www.wetlands.org/vacancies/"},
	{Name: "Noé · nous rejoindre", Kind: "page", Region: "ssa", Lang: "fr", URL: "https://noe.org/lassociation/nous-rejoindre/"},
	{Name: "BirdLife · calls for tender", Kind: "page", Region: "global", Lang: "en", URL: "https://www.birdlife.org/calls-for-tender/"},
	{Name: "BirdLife · careers hub", Kind: "page", Region: "global", Lang: "en", URL: "https://www.birdlife.org/careers-hub/"},
	{Name: "Conservation International (Taleo)", Kind: "page", Region: "global", Lang: "en", URL: "https://phh.tbe.taleo.net/phh04/ats/careers/v2/searchResults?org=CONSERVATION&cws=39"},
	{Name: "WWF Deutschland Stellen", Kind: "page", Region: "global", Lang: "de", URL: "https://www.wwf.de/ueber-uns/stellenangebote"},
	{Name: "WWF International jobs", Kind: "page", Region: "global", Lang: "en", URL: "https://wwf.panda.org/jobs_wwf/"},
	{Name: "NGO Jobs in Africa · African Parks", Kind: "page", Region: "ssa", Lang: "en", URL: "https://www.ngojobsinafrica.com/non-profit-organization/african-parks/"},
	{Name: "IFAW careers", Kind: "page", Region: "global", Lang: "en", URL: "https://www.ifaw.org/careers"},
	{Name: "Wildlife Conservation Network", Kind: "page", Region: "global", Lang: "en", URL: "https://wildnet.org/careers/"},
	{Name: "SNV careers", Kind: "page", Region: "global", Lang: "en", URL: "https://www.snv.org/careers"},

	// --- EU procurement (TED, official API, no key) ---
	{Name: "TED · EU tenders (EN/DE/FR)", Kind: "ted", Region: "eu", Lang: "en", URL: "https://api.ted.europa.eu/v3/notices/search"},

	// --- ReliefWeb jobs RSS (African Parks, WCS, FZS, AWF … post here; fetched via reader proxy) ---
	{Name: "ReliefWeb · African Parks jobs", Kind: "rss", Region: "ssa", Lang: "en", URL: "https://reliefweb.int/jobs/rss.xml?advanced-search=(S13968)"},
	{Name: "ReliefWeb · parks & protected areas (EN/FR)", Kind: "rss", Region: "global", Lang: "en", URL: "https://reliefweb.int/jobs/rss.xml?search=%22national+park%22+OR+%22protected+area%22+OR+%22protected+areas%22+OR+%22park+manager%22+OR+%22park+director%22+OR+%22parc+national%22+OR+%22aires+prot%C3%A9g%C3%A9es%22+OR+%22conservation+area%22"},
	{Name: "ReliefWeb · senior conservation roles", Kind: "rss", Region: "global", Lang: "en", URL: "https://reliefweb.int/jobs/rss.xml?search=%28conservation+OR+wildlife+OR+biodiversity%29+AND+%28director+OR+%22team+leader%22+OR+%22chief+of+party%22+OR+%22country+representative%22+OR+%22country+director%22%29"},

	// --- UNDP procurement notices (consultancies) ---
	{Name: "UNGM · protected area", Kind: "page", Region: "global", Lang: "en", URL: "https://www.ungm.org/Public/Notice"},
	{Name: "UNDP Procurement Notices", Kind: "undp", Region: "global", Lang: "en", URL: "https://procurement-notices.undp.org/"},

	// --- RSS ---
	{Name: "Conservation Job Board (RSS)", Kind: "rss", Region: "global", Lang: "en", URL: "https://www.conservationjobboard.com/rss"},
	{Name: "EUROPARC news/jobs (RSS)", Kind: "rss", Region: "eu", Lang: "en", URL: "https://www.europarc.org/feed/"},

	// --- Organisation career pages (generic link scraper) ---
	{Name: "IUCN careers", Kind: "page", Region: "global", Lang: "en", URL: "https://iucn.org/careers"},
	{Name: "IUCN procurement", Kind: "page", Region: "global", Lang: "en", URL: "https://www.iucn.org/resources/procurement-notices"},
	{Name: "Frankfurt Zoological Society jobs", Kind: "page", Region: "global", Lang: "en", URL: "https://fzs.org/en/about-us/organization/jobs/"},
	{Name: "Peace Parks Foundation careers", Kind: "mci", Region: "ssa", Lang: "en", URL: "https://peaceparksjobs.mcidirecthire.com/Vacancy"}, // peaceparks.org/careers now 302→home; portal is MCI Direct Hire (JS-rendered)
	{Name: "African Wildlife Foundation careers", Kind: "page", Region: "ssa", Lang: "en", URL: "https://www.awf.org/careers"},
	{Name: "WWF Österreich Jobs", Kind: "page", Region: "austria", Lang: "de", URL: "https://www.wwf.at/karriere-jobs/"},
	{Name: "Nationalparks Austria", Kind: "page", Region: "austria", Lang: "de", URL: "https://www.nationalparksaustria.at/de/jobs"},
	{Name: "Eurosite vacancies", Kind: "page", Region: "eu", Lang: "en", URL: "https://www.eurosite.org/vacancies/"},
	{Name: "EuroBrussels · environment", Kind: "page", Region: "eu", Lang: "en", URL: "https://www.eurobrussels.com/job_search/environment"},
	{Name: "UNjobnet · protected area", Kind: "page", Region: "global", Lang: "en", URL: "https://www.unjobnet.org/jobs?keywords=protected+area"},
	{Name: "Impactpool · protected area", Kind: "page", Region: "global", Lang: "en", URL: "https://www.impactpool.org/search?q=protected+area"},
	{Name: "CBD Secretariat jobs", Kind: "page", Region: "global", Lang: "en", URL: "https://www.cbd.int/jobs"},
	{Name: "GIZ Jobs · Naturschutz", Kind: "page", Region: "global", Lang: "de", URL: "https://jobs.giz.de/index.php?ac=search_result&search_criterion_keyword%5B%5D=naturschutz"},
	{Name: "Conservation International careers", Kind: "page", Region: "global", Lang: "en", URL: "https://www.conservation.org/careers"},
	{Name: "Texas A&M Wildlife & Fisheries job board", Kind: "page", Region: "global", Lang: "en", URL: "https://jobs.rwfm.tamu.edu/search/?q=director+international"},
	{Name: "NaturAfrica (EU, Central Africa)", Kind: "page", Region: "ssa", Lang: "fr", URL: "https://www.naturafrica.eu/"},
	{Name: "karriere.at · Nationalpark", Kind: "page", Region: "austria", Lang: "de", URL: "https://www.karriere.at/jobs/nationalpark"},
	{Name: "EEAS vacancies", Kind: "page", Region: "eu", Lang: "en", URL: "https://www.eeas.europa.eu/filter-page/vacancies_en"},

	// --- Austria: Land / Bund / park authorities ("foot in the door" pathway) ---
	// Every Bundesland runs a different ATS; see austria.go for the parsers.
	{Name: "Land Oberösterreich · Stellen", Kind: "ooe-api", Region: "austria", Lang: "de", URL: "https://www.land-oberoesterreich.gv.at/525548.htm"},                            // → NP Kalkalpen
	{Name: "Land Steiermark · Jobs", Kind: "erecruiter", Region: "austria", Lang: "de", URL: "https://jobs.stmk.gv.at/Jobs"},                                                     // Abt. 13 → NP Gesäuse
	{Name: "Land Niederösterreich · Stellenangebote", Kind: "noe-rexx", Region: "austria", Lang: "de", URL: "https://bewerbungen.noel.gv.at/stellenangebote.html"},               // → Donau-Auen, Thayatal
	{Name: "Land Burgenland · Stellen", Kind: "softgarden", Region: "austria", Lang: "de", URL: "https://burgenland.softgarden.io/de/vacancies"},                                 // → Neusiedler See
	{Name: "Land Salzburg · Stellen", Kind: "onlyfy", Region: "austria", Lang: "de", URL: "https://land-salzburg.onlyfy.jobs/candidate/widget/cqtfn65i"},                         // → Hohe Tauern
	{Name: "Land Tirol · Stellenangebote", Kind: "tirol", Region: "austria", Lang: "de", URL: "https://www.tirol.gv.at/buergerservice/karriereportal/aktuelle-stellenangebote/"}, // → Hohe Tauern
	{Name: "Land Kärnten · Stellenausschreibungen", Kind: "ktn", Region: "austria", Lang: "de", URL: "https://www.ktn.gv.at/Service/Stellenausschreibungen"},                     // → Hohe Tauern (geo-blocked; via proxy)
	{Name: "Land Vorarlberg · Stellenangebote", Kind: "erecruiter", Region: "austria", Lang: "de", URL: "https://stellenangebote.vorarlberg.at/Jobs"},
	{Name: "Stadt Wien · Stellenangebote", Kind: "talentlink", Region: "austria", Lang: "de", URL: "https://jobs.wien.gv.at/stellenangebote/"},                                                           // MA 22 → Donau-Auen
	{Name: "Bund Jobbörse · BML", Kind: "bund-odata", Region: "austria", Lang: "de", URL: "https://bund.jobboerse.gv.at/sap/opu/odata/sap/ZGW_EREC_JOBSUCHE_SRV/Jobs?$filter=Ressort%20eq%20%270042%27"}, // Nationalparks section
	{Name: "Bund Jobbörse · Natur/Landwirtschaft/Technik", Kind: "bund-odata", Region: "austria", Lang: "de", URL: "https://bund.jobboerse.gv.at/sap/opu/odata/sap/ZGW_EREC_JOBSUCHE_SRV/Jobs?$filter=FunctionalArea%20eq%20%270037%27"},
	{Name: "Österreichische Bundesforste · Karriere", Kind: "erecruiter", Region: "austria", Lang: "de", URL: "https://karriere.bundesforste.at/Jobs"}, // Naturraummanagement, NP-Betriebe
	{Name: "Umweltbundesamt · Karriere", Kind: "page", Region: "austria", Lang: "de", URL: "https://www.umweltbundesamt.at/karriere"},
	{Name: "Nationalpark Kalkalpen · Jobs", Kind: "page", Region: "austria", Lang: "de", URL: "https://www.kalkalpen.at/jobs-mitarbeit"},
	{Name: "Nationalpark Gesäuse · Stellen", Kind: "page", Region: "austria", Lang: "de", URL: "https://nationalpark-gesaeuse.at/nationalpark-mitgestalten/stellenausschreibung/"},
	{Name: "Nationalpark Hohe Tauern · Jobs", Kind: "page", Region: "austria", Lang: "de", URL: "https://hohetauern.at/de/jobs"},
}

func init() {
	Sources = append(Sources, linkedInCompanySources()...)
}

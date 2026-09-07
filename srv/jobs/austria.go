package jobs

// Austrian public-sector job portals. These are the "foot in the door"
// sources: a Referent / Amtssachverständige / Jurist post in a Land's
// Naturschutz department is a realistic stepping stone towards directing
// the national park that Land co-funds (OÖ → Kalkalpen, Stmk Abt. 13 →
// Gesäuse, NÖ → Donau-Auen/Thayatal, Bgld → Neusiedler See, Sbg/Tirol/Ktn →
// Hohe Tauern, BML → all six parks). Portals are heterogeneous — every
// Land runs a different ATS — so each gets a small dedicated parser.

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// --- Land Oberösterreich (ipapm JSON API) ----------------------------------

const ooeAPI = "https://e-gov.ooe.gv.at/ipapm/services/frontend/v1/extern/ausschreibung"

func fetchOOE(ctx context.Context, s Source) ([]Posting, error) {
	b, err := get(ctx, ooeAPI)
	if err != nil {
		return nil, err
	}
	var items []struct {
		ID    int64  `json:"id"`
		Titel string `json:"ausschreibungsTitel"`
		Frist string `json:"bewerbungsfrist"`
		Job   struct {
			Titel        string `json:"titel"`
			Dienststelle string `json:"dienststelle"`
			Direktion    string `json:"direktion"`
			Dienstort    string `json:"dienstort"`
			Leitung      bool   `json:"leitungsfunktion"`
			Einstufung   string `json:"einstufungLD"`
		} `json:"job"`
		Tags []string `json:"berufsfeldTagList"`
	}
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, fmt.Errorf("ooe json: %w", err)
	}
	var out []Posting
	for _, it := range items {
		title := it.Titel
		if title == "" {
			title = it.Job.Titel
		}
		var sn strings.Builder
		fmt.Fprintf(&sn, "[Land OÖ] %s", it.Job.Titel)
		if it.Job.Dienststelle != "" {
			fmt.Fprintf(&sn, " · %s", it.Job.Dienststelle)
		}
		if it.Job.Direktion != "" {
			fmt.Fprintf(&sn, " · %s", it.Job.Direktion)
		}
		if it.Job.Leitung {
			sn.WriteString(" · Leitungsfunktion")
		}
		if it.Job.Einstufung != "" {
			fmt.Fprintf(&sn, " · %s", it.Job.Einstufung)
		}
		for _, t := range it.Tags {
			fmt.Fprintf(&sn, " · %s", t)
		}
		// Role-like titles: pull the detail (department + Aufgabenbereich) so the
		// matcher can see whether this is a Naturschutz unit.
		if atRoleRe.MatchString(title+" "+it.Job.Titel) && !atNegRe.MatchString(title) {
			if db, err := get(ctx, fmt.Sprintf("%s/%d", ooeAPI, it.ID)); err == nil {
				var det struct {
					Dienststelle string `json:"dienststelleAbweichend"`
					Texte        map[string][]struct {
						Text string `json:"text"`
					} `json:"ausschreibungTextMap"`
				}
				if json.Unmarshal(db, &det) == nil {
					if det.Dienststelle != "" {
						fmt.Fprintf(&sn, " · %s", det.Dienststelle)
					}
					for _, k := range []string{"AUFGABENBEREICH", "IMAGETEXT_EXTERN", "ANFORDERUNGSPROFIL_FACH_EXT"} {
						for _, t := range det.Texte[k] {
							fmt.Fprintf(&sn, " · %s", truncate(clean(t.Text), 250))
						}
					}
				}
				time.Sleep(300 * time.Millisecond)
			}
		}
		p := Posting{
			URL:      fmt.Sprintf("https://www.land-oberoesterreich.gv.at/525548.htm#/detail/%d", it.ID),
			Title:    title,
			Org:      "Amt der Oö. Landesregierung",
			Location: it.Job.Dienstort,
			Snippet:  sn.String(),
			Deadline: isoDate(it.Frist),
		}
		if it.Job.Dienststelle != "" {
			p.Org += " · " + it.Job.Dienststelle
		}
		out = append(out, p)
	}
	return out, nil
}

// --- eRecruiter portals (Land Steiermark, Vorarlberg, Bundesforste) ---------
// The listing page embeds its model as JSON: "Jobs":[{"Id":..,"Title":..}]

var erecJobsRe = regexp.MustCompile(`"Jobs":(\[\{.*?\}\])[,}]`)

func fetchERecruiter(ctx context.Context, s Source) ([]Posting, error) {
	b, err := get(ctx, s.URL)
	if err != nil {
		return nil, err
	}
	m := erecJobsRe.FindSubmatch(b)
	if m == nil {
		return nil, fmt.Errorf("erecruiter: no embedded Jobs json")
	}
	var items []struct {
		ID       int64  `json:"Id"`
		Title    string `json:"Title"`
		SubTitle string `json:"SubTitle"`
		Location string `json:"Location"`
		Date     string `json:"Date"`
		Slug     string `json:"UrlEncodedTitle"`
	}
	if err := json.Unmarshal(m[1], &items); err != nil {
		return nil, fmt.Errorf("erecruiter json: %w", err)
	}
	base, _ := url.Parse(s.URL)
	org := s.Name
	if i := strings.Index(org, " ·"); i > 0 {
		org = org[:i]
	}
	var out []Posting
	for _, it := range items {
		u := fmt.Sprintf("%s://%s/Job/%d", base.Scheme, base.Host, it.ID)
		if it.Slug != "" {
			u += "/" + it.Slug
		}
		loc := it.Location
		if loc == "" {
			loc = strings.TrimPrefix(it.SubTitle, "Dienstort: ")
		}
		out = append(out, Posting{
			URL: u, Title: it.Title, Org: org, Location: loc,
			Snippet: "[" + org + "] " + it.SubTitle,
			Posted:  isoDate(it.Date),
		})
	}
	return out, nil
}

// --- Bund Jobbörse (SAP OData) --------------------------------------------
// Federal ministries (BML = Nationalparks section, BMK/Umweltbundesamt …).
// s.URL carries the OData query; we page through with $skip.

func fetchBundOData(ctx context.Context, s Source) ([]Posting, error) {
	var out []Posting
	for skip := 0; skip < 2000; skip += 100 {
		u := s.URL
		if !strings.Contains(u, "$format=json") {
			u += "&$format=json"
		}
		u += fmt.Sprintf("&$top=100&$skip=%d", skip)
		b, err := get(ctx, u)
		if err != nil {
			return out, err
		}
		var res struct {
			D struct {
				Results []struct {
					Guid      string `json:"PinstGuid"`
					Header    string `json:"PostingHeader"`
					Ort       string `json:"Zzdienstort"`
					End       string `json:"EndDate"`
					Begin     string `json:"PublishedBegda"`
					Ressort   string `json:"RessortTxt"`
					FunctArea string `json:"FunctionalAreaTxt"`
					Preamble  string `json:"Preamble"`
					Tasks     string `json:"Tasks"`
				} `json:"results"`
			} `json:"d"`
		}
		if err := json.Unmarshal(b, &res); err != nil {
			return out, fmt.Errorf("bund odata json: %w", err)
		}
		for _, r := range res.D.Results {
			// Pull department + duties for role-like titles (one extra request each).
			if atRoleRe.MatchString(r.Header) && !atNegRe.MatchString(r.Header) {
				if db, err := get(ctx, fmt.Sprintf("https://bund.jobboerse.gv.at/sap/opu/odata/sap/ZGW_EREC_JOBSUCHE_SRV/Jobs('%s')?$format=json", r.Guid)); err == nil {
					var det struct {
						D struct {
							Department, Preamble, Tasks string
						} `json:"d"`
					}
					if json.Unmarshal(db, &det) == nil {
						r.FunctArea = strings.TrimSpace(r.FunctArea + " " + det.D.Department)
						r.Preamble, r.Tasks = det.D.Preamble, det.D.Tasks
					}
				}
			}
			sn := "[Bund] " + r.Ressort
			if r.FunctArea != "" {
				sn += " · " + r.FunctArea
			}
			if r.Preamble != "" {
				sn += " · " + truncate(clean(r.Preamble), 300)
			}
			if r.Tasks != "" {
				sn += " · " + truncate(clean(r.Tasks), 400)
			}
			out = append(out, Posting{
				URL:      "https://bund.jobboerse.gv.at/sap/bc/jobs/index.html#/details/" + r.Guid,
				Title:    r.Header,
				Org:      r.Ressort,
				Location: r.Ort,
				Snippet:  sn,
				Posted:   sapDate(r.Begin),
				Deadline: sapDate(r.End),
			})
		}
		if len(res.D.Results) < 100 {
			break
		}
	}
	return out, nil
}

var sapDateRe = regexp.MustCompile(`/Date\((\d+)\)/`)

func sapDate(s string) string {
	m := sapDateRe.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	ms, _ := strconv.ParseInt(m[1], 10, 64)
	return time.UnixMilli(ms).UTC().Format("2006-01-02")
}

// isoDate normalises dd.mm.yyyy / yyyy-mm-dd[…] to yyyy-mm-dd.
func isoDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) >= 10 && s[4] == '-' {
		return s[:10]
	}
	if t, err := time.Parse("02.01.2006", s); err == nil {
		return t.Format("2006-01-02")
	}
	return ""
}

// --- Land Niederösterreich (rexx systems) ---------------------------------
// Certificate chain is incomplete (GEANT intermediate missing) → insecure client.

var noeJobRe = regexp.MustCompile(`(?s)<a[^>]*href="(https://bewerbungen\.noel\.gv\.at/[^"]*-j(\d+)\.html)"[^>]*>(.*?)</a>`)

func fetchNOE(ctx context.Context, s Source) ([]Posting, error) {
	b, err := getWith(ctx, insecureClient, s.URL)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []Posting
	for _, m := range noeJobRe.FindAllStringSubmatch(string(b), -1) {
		t := clean(m[3])
		if t == "" || seen[m[2]] {
			continue
		}
		seen[m[2]] = true
		out = append(out, Posting{URL: m[1], Title: t, Org: "Amt der NÖ Landesregierung", Location: "Niederösterreich", Snippet: "[Land NÖ] " + t})
	}
	return out, nil
}

// --- Land Burgenland (softgarden) -----------------------------------------

var sgJobRe = regexp.MustCompile(`(?s)<a[^>]*href="\.\./job/(\d+)/([^"?]*)[^"]*"[^>]*>(.*?)</a>`)

func fetchSoftgarden(ctx context.Context, s Source) ([]Posting, error) {
	b, err := get(ctx, s.URL)
	if err != nil {
		return nil, err
	}
	base, _ := url.Parse(s.URL)
	seen := map[string]bool{}
	var out []Posting
	for _, m := range sgJobRe.FindAllStringSubmatch(string(b), -1) {
		t := clean(m[3])
		if t == "" || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		out = append(out, Posting{
			URL:   fmt.Sprintf("%s://%s/job/%s/%s", base.Scheme, base.Host, m[1], m[2]),
			Title: t, Org: "Land Burgenland", Location: "Burgenland", Snippet: "[Land Burgenland] " + t,
		})
	}
	return out, nil
}

// --- Land Kärnten -----------------------------------------------------------
// ktn.gv.at drops connections from non-EU cloud ranges, so we go through
// the free-proxy pool (see proxy.go) and fall back to the Wayback Machine.

var ktnJobRe = regexp.MustCompile(`(?s)<a[^>]*href="(/Service/Stellenausschreibungen/Details\?id=(\d+))"[^>]*>(.*?)</a>`)

func fetchKTN(ctx context.Context, s Source) ([]Posting, error) {
	b, err := getViaProxy(ctx, s.URL, func(b []byte) bool { return strings.Contains(string(b), "Details?id=") })
	if err != nil {
		wb, werr := get(ctx, "https://web.archive.org/web/2id_/"+s.URL)
		if werr != nil {
			return nil, fmt.Errorf("proxy: %v; wayback: %v", err, werr)
		}
		b = wb
	}
	seen := map[string]bool{}
	var out []Posting
	for _, m := range ktnJobRe.FindAllStringSubmatch(string(b), -1) {
		t := clean(m[3])
		if t == "" || seen[m[2]] {
			continue
		}
		seen[m[2]] = true
		out = append(out, Posting{
			URL: "https://www.ktn.gv.at" + html.UnescapeString(m[1]), Title: t,
			Org: "Amt der Kärntner Landesregierung", Location: "Kärnten", Snippet: "[Land Kärnten] " + t,
		})
	}
	return out, nil
}

// --- Land Salzburg (onlyfy / prescreen widget) ------------------------------
// The public career page is client-rendered, but the embeddable widget at
// /candidate/widget/<id> is plain server-side HTML.

var onlyfyJobRe = regexp.MustCompile(`(?s)<a[^>]*href="(/candidate/widget/[a-z0-9]+/job/([a-z0-9]+)[^"]*)"[^>]*>(.*?)</a>`)

func fetchOnlyfy(ctx context.Context, s Source) ([]Posting, error) {
	u := s.URL
	if !strings.Contains(u, "display_length") {
		u += "?display_length=100&page=1&sort=date&sort_dir=DESC"
	}
	b, err := get(ctx, u)
	if err != nil {
		return nil, err
	}
	base, _ := url.Parse(s.URL)
	seen := map[string]bool{}
	var out []Posting
	for _, m := range onlyfyJobRe.FindAllStringSubmatch(string(b), -1) {
		t := clean(m[3])
		if t == "" || seen[m[2]] {
			continue
		}
		seen[m[2]] = true
		out = append(out, Posting{
			URL:   fmt.Sprintf("%s://%s/de/job/%s", base.Scheme, base.Host, m[2]),
			Title: t, Org: "Land Salzburg", Location: "Salzburg", Snippet: "[Land Salzburg] " + t,
		})
	}
	return out, nil
}

// --- Stadt Wien (Lumesse / Cornerstone TalentLink) -------------------------
// jobs.wien.gv.at is a widget over emea3.recruitmentplatform.com; the widget
// authenticates as "<siteTechId>:guest:FO"/"guest" via plain headers.

const wienSiteID = "QYOFK026203F3VBQBLO8MV7XN"

func fetchTalentLink(ctx context.Context, s Source) ([]Posting, error) {
	var out []Posting
	for first := 0; first < 1000; first += 100 {
		u := fmt.Sprintf("https://emea3.recruitmentplatform.com/fo/rest/jobs?firstResult=%d&maxResults=100&sortBy=dPostingStart&sortOrder=DESC", first)
		req, err := http.NewRequestWithContext(ctx, "POST", u, strings.NewReader(`{"searchCriteria":{"criteria":[]}}`))
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("username", wienSiteID+":guest:FO")
		req.Header.Set("password", "guest")
		req.Header.Set("lumesse-language", "DE")
		resp, err := httpClient.Do(req)
		if err != nil {
			return out, err
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 6<<20))
		resp.Body.Close()
		if err != nil {
			return out, err
		}
		if resp.StatusCode >= 400 {
			return out, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		var res struct {
			Globals struct {
				JobsCount int `json:"jobsCount"`
			} `json:"globals"`
			Jobs []struct {
				ID     int64 `json:"id"`
				Fields struct {
					Title string `json:"jobTitle"`
					Dept  string `json:"SDPTNAMELEVEL2"`
					Addr  string `json:"FFIELD001_002"`
					End   int64  `json:"DPOSTINGEND"`
				} `json:"jobFields"`
				Custom []struct {
					Title   string `json:"title"`
					Content string `json:"content"`
				} `json:"customFields"`
			} `json:"jobs"`
		}
		if err := json.Unmarshal(b, &res); err != nil {
			return out, fmt.Errorf("talentlink json: %w", err)
		}
		for _, j := range res.Jobs {
			sn := "[Stadt Wien] " + j.Fields.Dept
			for _, c := range j.Custom {
				if strings.Contains(strings.ToLower(c.Title), "dienststelle") || strings.Contains(strings.ToLower(c.Title), "aufgaben") {
					sn += " · " + truncate(clean(c.Content), 300)
				}
			}
			p := Posting{
				URL:      fmt.Sprintf("https://jobs.wien.gv.at/stellenangebote/details.html?jobId=%d", j.ID),
				Title:    j.Fields.Title,
				Org:      "Stadt Wien · " + j.Fields.Dept,
				Location: j.Fields.Addr,
				Snippet:  sn,
			}
			if j.Fields.End > 0 {
				p.Deadline = time.UnixMilli(j.Fields.End).UTC().Format("2006-01-02")
			}
			out = append(out, p)
		}
		if first+100 >= res.Globals.JobsCount || len(res.Jobs) == 0 {
			break
		}
	}
	return out, nil
}

// --- Land Tirol (TYPO3 category pages) --------------------------------------
// Each category page lists postings as content-box cards: <a href=…><h3>Title</h3>
// <p class="small-text">Abteilung … / Bewerbungsfrist: …</p></a>.

var (
	tirolCategories = []string{
		"natur-und-geisteswissenschaftlerinnen", "technikerinnen-und-forstmitarbeiterinnen",
		"verwaltungsjuristinnen", "wirtschaftsberufe", "verwaltungsmitarbeiterinnen",
	}
	tirolCardRe = regexp.MustCompile(`(?s)<a[^>]*href="(/buergerservice/karriereportal/aktuelle-stellenangebote/[a-z0-9-]+/[a-z0-9-]+/)"[^>]*>(.*?)</a>`)
	tirolH3Re   = regexp.MustCompile(`(?s)<h3[^>]*>(.*?)</h3>`)
	tirolSubRe  = regexp.MustCompile(`(?s)<p class="small-text">(.*?)</p>`)
	tirolFrist  = regexp.MustCompile(`Bewerbungsfrist:\s*(\d{1,2})\.\s*([A-Za-zäö]+)\s*(\d{4})`)
)

var deMonths = map[string]string{"jänner": "01", "januar": "01", "februar": "02", "märz": "03", "april": "04", "mai": "05", "juni": "06", "juli": "07", "august": "08", "september": "09", "oktober": "10", "november": "11", "dezember": "12"}

func fetchTirol(ctx context.Context, s Source) ([]Posting, error) {
	base := strings.TrimSuffix(s.URL, "/")
	seen := map[string]bool{}
	var out []Posting
	var lastErr error
	for _, cat := range tirolCategories {
		b, err := get(ctx, base+"/"+cat+"/")
		if err != nil {
			lastErr = err
			continue
		}
		for _, m := range tirolCardRe.FindAllStringSubmatch(string(b), -1) {
			if seen[m[1]] {
				continue
			}
			h := tirolH3Re.FindStringSubmatch(m[2])
			if h == nil {
				continue
			}
			seen[m[1]] = true
			p := Posting{URL: "https://www.tirol.gv.at" + m[1], Title: clean(h[1]), Org: "Amt der Tiroler Landesregierung", Location: "Tirol"}
			if sub := tirolSubRe.FindStringSubmatch(m[2]); sub != nil {
				p.Snippet = "[Land Tirol] " + clean(sub[1])
				if f := tirolFrist.FindStringSubmatch(p.Snippet); f != nil {
					if mo, ok := deMonths[strings.ToLower(f[2])]; ok {
						p.Deadline = fmt.Sprintf("%s-%s-%02s", f[3], mo, f[1])
					}
				}
			}
			out = append(out, p)
		}
	}
	if len(out) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return out, nil
}

// --- Detail enrichment ------------------------------------------------------
// Several Land portals list only a bare title ("Juristin bzw. Jurist"). For
// titles that look like a professional role we pull the detail page once so
// the keyword matcher / ranker can see the department and duties.

var atDetailKinds = map[string]bool{"erecruiter": true, "noe-rexx": true, "softgarden": true, "onlyfy": true, "tirol": true}

var (
	scriptRe = regexp.MustCompile(`(?s)<(script|style|nav|header|footer)[^>]*>.*?</(script|style|nav|header|footer)>`)
	bodyRe   = regexp.MustCompile(`(?s)<body[^>]*>(.*)</body>`)
)

func enrichAT(ctx context.Context, s Source, ps []Posting) {
	if !atDetailKinds[s.Kind] {
		return
	}
	n := 0
	for i := range ps {
		p := &ps[i]
		if n >= 20 || !atRoleRe.MatchString(p.Title) || atNegRe.MatchString(p.Title) || atTopicRe.MatchString(p.Snippet) {
			continue
		}
		n++
		cl := httpClient
		if s.Kind == "noe-rexx" {
			cl = insecureClient
		}
		b, err := getWith(ctx, cl, p.URL)
		if err != nil {
			continue
		}
		t := string(b)
		if m := bodyRe.FindStringSubmatch(t); m != nil {
			t = m[1]
		}
		t = clean(scriptRe.ReplaceAllString(t, " "))
		// Skip the navigation boilerplate: start at the first title occurrence.
		if i := strings.Index(t, p.Title); i > 0 {
			t = t[i:]
		}
		p.Snippet = strings.TrimSpace(p.Snippet + " · " + truncate(t, 1800))
		time.Sleep(400 * time.Millisecond)
	}
}

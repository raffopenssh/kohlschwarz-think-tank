package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0 Safari/537.36 kohlschwarz-jobradar/1.0"

var httpClient = &http.Client{Timeout: 40 * time.Second}

func get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(u, "https://r.jina.ai/") {
		// The reader proxy's Cloudflare challenges browser UAs but admits plain clients.
		req.Header.Set("User-Agent", "curl/8.5.0 kohlschwarz-jobradar/1.0")
		req.Header.Set("Accept", "*/*")
	} else {
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml,application/json;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en,de;q=0.9,fr;q=0.8")
	}
	var resp *http.Response
	for attempt := 0; ; attempt++ {
		resp, err = httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == 429 && attempt < 3 {
			resp.Body.Close()
			wait := time.Duration(20*(attempt+1)) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			continue
		}
		break
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 6<<20))
}

// Fetch dispatches on source kind.
func Fetch(ctx context.Context, s Source) ([]Posting, error) {
	var ps []Posting
	var err error
	switch s.Kind {
	case "linkedin":
		ps, err = fetchLinkedIn(ctx, s)
	case "rss":
		ps, err = fetchRSS(ctx, s)
	case "ted":
		ps, err = fetchTED(ctx, s)
	case "undp":
		ps, err = fetchUNDP(ctx, s)
	case "mci":
		ps, err = fetchMCI(ctx, s)
	case "ooe-api":
		ps, err = fetchOOE(ctx, s)
	case "erecruiter":
		ps, err = fetchERecruiter(ctx, s)
	case "bund-odata":
		ps, err = fetchBundOData(ctx, s)
	case "noe-rexx":
		ps, err = fetchNOE(ctx, s)
	case "softgarden":
		ps, err = fetchSoftgarden(ctx, s)
	case "ktn":
		ps, err = fetchKTN(ctx, s)
	case "onlyfy":
		ps, err = fetchOnlyfy(ctx, s)
	case "talentlink":
		ps, err = fetchTalentLink(ctx, s)
	case "tirol":
		ps, err = fetchTirol(ctx, s)
	default:
		ps, err = fetchPage(ctx, s)
	}
	enrichAT(ctx, s, ps)
	for i := range ps {
		ps[i].Source = s.Name
		if ps[i].Lang == "" {
			ps[i].Lang = s.Lang
		}
		ps[i].Title = clean(ps[i].Title)
		ps[i].Org = clean(ps[i].Org)
		ps[i].Location = clean(ps[i].Location)
		ps[i].Snippet = clean(ps[i].Snippet)
		ps[i].Region = GuessRegion(ps[i], s.Region)
	}
	return ps, err
}

var (
	tagRe = regexp.MustCompile(`<[^>]+>`)
	wsRe  = regexp.MustCompile(`\s+`)
)

func clean(s string) string {
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = wsRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func stripQuery(u string) string {
	if i := strings.IndexAny(u, "?#"); i > 0 {
		return u[:i]
	}
	return u
}

// --- LinkedIn guest search --------------------------------------------------

var (
	liCardRe  = regexp.MustCompile(`(?s)<div class="base-card[^"]*job-search-card[^"]*"(.*?)</li>`)
	liLinkRe  = regexp.MustCompile(`href="(https://[a-z]{2,3}\.linkedin\.com/jobs/view/[^"]+)"`)
	liTitleRe = regexp.MustCompile(`(?s)base-search-card__title">(.*?)</h3>`)
	liOrgRe   = regexp.MustCompile(`(?s)base-search-card__subtitle">(.*?)</h4>`)
	liLocRe   = regexp.MustCompile(`(?s)job-search-card__location">(.*?)</span>`)
	liDateRe  = regexp.MustCompile(`datetime="(\d{4}-\d{2}-\d{2})"`)
)

func fetchLinkedIn(ctx context.Context, s Source) ([]Posting, error) {
	var out []Posting
	for start := 0; start < 100; start += 25 {
		u := strings.Replace(s.URL, "start=0", fmt.Sprintf("start=%d", start), 1)
		b, err := get(ctx, u)
		if err != nil {
			if start == 0 {
				return nil, err
			}
			break
		}
		cards := liCardRe.FindAllStringSubmatch(string(b), -1)
		for _, c := range cards {
			card := c[1]
			link := liLinkRe.FindStringSubmatch(card)
			title := liTitleRe.FindStringSubmatch(card)
			if link == nil || title == nil {
				continue
			}
			p := Posting{URL: stripQuery(html.UnescapeString(link[1])), Title: title[1]}
			if m := liOrgRe.FindStringSubmatch(card); m != nil {
				p.Org = m[1]
			}
			if m := liLocRe.FindStringSubmatch(card); m != nil {
				p.Location = m[1]
			}
			if m := liDateRe.FindStringSubmatch(card); m != nil {
				p.Posted = m[1]
			}
			out = append(out, p)
		}
		if len(cards) < 25 {
			break
		}
		time.Sleep(4 * time.Second)
	}
	return out, nil
}

// --- RSS -------------------------------------------------------------------

type rssDoc struct {
	Items []struct {
		Title   string `xml:"title"`
		Link    string `xml:"link"`
		Desc    string `xml:"description"`
		PubDate string `xml:"pubDate"`
	} `xml:"channel>item"`
}

// viaReader lists hosts behind a JS/WAF challenge that plain HTTP cannot pass;
// they are fetched through the r.jina.ai rendering proxy (returns the raw feed).
var viaReader = []string{"reliefweb.int"}

func fetchRSS(ctx context.Context, s Source) ([]Posting, error) {
	u := s.URL
	for _, h := range viaReader {
		if strings.Contains(u, h) {
			u = "https://r.jina.ai/" + u
		}
	}
	b, err := get(ctx, u)
	if err != nil {
		return nil, err
	}
	if i := bytes.Index(b, []byte("<?xml")); i > 0 {
		b = b[i:]
	} else if i := bytes.Index(b, []byte("<rss")); i > 0 {
		b = b[i:]
	}
	var d rssDoc
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.Strict = false
	if err := dec.Decode(&d); err != nil {
		return nil, err
	}
	var out []Posting
	for _, it := range d.Items {
		p := Posting{URL: strings.TrimSpace(it.Link), Title: it.Title, Snippet: it.Desc}
		if t, err := parseAnyDate(it.PubDate); err == nil {
			p.Posted = t.Format("2006-01-02")
		}
		desc := html.UnescapeString(it.Desc)
		if m := rwOrgRe.FindStringSubmatch(desc); m != nil { // ReliefWeb
			p.Org = m[1]
			if c := rwCountryRe.FindStringSubmatch(desc); c != nil {
				p.Location = c[1]
			}
			if d := rwClosingRe.FindStringSubmatch(desc); d != nil {
				if t, err := time.Parse("2 Jan 2006", strings.TrimSpace(d[1])); err == nil {
					p.Deadline = t.Format("2006-01-02")
				}
			}
			p.Snippet = truncate(clean(rwTagRe.ReplaceAllString(desc, " ")), 700)
		} else if i := strings.LastIndex(it.Desc, "--"); i > 0 { // Conservation Job Board: "Title<br>$x -- Org<br>"
			p.Org = it.Desc[i+2:]
		}
		out = append(out, p)
	}
	return out, nil
}

var (
	rwOrgRe     = regexp.MustCompile(`Organization:\s*([^<]+)<`)
	rwCountryRe = regexp.MustCompile(`Country:\s*([^<]+)<`)
	rwClosingRe = regexp.MustCompile(`Closing date:\s*([^<]+)<`)
	rwTagRe     = regexp.MustCompile(`(?s)<div class="(tag|date)[^"]*">.*?</div>`)
)

func parseAnyDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, f := range []string{time.RFC1123Z, time.RFC1123, "Mon, 2 Jan 2006", "Mon, 02 Jan 2006", time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("bad date %q", s)
}

// --- TED (EU Tenders Electronic Daily) -------------------------------------

var tedTerms = []string{"national park", "national parks", "protected area", "protected areas", "parc national", "parcs nationaux", "aires protégées", "aire protégée", "Nationalpark", "Nationalparks", "Schutzgebiet", "Schutzgebiete", "Naturpark", "Biosphärenpark", "biosphere reserve", "game reserve", "wildlife management", "conservation area"}

func fetchTED(ctx context.Context, s Source) ([]Posting, error) {
	since := time.Now().AddDate(0, 0, -45).Format("20060102")
	seen := map[string]bool{}
	var out []Posting
	var lastErr error
	for _, term := range tedTerms {
		// TED's FT operator does not combine OR terms reliably; one query per term.
		q := fmt.Sprintf(`FT ~ ("%s") AND PD>=%s AND notice-type IN (cn-standard cn-social pin-cfc-standard pin-cfc-social pin-only cn-desg)`, term, since)
		ps, err := tedQuery(ctx, s.URL, q)
		if err != nil {
			lastErr = err
			continue
		}
		for _, p := range ps {
			if !seen[p.URL] {
				seen[p.URL] = true
				out = append(out, p)
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
	if len(out) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return out, nil
}

func tedQuery(ctx context.Context, endpoint, q string) ([]Posting, error) {
	body, _ := json.Marshal(map[string]any{
		"query":  q,
		"fields": []string{"publication-number", "title-proc", "description-proc", "buyer-name", "place-of-performance", "publication-date", "deadline-receipt-tender-date-lot", "notice-type"},
		"limit":  50,
		"scope":  "ACTIVE",
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 6<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %.200s", resp.StatusCode, b)
	}
	var r struct {
		Notices []map[string]json.RawMessage `json:"notices"`
		Message string                       `json:"message"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	if r.Message != "" {
		return nil, fmt.Errorf("ted: %s", r.Message)
	}
	var out []Posting
	for _, n := range r.Notices {
		var pn, pd, nt string
		json.Unmarshal(n["publication-number"], &pn)
		json.Unmarshal(n["publication-date"], &pd)
		json.Unmarshal(n["notice-type"], &nt)
		if pn == "" {
			continue
		}
		title, lang := pickLang(n["title-proc"])
		desc, _ := pickLang(n["description-proc"])
		buyer, _ := pickLang(n["buyer-name"])
		var places []string
		json.Unmarshal(n["place-of-performance"], &places)
		var deadlines []string
		json.Unmarshal(n["deadline-receipt-tender-date-lot"], &deadlines)
		p := Posting{
			URL:      "https://ted.europa.eu/en/notice/-/detail/" + pn,
			Title:    title,
			Org:      buyer,
			Location: strings.Join(uniq(places), ", "),
			Snippet:  "[TED " + nt + "] " + truncate(desc, 600),
			Lang:     lang,
			Tender:   true,
		}
		if len(pd) >= 10 {
			p.Posted = pd[:10]
		}
		if len(deadlines) > 0 && len(deadlines[0]) >= 10 {
			p.Deadline = deadlines[0][:10]
		}
		out = append(out, p)
	}
	return out, nil
}

// pickLang picks eng, then deu, then fra, then anything from a TED multilingual field.
func pickLang(raw json.RawMessage) (string, string) {
	if raw == nil {
		return "", ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		var s string
		json.Unmarshal(raw, &s)
		return s, ""
	}
	for _, k := range []string{"eng", "deu", "fra"} {
		if v, ok := m[k]; ok {
			return rawToString(v), map[string]string{"eng": "en", "deu": "de", "fra": "fr"}[k]
		}
	}
	for _, v := range m {
		return rawToString(v), ""
	}
	return "", ""
}

func rawToString(v json.RawMessage) string {
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	var a []string
	if json.Unmarshal(v, &a) == nil {
		return strings.Join(a, " ")
	}
	return string(v)
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// --- UNDP procurement notices ---------------------------------------------

var undpRowRe = regexp.MustCompile(`(?s)<a href="(view_notice\.cfm\?notice_id=\d+)"[^>]*>(.*?)</a>`)
var undpCellRe = regexp.MustCompile(`(?s)<span>(.*?)</span>`)

func fetchUNDP(ctx context.Context, s Source) ([]Posting, error) {
	b, err := get(ctx, s.URL)
	if err != nil {
		return nil, err
	}
	var out []Posting
	for _, m := range undpRowRe.FindAllStringSubmatch(string(b), -1) {
		cells := undpCellRe.FindAllStringSubmatch(m[2], -1)
		if len(cells) < 3 {
			continue
		}
		p := Posting{URL: "https://procurement-notices.undp.org/" + m[1], Title: cells[0][1], Org: "UNDP", Location: cells[2][1], Snippet: "[UNDP procurement] ", Tender: true}
		if len(cells) >= 4 {
			p.Snippet += clean(cells[3][1])
		}
		if len(cells) >= 6 {
			if t, err := time.Parse("02-Jan-06", strings.TrimSpace(clean(cells[5][1]))); err == nil {
				p.Deadline = t.Format("2006-01-02")
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// --- Generic page: extract anchors whose text looks like a job title -------

var anchorRe = regexp.MustCompile(`(?s)<a\s[^>]*href="([^"#]+)"[^>]*>(.*?)</a>`)

func fetchPage(ctx context.Context, s Source) ([]Posting, error) {
	b, err := get(ctx, s.URL)
	if err != nil {
		return nil, err
	}
	base, _ := url.Parse(s.URL)
	var out []Posting
	seen := map[string]bool{}
	for _, m := range anchorRe.FindAllStringSubmatch(string(b), -1) {
		text := clean(m[2])
		if len(text) < 12 || len(text) > 180 {
			continue
		}
		ref, err := url.Parse(html.UnescapeString(m[1]))
		if err != nil {
			continue
		}
		abs := base.ResolveReference(ref).String()
		if strings.HasPrefix(abs, "mailto:") || strings.HasPrefix(abs, "javascript:") || seen[abs] {
			continue
		}
		// Skip obvious nav links: same URL as page, social, share.
		if abs == s.URL || strings.Contains(abs, "facebook.com") || strings.Contains(abs, "twitter.com") || strings.Contains(abs, "instagram.com") || strings.Contains(abs, "youtube.com") || strings.Contains(abs, "shareArticle") {
			continue
		}
		seen[abs] = true
		out = append(out, Posting{URL: abs, Title: text})
	}
	return out, nil
}

// MCI Direct Hire career portals (e.g. peaceparksjobs.mcidirecthire.com) render
// the list client-side from GET /Vacancy/Vacancies. Each job-card carries the
// title, posted date, location and a share link to /Vacancy/ViewDetails.
var (
	mciTitleRe = regexp.MustCompile(`job-title">\s*([^<]+)`)
	mciDateRe  = regexp.MustCompile(`fa-calendar"></i>\s*(\d{4})/(\d{2})/(\d{2})`)
	mciLocRe   = regexp.MustCompile(`fa-map-marker"></i>\s*([^<]+)`)
	mciURLRe   = regexp.MustCompile(`(/Vacancy/ViewDetails\?parameters=[^"'&\s]+)`)
)

func fetchMCI(ctx context.Context, s Source) ([]Posting, error) {
	base, err := url.Parse(s.URL)
	if err != nil {
		return nil, err
	}
	b, err := get(ctx, base.Scheme+"://"+base.Host+"/Vacancy/Vacancies?PageNumber=1&GroupID=0")
	if err != nil {
		return nil, err
	}
	var out []Posting
	for _, c := range strings.Split(string(b), `class="card job-card"`)[1:] {
		t := mciTitleRe.FindStringSubmatch(c)
		u := mciURLRe.FindStringSubmatch(c)
		if t == nil || u == nil {
			continue
		}
		p := Posting{URL: base.Scheme + "://" + base.Host + html.UnescapeString(u[1]), Title: clean(t[1])}
		if d := mciDateRe.FindStringSubmatch(c); d != nil {
			p.Posted = d[1] + "-" + d[2] + "-" + d[3]
		}
		if l := mciLocRe.FindStringSubmatch(c); l != nil {
			p.Location = clean(l[1])
		}
		out = append(out, p)
	}
	return out, nil
}

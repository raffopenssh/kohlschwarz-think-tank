package srv

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"srv.exe.dev/db"
	"srv.exe.dev/db/dbgen"
	"srv.exe.dev/srv/jobs"
)

type Server struct {
	DB           *sql.DB
	Hostname     string
	TemplatesDir string
	StaticDir    string
}

type pageData struct {
	Hostname string
	Apps     []dbgen.App
	App      *dbgen.App
	Error    string
	Success  string
	Lang     string // "de" or "en"
	IsAdmin  bool   // exe.dev-authenticated owner
}

func (p pageData) DE() bool { return p.Lang != "en" }

// Clicks returns the click count value (0 if unset).
func (p pageData) Clicks(app dbgen.App) int64 {
	if app.ClickCount != nil {
		return *app.ClickCount
	}
	return 0
}

// Desc returns the description in the page language, falling back to English.
func (p pageData) Desc(app dbgen.App) string {
	if p.DE() && app.DescriptionDe != nil && *app.DescriptionDe != "" {
		return *app.DescriptionDe
	}
	return app.Description
}

func New(dbPath, hostname string) (*Server, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(thisFile)
	srv := &Server{
		Hostname:     hostname,
		TemplatesDir: filepath.Join(baseDir, "templates"),
		StaticDir:    filepath.Join(baseDir, "static"),
	}
	if err := srv.setUpDatabase(dbPath); err != nil {
		return nil, err
	}
	if err := srv.seedApps(); err != nil {
		slog.Warn("seed apps", "error", err)
	}
	return srv, nil
}

func (s *Server) HandleRoot(w http.ResponseWriter, r *http.Request) {
	s.renderIndex(w, r, "de")
}

func (s *Server) HandleRootEN(w http.ResponseWriter, r *http.Request) {
	s.renderIndex(w, r, "en")
}

func (s *Server) renderIndex(w http.ResponseWriter, r *http.Request, lang string) {
	q := dbgen.New(s.DB)
	apps, err := q.ListApps(r.Context())
	if err != nil {
		slog.Warn("list apps", "error", err)
	}

	data := pageData{
		Hostname: s.Hostname,
		Apps:     apps,
		Lang:     lang,
		IsAdmin:  s.isAdmin(r),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "index.html", data); err != nil {
		slog.Warn("render template", "url", r.URL.Path, "error", err)
	}
}

// AdminEmail is the exe.dev account allowed to see /admin. Set via ADMIN_EMAIL; defaults to the VM owner.
func adminEmail() string {
	if e := os.Getenv("ADMIN_EMAIL"); e != "" {
		return strings.ToLower(e)
	}
	return "raffaelhickisch+exedev@gmail.com"
}

// isAdmin is true when the request carries the exe.dev auth header for the admin email.
func (s *Server) isAdmin(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-ExeDev-Email")), adminEmail())
}

// requireAuth gates admin pages: exe.dev login (owner email) is the primary
// mechanism; HTTP basic auth with ADMIN_PASSWORD remains as a fallback for
// access paths that bypass the exe.dev proxy (e.g. kohlschwarz.at via Cloudflare).
func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if s.isAdmin(r) {
		return true
	}
	if r.Header.Get("X-ExeDev-Email") != "" {
		// Logged in via exe.dev, but not the owner.
		http.Error(w, "Forbidden", http.StatusForbidden)
		return false
	}
	// Not logged in: bounce through exe.dev login (works on *.exe.xyz and the
	// custom domain, both served by the exe.dev proxy). Basic auth remains as
	// an explicit fallback via ?basic=1 or when running without the proxy.
	_, _, hasBasic := r.BasicAuth()
	if !hasBasic && r.URL.Query().Get("basic") == "" && r.Method == http.MethodGet && r.Header.Get("X-Forwarded-Host") != "" {
		http.Redirect(w, r, "/__exe.dev/login?redirect="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
		return false
	}
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		adminPassword = "changeme" // fallback for local dev
	}
	user, pass, ok := r.BasicAuth()
	if !ok || user != "admin" || pass != adminPassword {
		w.Header().Set("WWW-Authenticate", `Basic realm="Admin"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func (s *Server) HandleAdmin(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}

	q := dbgen.New(s.DB)
	apps, err := q.ListApps(r.Context())
	if err != nil {
		slog.Warn("list apps", "error", err)
	}

	data := pageData{
		Hostname: s.Hostname,
		Apps:     apps,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "admin.html", data); err != nil {
		slog.Warn("render template", "url", r.URL.Path, "error", err)
	}
}

func (s *Server) HandleAdminEdit(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}

	idStr := r.PathValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	q := dbgen.New(s.DB)
	data := pageData{Hostname: s.Hostname}

	if id > 0 {
		app, err := q.GetApp(r.Context(), id)
		if err != nil {
			data.Error = "App not found"
		} else {
			data.App = &app
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "edit.html", data); err != nil {
		slog.Warn("render template", "url", r.URL.Path, "error", err)
	}
}

func (s *Server) HandleAdminSave(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}

	idStr := r.FormValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	url := r.FormValue("url")
	title := r.FormValue("title")
	description := r.FormValue("description")
	descriptionDE := r.FormValue("description_de")
	thumbnail := r.FormValue("thumbnail")
	sortOrderStr := r.FormValue("sort_order")
	sortOrder, _ := strconv.ParseInt(sortOrderStr, 10, 64)
	featured := int64(0)
	if r.FormValue("featured") != "" {
		featured = 1
	}

	q := dbgen.New(s.DB)
	ctx := r.Context()

	prompt := r.FormValue("prompt")
	repoURL := r.FormValue("repo_url")

	if id > 0 {
		err := q.UpdateApp(ctx, dbgen.UpdateAppParams{
			ID:            id,
			Url:           url,
			Title:         title,
			Description:   description,
			DescriptionDe: &descriptionDE,
			Thumbnail:     &thumbnail,
			SortOrder:     &sortOrder,
			Featured:      featured,
			Prompt:        &prompt,
			RepoUrl:       &repoURL,
		})
		if err != nil {
			slog.Warn("update app", "error", err)
		}
	} else {
		_, err := q.CreateApp(ctx, dbgen.CreateAppParams{
			Url:           url,
			Title:         title,
			Description:   description,
			DescriptionDe: &descriptionDE,
			Thumbnail:     &thumbnail,
			SortOrder:     &sortOrder,
			Featured:      featured,
			Prompt:        &prompt,
			RepoUrl:       &repoURL,
		})
		if err != nil {
			slog.Warn("create app", "error", err)
		}
	}

	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) HandleAdminDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}

	idStr := r.PathValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	if id > 0 {
		q := dbgen.New(s.DB)
		if err := q.DeleteApp(r.Context(), id); err != nil {
			slog.Warn("delete app", "error", err)
		}
	}

	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) HandleAPIApps(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.DB)
	apps, err := q.ListApps(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apps)
}

func (s *Server) HandleTrackClick(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	q := dbgen.New(s.DB)
	if err := q.IncrementClickCount(r.Context(), id); err != nil {
		slog.Warn("increment click", "error", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

func (s *Server) renderTemplate(w http.ResponseWriter, name string, data any) error {
	path := filepath.Join(s.TemplatesDir, name)
	tmpl, err := template.ParseFiles(path)
	if err != nil {
		return fmt.Errorf("parse template %q: %w", name, err)
	}
	if err := tmpl.Execute(w, data); err != nil {
		return fmt.Errorf("execute template %q: %w", name, err)
	}
	return nil
}

func (s *Server) setUpDatabase(dbPath string) error {
	wdb, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open db: %w", err)
	}
	s.DB = wdb
	if err := db.RunMigrations(wdb); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	return nil
}

func (s *Server) seedApps() error {
	ctx := context.Background()
	q := dbgen.New(s.DB)
	apps, _ := q.ListApps(ctx)
	if len(apps) > 0 {
		return nil
	}

	seedData := []dbgen.CreateAppParams{
		{
			Url:           "https://siedler-oesterreich.exe.xyz/",
			Title:         "Siedler Österreich",
			Description:   "Multiplayer browser game on real Austrian cadastral parcels. Explore, claim land, and put 30% of your municipality under nature protection.",
			DescriptionDe: ptr("Multiplayer-Browserspiel auf echten österreichischen Katasterparzellen. Erkunde, beanspruche Flächen und stelle 30% deiner Gemeinde unter Naturschutz."),
			Thumbnail:     ptr("/static/thumbs/siedler.jpg"),
			SortOrder:     ptr(int64(1)),
			Featured:      1,
			Prompt:        ptr("Build a multiplayer browser game on real Austrian cadastral data. Players explore and claim parcels in their municipality, earn points for biodiversity, and win by putting 30% of the area under nature protection."),
			RepoUrl:       ptr("https://github.com/raffopenssh/siedler--sterreich"),
		},
		{
			Url:           "https://five-megapixel-conservation.exe.xyz/",
			Title:         "5MP.globe Conservation",
			Description:   "Real-time fire detection, deforestation monitoring, and conservation effort tracking for 162 African keystone protected areas.",
			DescriptionDe: ptr("Echtzeit-Feuererkennung, Entwaldungs-Monitoring und Erfassung des Schutzaufwands für 162 afrikanische Schutzgebiete."),
			Thumbnail:     ptr("/static/thumbs/five-megapixel.jpg"),
			SortOrder:     ptr(int64(2)),
			Featured:      1,
			Prompt:        ptr("Build a conservation monitoring dashboard for African protected areas. Combine NASA FIRMS fire alerts, forest change data, and conservation effort tracks (GPX from rangers, vehicles, aircraft) on an interactive globe. Alert on new fires inside park boundaries."),
			RepoUrl:       ptr("https://github.com/raffopenssh/5mp"),
		},
		{
			Url:           "https://groundwater-at.exe.xyz/",
			Title:         "GW Power – Grundwasser Österreich",
			Description:   "Groundwater status for every Austrian cadastral municipality: level trends, precipitation divergence, nitrate pollution and drought pressure in one index.",
			DescriptionDe: ptr("Grundwasserstatus jeder österreichischen Katastralgemeinde: Pegeltrends, Niederschlagsabweichung, Nitratbelastung und Dürredruck in einem Index."),
			Thumbnail:     ptr("/static/thumbs/groundwater.jpg"),
			SortOrder:     ptr(int64(3)),
			Featured:      1,
			Prompt:        ptr("Build a drought risk map for Austria combining groundwater monitoring stations with hydropower plant locations. Show which municipalities face water stress based on declining groundwater trends and power generation dependency."),
			RepoUrl:       ptr("https://github.com/raffopenssh/austria-drought-map"),
		},
		{
			Url:           "https://holzeinschlag-at.exe.xyz/",
			Title:         "Holzeinschlag Österreich",
			Description:   "Forest loss & carbon emissions by municipality. Satellite-derived harvest data 2001-2024 with ETS carbon pricing.",
			DescriptionDe: ptr("Waldverlust & CO₂-Emissionen pro Gemeinde. Satellitengestützte Einschlagsdaten 2001-2024 mit ETS-CO₂-Bepreisung."),
			Thumbnail:     ptr("/static/thumbs/holzeinschlag.jpg"),
			SortOrder:     ptr(int64(4)),
			Prompt:        ptr("Map Austria's forest harvest by municipality using Hansen satellite data. Calculate timber volume from tree cover loss, add carbon emissions and ETS liability at current prices. Let users select years and combine municipalities."),
			RepoUrl:       ptr("https://github.com/raffopenssh/holzeinschlag-austria"),
		},
		{
			Url:           "https://landcruiser-spares.exe.xyz:8001/",
			Title:         "Land Cruiser 100 Blueprint",
			Description:   "3D wireframe assembly viewer for Toyota UZJ100/FZJ100. Exploded views from service manuals for parts identification.",
			DescriptionDe: ptr("3D-Drahtgitter-Viewer für Toyota UZJ100/FZJ100. Explosionszeichnungen aus Werkstatthandbüchern zur Teile-Identifikation."),
			Thumbnail:     ptr("/static/thumbs/landcruiser.jpg"),
			SortOrder:     ptr(int64(5)),
			Prompt:        ptr("Build a 3D wireframe viewer for the Toyota Land Cruiser 100 series. Extract part diagrams from service manuals, create exploded views by system (engine, transmission, suspension), let users identify and search for parts."),
			RepoUrl:       ptr("https://github.com/raffopenssh/landcruiser-100-3d"),
		},
		{
			Url:           "https://schools-at.exe.xyz/",
			Title:         "Schulqualität Österreich",
			Description:   "5,752 schools across 2,120 municipalities. Service quality ratings, class sizes, and all-day school coverage.",
			DescriptionDe: ptr("5.752 Schulen in 2.120 Gemeinden. Qualitätsbewertungen, Klassengrößen und ganztägige Schulangebote."),
			Thumbnail:     ptr("/static/thumbs/schools.jpg"),
			SortOrder:     ptr(int64(6)),
			Prompt:        ptr("Map all Austrian schools by municipality with quality indicators. Include student-teacher ratios, all-day school availability, and compare educational supply to school-age population. Help parents find schools near them."),
		},
		{
			Url:           "https://maternity-ward-closure.exe.xyz/",
			Title:         "Geburtshilfe-Erreichbarkeit",
			Description:   "Maternity ward accessibility via OSRM routing. Simulate closures to see drive time impacts on 90k women aged 15-44.",
			DescriptionDe: ptr("Erreichbarkeit von Geburtenstationen via OSRM-Routing. Simuliere Schließungen und deren Fahrzeit-Folgen für 90k Frauen (15-44)."),
			Thumbnail:     ptr("/static/thumbs/maternity.jpg"),
			SortOrder:     ptr(int64(7)),
			Prompt:        ptr("Model maternity ward accessibility in Austria using real driving times. Weight by female population 15-44, show which areas exceed 30/45 min drive times. Let users simulate ward closures and see the impact."),
			RepoUrl:       ptr("https://github.com/raffopenssh/maternity-app"),
		},
		{
			Url:           "https://child-care-access-at.exe.xyz/",
			Title:         "Kinderbetreuung Österreich",
			Description:   "9,863 childcare facilities mapped. 55% average coverage rate, 848 municipalities without infant care.",
			DescriptionDe: ptr("9.863 Betreuungseinrichtungen kartiert. 55% durchschnittliche Betreuungsquote, 848 Gemeinden ohne Kleinkindbetreuung."),
			Thumbnail:     ptr("/static/thumbs/childcare.jpg"),
			SortOrder:     ptr(int64(8)),
			Prompt:        ptr("Visualize childcare availability across Austrian municipalities. Show coverage rates, identify gaps where no infant care exists, compare facility quality indicators. Download data for analysis."),
			RepoUrl:       ptr("https://github.com/raffopenssh/childcare-austria"),
		},
		{
			Url:           "https://austria-power.exe.xyz/",
			Title:         "Netzkapazität Österreich",
			Description:   "Site analysis for PV, wind & battery at any address: yield, revenue, and grid feed-in capacity for expansion. Plus power market analytics from 3.5 years of ENTSO-E data.",
			DescriptionDe: ptr("Standortanalyse für PV, Wind & Batterie an jeder Adresse: Ertrag, Erlös und Netzkapazität für Zubau. Dazu Strommarkt-Analytics aus 3,5 Jahren ENTSO-E-Daten."),
			Thumbnail:     ptr("/static/thumbs/power.jpg?v=2"),
			SortOrder:     ptr(int64(4)),
			Featured:      1,
			Prompt:        ptr("Map Austria's grid capacity for renewables: wind turbines from Austro Control obstacle data, substations and lines from OSM/APG. Add a site analysis tool — pick any address, choose rooftop PV, PV field, wind or battery, get expected yield, revenue and solar capture rate. Include power market analytics from ENTSO-E day-ahead prices showing negative price hours."),
			RepoUrl:       ptr("https://github.com/raffopenssh/austria-grid"),
		},
		{
			Url:           "https://farm-subsidies-austria.exe.xyz/",
			Title:         "Agrarsubventionen Österreich",
			Description:   "EU farm payments 2023-2025 visualized by municipality. €2B/year across 2,053 communes — compare actual vs expected allocations.",
			DescriptionDe: ptr("EU-Agrarzahlungen 2023-2025 pro Gemeinde visualisiert. 2 Mrd. €/Jahr in 2.053 Gemeinden — tatsächliche vs. erwartete Zuteilungen."),
			Thumbnail:     ptr("/static/thumbs/farm-subsidies.jpg"),
			SortOrder:     ptr(int64(10)),
			Prompt:        ptr("Show EU farm subsidy payments by Austrian municipality. Compare actual payments to what you'd expect based on agricultural area and regional factors. Help farmers understand what programs they might qualify for."),
		},
	}

	for _, app := range seedData {
		_, err := q.CreateApp(ctx, app)
		if err != nil {
			slog.Warn("seed app", "title", app.Title, "error", err)
		}
	}
	return nil
}

func ptr[T any](v T) *T {
	return &v
}

func (s *Server) HandleSitemap(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.DB)
	apps, _ := q.ListApps(r.Context())

	w.Header().Set("Content-Type", "application/xml")
	w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"
        xmlns:xhtml="http://www.w3.org/1999/xhtml">
  <url>
    <loc>https://kohlschwarz.at:8000/</loc>
    <xhtml:link rel="alternate" hreflang="de" href="https://kohlschwarz.at:8000/"/>
    <xhtml:link rel="alternate" hreflang="en" href="https://kohlschwarz.at:8000/en"/>
    <changefreq>weekly</changefreq>
    <priority>1.0</priority>
  </url>
  <url>
    <loc>https://kohlschwarz.at:8000/en</loc>
    <xhtml:link rel="alternate" hreflang="de" href="https://kohlschwarz.at:8000/"/>
    <xhtml:link rel="alternate" hreflang="en" href="https://kohlschwarz.at:8000/en"/>
    <changefreq>weekly</changefreq>
    <priority>0.9</priority>
  </url>
  <url>
    <loc>https://kohlschwarz.at:8000/impressum</loc>
    <changefreq>monthly</changefreq>
    <priority>0.3</priority>
  </url>
  <url>
    <loc>https://kohlschwarz.at:8000/en/impressum</loc>
    <changefreq>monthly</changefreq>
    <priority>0.3</priority>
  </url>
  <url>
    <loc>https://kohlschwarz.at:8000/datenschutz</loc>
    <changefreq>monthly</changefreq>
    <priority>0.3</priority>
  </url>
  <url>
    <loc>https://kohlschwarz.at:8000/en/datenschutz</loc>
    <changefreq>monthly</changefreq>
    <priority>0.3</priority>
  </url>
`))
	for _, app := range apps {
		fmt.Fprintf(w, `  <url>
    <loc>%s</loc>
    <changefreq>monthly</changefreq>
    <priority>0.8</priority>
  </url>
`, app.Url)
	}
	w.Write([]byte(`</urlset>`))
}

func (s *Server) HandleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(`User-agent: *
Allow: /
Disallow: /admin

Sitemap: https://kohlschwarz.at:8000/sitemap.xml
`))
}

func (s *Server) HandleLLMTxt(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.DB)
	apps, _ := q.ListApps(r.Context())

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, `# Kohlschwarz

Apps built from public data with Shelley on exe.dev – exploring what good
can be done with it. Open-source, methods and data included.
All apps are interactive, browser-based, and freely accessible.

Background story: https://blog.exe.dev/meet-the-conservationist-who-turned-40-terabytes-of-government-data-into-a-video-game

## Apps

`)
	for _, app := range apps {
		fmt.Fprintf(w, "### %s\n", app.Title)
		fmt.Fprintf(w, "URL: %s\n", app.Url)
		if app.RepoUrl != nil && *app.RepoUrl != "" {
			fmt.Fprintf(w, "Source: %s\n", *app.RepoUrl)
		}
		fmt.Fprintf(w, "%s\n\n", app.Description)
	}
	fmt.Fprint(w, `## Contact

Use the contact link on https://kohlschwarz.at:8000/
GitHub: https://github.com/raffopenssh
`)
}

func (s *Server) HandleImpressum(w http.ResponseWriter, r *http.Request) {
	s.renderLegal(w, r, "impressum.html", "de")
}

func (s *Server) HandleImpressumEN(w http.ResponseWriter, r *http.Request) {
	s.renderLegal(w, r, "impressum.html", "en")
}

func (s *Server) HandleDatenschutz(w http.ResponseWriter, r *http.Request) {
	s.renderLegal(w, r, "datenschutz.html", "de")
}

func (s *Server) HandleDatenschutzEN(w http.ResponseWriter, r *http.Request) {
	s.renderLegal(w, r, "datenschutz.html", "en")
}

func (s *Server) renderLegal(w http.ResponseWriter, r *http.Request, tmpl, lang string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, tmpl, pageData{Lang: lang}); err != nil {
		slog.Warn("render template", "url", r.URL.Path, "error", err)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		// Cache static assets for 1 week, HTML for 1 hour
		if strings.HasPrefix(r.URL.Path, "/static/") {
			w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
		} else if r.URL.Path == "/sitemap.xml" || r.URL.Path == "/robots.txt" {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=300")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) Serve(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.HandleRoot)
	mux.HandleFunc("GET /en", s.HandleRootEN)
	mux.HandleFunc("GET /en/{$}", s.HandleRootEN)
	mux.HandleFunc("GET /impressum", s.HandleImpressum)
	mux.HandleFunc("GET /en/impressum", s.HandleImpressumEN)
	mux.HandleFunc("GET /datenschutz", s.HandleDatenschutz)
	mux.HandleFunc("GET /en/datenschutz", s.HandleDatenschutzEN)
	mux.HandleFunc("GET /sitemap.xml", s.HandleSitemap)
	mux.HandleFunc("GET /robots.txt", s.HandleRobots)
	mux.HandleFunc("GET /admin", s.HandleAdmin)
	mux.HandleFunc("GET /admin/edit/{id}", s.HandleAdminEdit)
	mux.HandleFunc("GET /admin/new", s.HandleAdminEdit)
	mux.HandleFunc("POST /admin/save", s.HandleAdminSave)
	mux.HandleFunc("POST /admin/delete/{id}", s.HandleAdminDelete)
	mux.HandleFunc("GET /admin/jobs", s.HandleAdminJobs)
	mux.HandleFunc("GET /admin/jobs/report.txt", s.HandleAdminJobsReport)
	mux.HandleFunc("POST /admin/jobs/fetch", s.HandleAdminJobsFetch)
	mux.HandleFunc("POST /admin/jobs/rank", s.HandleAdminJobsRank)
	mux.HandleFunc("POST /admin/jobs/email", s.HandleAdminJobsEmail)
	mux.HandleFunc("POST /admin/jobs/hide/{id}", s.HandleAdminJobsHide)
	go jobs.Scheduler(context.Background(), s.DB, adminEmail(), s.siteURL())
	mux.HandleFunc("GET /llm.txt", s.HandleLLMTxt)
	mux.HandleFunc("GET /api/apps", s.HandleAPIApps)
	mux.HandleFunc("POST /api/click/{id}", s.HandleTrackClick)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(s.StaticDir))))
	slog.Info("starting server", "addr", addr)
	return http.ListenAndServe(addr, securityHeaders(mux))
}

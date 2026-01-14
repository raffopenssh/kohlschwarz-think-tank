package srv

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"

	"srv.exe.dev/db"
	"srv.exe.dev/db/dbgen"
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
	if err := s.renderTemplate(w, "index.html", data); err != nil {
		slog.Warn("render template", "url", r.URL.Path, "error", err)
	}
}

func (s *Server) HandleAdmin(w http.ResponseWriter, r *http.Request) {
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
	idStr := r.FormValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	url := r.FormValue("url")
	title := r.FormValue("title")
	description := r.FormValue("description")
	shelleyCmd := r.FormValue("shelley_command")
	thumbnail := r.FormValue("thumbnail")
	sortOrderStr := r.FormValue("sort_order")
	sortOrder, _ := strconv.ParseInt(sortOrderStr, 10, 64)

	q := dbgen.New(s.DB)
	ctx := r.Context()

	if id > 0 {
		err := q.UpdateApp(ctx, dbgen.UpdateAppParams{
			ID:             id,
			Url:            url,
			Title:          title,
			Description:    description,
			ShelleyCommand: &shelleyCmd,
			Thumbnail:      &thumbnail,
			SortOrder:      &sortOrder,
		})
		if err != nil {
			slog.Warn("update app", "error", err)
		}
	} else {
		_, err := q.CreateApp(ctx, dbgen.CreateAppParams{
			Url:            url,
			Title:          title,
			Description:    description,
			ShelleyCommand: &shelleyCmd,
			Thumbnail:      &thumbnail,
			SortOrder:      &sortOrder,
		})
		if err != nil {
			slog.Warn("create app", "error", err)
		}
	}

	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) HandleAdminDelete(w http.ResponseWriter, r *http.Request) {
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
			Url:            "https://holzeinschlag-at.exe.xyz/",
			Title:          "Holzeinschlag Österreich",
			Description:    "Forest loss & carbon emissions by municipality. Satellite-derived harvest data 2001-2024 with ETS carbon pricing.",
			ShelleyCommand: ptr("shelley holzeinschlag-at"),
			Thumbnail:      ptr("/static/thumbs/holzeinschlag.jpg"),
			SortOrder:      ptr(int64(1)),
		},
		{
			Url:            "https://groundwater-at.exe.xyz/",
			Title:          "Drought Risk Map",
			Description:    "Groundwater levels meet hydropower. Municipality drought risk from 2,118 stations and 156 power plants.",
			ShelleyCommand: ptr("shelley groundwater-at"),
			Thumbnail:      ptr("/static/thumbs/groundwater.jpg"),
			SortOrder:      ptr(int64(2)),
		},
		{
			Url:            "https://msf-prep.exe.xyz/",
			Title:          "MSF Medical Training",
			Description:    "Interactive exam trainer based on Médecins Sans Frontières clinical guidelines. Practice protocols before deployment.",
			ShelleyCommand: ptr("shelley msf-prep"),
			Thumbnail:      ptr("/static/thumbs/msf-prep.jpg"),
			SortOrder:      ptr(int64(3)),
		},
		{
			Url:            "https://landcruiser-spares.exe.xyz:8001/",
			Title:          "Land Cruiser 100 Blueprint",
			Description:    "3D wireframe assembly viewer for Toyota UZJ100/FZJ100. Exploded views from service manuals for parts identification.",
			ShelleyCommand: ptr("shelley landcruiser-spares"),
			Thumbnail:      ptr("/static/thumbs/landcruiser.jpg"),
			SortOrder:      ptr(int64(4)),
		},
		{
			Url:            "https://schools-at.exe.xyz/",
			Title:          "Schulqualität Österreich",
			Description:    "5,752 schools across 2,120 municipalities. Service quality ratings, class sizes, and all-day school coverage.",
			ShelleyCommand: ptr("shelley schools-at"),
			Thumbnail:      ptr("/static/thumbs/schools.jpg"),
			SortOrder:      ptr(int64(5)),
		},
		{
			Url:            "https://maternity-ward-closure.exe.xyz/",
			Title:          "Geburtshilfe-Erreichbarkeit",
			Description:    "Maternity ward accessibility via OSRM routing. Simulate closures to see drive time impacts on 90k women aged 15-44.",
			ShelleyCommand: ptr("shelley maternity-ward-closure"),
			Thumbnail:      ptr("/static/thumbs/maternity.jpg"),
			SortOrder:      ptr(int64(6)),
		},
		{
			Url:            "https://child-care-access-at.exe.xyz/",
			Title:          "Kinderbetreuung Österreich",
			Description:    "9,863 childcare facilities mapped. 55% average coverage rate, 848 municipalities without infant care.",
			ShelleyCommand: ptr("shelley child-care-access-at"),
			Thumbnail:      ptr("/static/thumbs/childcare.jpg"),
			SortOrder:      ptr(int64(7)),
		},
		{
			Url:            "https://austria-power.exe.xyz/",
			Title:          "Wind Grid Capacity",
			Description:    "1,578 turbines, 441 substations, 30 GW installed. Grid feed-in capacity analysis for wind expansion.",
			ShelleyCommand: ptr("shelley austria-power"),
			Thumbnail:      ptr("/static/thumbs/power.jpg"),
			SortOrder:      ptr(int64(8)),
		},
		{
			Url:            "https://farm-subsidies-austria.exe.xyz/",
			Title:          "Agrarsubventionen Österreich",
			Description:    "€3.6B in EU farm payments visualized by municipality. Compare actual vs expected allocations across 2,117 communes.",
			ShelleyCommand: ptr("shelley farm-subsidies-austria"),
			Thumbnail:      ptr("/static/thumbs/farm-subsidies.jpg"),
			SortOrder:      ptr(int64(9)),
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

func (s *Server) Serve(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.HandleRoot)
	mux.HandleFunc("GET /admin", s.HandleAdmin)
	mux.HandleFunc("GET /admin/edit/{id}", s.HandleAdminEdit)
	mux.HandleFunc("GET /admin/new", s.HandleAdminEdit)
	mux.HandleFunc("POST /admin/save", s.HandleAdminSave)
	mux.HandleFunc("POST /admin/delete/{id}", s.HandleAdminDelete)
	mux.HandleFunc("GET /api/apps", s.HandleAPIApps)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(s.StaticDir))))
	slog.Info("starting server", "addr", addr)
	return http.ListenAndServe(addr, mux)
}

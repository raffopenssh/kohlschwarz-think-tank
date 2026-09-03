# AGENTS.md — kohlschwarz.at (Go, sqlite, no JS framework)

Public site (app showcase, DE/EN) + two private admin radars: **jobs** (`/admin/jobs`) and **funding** (`/admin/funding`).
Live: https://kohlschwarz.exe.xyz (exe.dev login) · https://kohlschwarz.at (Cloudflare → basic auth fallback).

## Build / run / deploy
```
make build && sudo systemctl restart srv      # binary = ./server (NOT ./srv — that's a source dir)
go test ./srv/...                              # dedupe tests etc.
journalctl -u srv -n 50 --no-pager
```
- `.env` holds `ADMIN_PASSWORD` (basic auth user `admin`); optional `ADMIN_EMAIL`, `ADMIN_URL`, `JOBS_LLM_BUDGET_USD`.
- Local check: `curl -u admin:$(grep ADMIN_PASSWORD .env|cut -d= -f2) localhost:8000/admin/jobs`. Headless browser can't send basic auth → save HTML to a tmp dir and serve with `busybox httpd` on a free port (8765 is often taken).
- If restart loops with "address already in use": `sudo ss -ltnp | grep :8000` and kill the orphan `server`.
- Templates are parsed per request (`renderTemplate`, FuncMap: `runAgo`) → template/CSS/JS edits need no rebuild, Go edits do. Bump `?v=` on `radar.css`/`radar.js` links after changes.
- Migrations: `db/migrations/NNN-name.sql`, applied at startup; end with `INSERT OR IGNORE INTO migrations …`. Latest: 009 (settings k/v).
- Commit with `git add <files>` explicitly (blind `git add -A` is blocked).

## Layout
```
cmd/srv/main.go            entrypoint
srv/srv.go                 routes (mux.HandleFunc), auth (isAdmin/requireAuth), renderTemplate, scheduler start
srv/jobs_handlers.go       /admin/jobs* handlers; background runs via startBackground + jobs.Current
srv/funding_handlers.go    /admin/funding*
srv/jobs/                  sources.go (71 feeds) · fetch.go · match.go (keyword filter) · rank.go (LLM, budget)
                           dedupe.go (union-find: canonical URL + org|title + synonym Jaccard) · report.go (weekly email, Scheduler)
                           status.go (Activity slot, Ago()) · store.go (Row, Run, sql)
srv/funding/               seed.go (hand-curated entries) · verified.go · store.go · verification-*/ (raw notes)
srv/templates/*.html       jobs.html + funding.html share static/radar.css + radar.js (chips, collapse, live status poll)
db/                        sqlite open + migrations; dbgen = sqlc output for public-site tables only (radars use raw sql)
```

## Jobs radar conventions
- Pipeline: `FetchAll` (upsert by url) → `RankPending` (muse-glimmer via exe.dev gateway, cap `MaxMonthUSD`) → `WeeklyReport` (Mon 04:00 UTC; daily fetch). Only one of fetch/rank/email runs at a time (`jobs.Current.Start/Finish`); UI polls `/admin/jobs/status.json`.
- List is deduped at render time (`jobs.Dedupe`), never in the DB; merged copies show as “+N copies merged”. Add multilingual role words to `synonyms` in dedupe.go, add a case to `dedupe_test.go`.
- Every report/UI cost line must use `Cost.CostLine()`.
- Adding a source: append to `Sources` in sources.go; LinkedIn sleeps 6s between requests.

## Funding radar conventions
- Data lives in code (`seed.go`); `reseed` replaces DB rows but keeps `status`/`user_note`. Scores/deadlines are manually verified — record notes under `srv/funding/verification-<date>/`.

## Style
- Mobile-first, text-first admin UI; no frameworks, inline SVG symbols, system fonts. Descriptive commits (see `git log`).

## Access model
- **Owner** (`ADMIN_EMAIL`, or basic-auth fallback) → everything. **Viewers** (allowlist in `settings.viewer_emails`, edited at `/admin/config`) → read-only `/admin/jobs`, `/admin/funding`, `report.txt`, `status.json`; every POST and `/admin/config` stays owner-only (`requireAuth`). Handlers use `requireViewer` → `(ok, owner)`; templates hide controls with `{{if .Owner}}`.

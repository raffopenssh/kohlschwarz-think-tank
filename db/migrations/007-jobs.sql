-- Job radar: national park director / protected area senior roles & consultancies
CREATE TABLE IF NOT EXISTS job_postings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL UNIQUE,
    source TEXT NOT NULL,
    title TEXT NOT NULL,
    org TEXT NOT NULL DEFAULT '',
    location TEXT NOT NULL DEFAULT '',
    snippet TEXT NOT NULL DEFAULT '',
    lang TEXT NOT NULL DEFAULT '',
    region TEXT NOT NULL DEFAULT '',      -- austria | ssa | global | other (heuristic, LLM may override)
    kind TEXT NOT NULL DEFAULT '',        -- director | senior | consultancy | other (LLM)
    posted TEXT NOT NULL DEFAULT '',      -- YYYY-MM-DD if known
    deadline TEXT NOT NULL DEFAULT '',    -- YYYY-MM-DD if known
    first_seen TEXT NOT NULL DEFAULT (datetime('now')),
    last_seen TEXT NOT NULL DEFAULT (datetime('now')),
    score INTEGER,                        -- 0-100 by LLM, NULL = not yet ranked
    why TEXT NOT NULL DEFAULT '',
    scored_at TEXT,
    reported_at TEXT,                     -- when it was included in an email report
    hidden INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_job_postings_score ON job_postings(score DESC, first_seen DESC);

CREATE TABLE IF NOT EXISTS job_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    started TEXT NOT NULL DEFAULT (datetime('now')),
    finished TEXT,
    kind TEXT NOT NULL DEFAULT 'fetch',   -- fetch | rank | email
    sources_ok INTEGER NOT NULL DEFAULT 0,
    sources_err INTEGER NOT NULL DEFAULT 0,
    found INTEGER NOT NULL DEFAULT 0,
    matched INTEGER NOT NULL DEFAULT 0,
    new_count INTEGER NOT NULL DEFAULT 0,
    ranked INTEGER NOT NULL DEFAULT 0,
    llm_model TEXT NOT NULL DEFAULT '',
    llm_in_tokens INTEGER NOT NULL DEFAULT 0,
    llm_out_tokens INTEGER NOT NULL DEFAULT 0,
    llm_cost_usd REAL NOT NULL DEFAULT 0,
    log TEXT NOT NULL DEFAULT ''
);

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (007, '007-jobs');

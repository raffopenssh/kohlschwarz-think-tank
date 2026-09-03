-- Funding radar: grants, prizes, accelerators, investors for Veridical Earth / kohlschwarz apps.
-- Curated list seeded from code (srv/funding/seed.go); status & notes editable in /admin/funding.
CREATE TABLE IF NOT EXISTS funding (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key TEXT NOT NULL UNIQUE,             -- stable slug used for seed upserts
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    kind TEXT NOT NULL DEFAULT '',        -- grant | prize | accelerator | equity | program | loan
    track TEXT NOT NULL DEFAULT '',       -- at | eu | space | conservation | prize | vc | africa
    amount TEXT NOT NULL DEFAULT '',
    deadline TEXT NOT NULL DEFAULT '',    -- YYYY-MM-DD when known, else ''
    deadline_note TEXT NOT NULL DEFAULT '',-- 'rolling', 'annual ~Oct, verify', ...
    eligibility TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    score INTEGER NOT NULL DEFAULT 0,     -- 0-100 suitability, scored once by hand
    why TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'open',  -- open | applied | rejected | won | skip
    user_note TEXT NOT NULL DEFAULT '',
    seeded_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_funding_score ON funding(score DESC, deadline);

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (008, '008-funding');

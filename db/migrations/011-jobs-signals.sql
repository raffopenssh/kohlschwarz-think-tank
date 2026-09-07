-- Hiring-difficulty signals: page re-checks (applicants, salary, closed marker)
-- and a change history per posting (deadline extended, reposted, reappeared, …).
ALTER TABLE job_postings ADD COLUMN salary TEXT NOT NULL DEFAULT '';
ALTER TABLE job_postings ADD COLUMN applicants INTEGER;      -- LinkedIn public count, NULL = unknown
ALTER TABLE job_postings ADD COLUMN checked_at TEXT;         -- last page re-check
ALTER TABLE job_postings ADD COLUMN closed_at TEXT;          -- page says closed / 404, NULL = live
ALTER TABLE job_postings ADD COLUMN reposted INTEGER NOT NULL DEFAULT 0; -- source marks it re-advertised

CREATE TABLE IF NOT EXISTS job_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    posting_id INTEGER NOT NULL REFERENCES job_postings(id) ON DELETE CASCADE,
    at TEXT NOT NULL DEFAULT (datetime('now')),
    kind TEXT NOT NULL,        -- deadline_extended | deadline_shortened | salary | reposted | reappeared | closed | reopened | applicants | readvertised
    old TEXT NOT NULL DEFAULT '',
    new TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_job_events_posting ON job_events(posting_id, at);

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (011, '011-jobs-signals');

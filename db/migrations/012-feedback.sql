-- Owner feedback on radar items: thumbs up/down, free-text note, and an optional
-- one-time reason when something is trashed (jobs: hidden; grants: skip/rejected).
-- feedback_log keeps the history so the ranker can learn from it.
ALTER TABLE job_postings ADD COLUMN vote INTEGER NOT NULL DEFAULT 0;        -- 1 up, -1 down, 0 none
ALTER TABLE job_postings ADD COLUMN user_note TEXT NOT NULL DEFAULT '';
ALTER TABLE job_postings ADD COLUMN trash_reason TEXT NOT NULL DEFAULT '';  -- '' = never asked / skipped
ALTER TABLE funding ADD COLUMN vote INTEGER NOT NULL DEFAULT 0;
ALTER TABLE funding ADD COLUMN trash_reason TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS feedback_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    at TEXT NOT NULL DEFAULT (datetime('now')),
    radar TEXT NOT NULL,          -- job | grant
    item_id INTEGER NOT NULL,
    action TEXT NOT NULL,         -- up | down | clear | trash | restore | note | status
    reason TEXT NOT NULL DEFAULT '',
    detail TEXT NOT NULL DEFAULT '',   -- note text / new status
    title TEXT NOT NULL DEFAULT '',
    org TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_feedback_log_item ON feedback_log(radar, item_id, at);

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (012, '012-feedback');

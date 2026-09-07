-- LLM-written brief (what the post/tender actually is, unit, level, deadline, fit)
-- for ranked postings with score >= 35; shown in the details panel.
ALTER TABLE job_postings ADD COLUMN brief TEXT NOT NULL DEFAULT '';
ALTER TABLE job_postings ADD COLUMN briefed_at TEXT;

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (010, '010-jobs-brief');

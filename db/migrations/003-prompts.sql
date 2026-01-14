-- Add prompt field to apps
ALTER TABLE apps ADD COLUMN prompt TEXT;

-- Record execution of this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (003, '003-prompts');

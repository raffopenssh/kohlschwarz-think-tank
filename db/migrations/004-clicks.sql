-- Add click count to apps
ALTER TABLE apps ADD COLUMN click_count INTEGER DEFAULT 0;

-- Record execution of this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (004, '004-clicks');

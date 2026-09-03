-- Key/value settings editable at /admin/config (e.g. viewer allowlist).
CREATE TABLE IF NOT EXISTS settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (009, '009-settings');

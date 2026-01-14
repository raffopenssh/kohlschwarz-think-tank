-- name: ListApps :many
SELECT * FROM apps ORDER BY sort_order ASC, id ASC;

-- name: GetApp :one
SELECT * FROM apps WHERE id = ?;

-- name: CreateApp :one
INSERT INTO apps (url, title, description, shelley_command, sort_order, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING *;

-- name: UpdateApp :exec
UPDATE apps SET
    url = ?,
    title = ?,
    description = ?,
    shelley_command = ?,
    sort_order = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: DeleteApp :exec
DELETE FROM apps WHERE id = ?;

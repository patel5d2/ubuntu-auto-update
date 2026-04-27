-- name: GetHost :one
SELECT * FROM hosts
WHERE id = $1 LIMIT 1;
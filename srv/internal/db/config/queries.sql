-- name: CreateUser :one
INSERT INTO users (
    username, password_hash, created_at
) VALUES (
    $1, $2, CURRENT_TIMESTAMP
)
RETURNING *;

-- name: GetUsers :many
SELECT * FROM users
ORDER BY created_at DESC;
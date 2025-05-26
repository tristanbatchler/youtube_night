-- name: CreateUser :one
INSERT INTO users (
    name, avatar_path
) VALUES (
    $1, $2
)
RETURNING *;

-- name: GetUsers :many
SELECT * FROM users
ORDER BY name;

-- name: CreateGang :one
INSERT INTO gangs (
    name
) VALUES (
    $1
)
RETURNING *;

-- name: GetGangs :many
SELECT * FROM gangs
ORDER BY name;

-- name: AssociateUserWithGang :exec
INSERT INTO users_gangs (
    user_id, gang_id, isHost
) VALUES (
    $1, $2, $3
);
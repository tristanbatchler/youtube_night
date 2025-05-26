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

-- name: GetUserById :one
SELECT * FROM users
WHERE id = $1;

-- name: CreateGang :one
INSERT INTO gangs (
    name, entry_password_hash
) VALUES (
    $1, $2
)
RETURNING *;

-- name: GetUsersInGang :many
SELECT u.* FROM users u
JOIN users_gangs ug ON u.id = ug.user_id
WHERE ug.gang_id = $1
ORDER BY u.name;

-- name: GetGangs :many
SELECT * FROM gangs
ORDER BY name;

-- name: GetGangById :one
SELECT * FROM gangs
WHERE id = $1;

-- name: AssociateUserWithGang :exec
INSERT INTO users_gangs (
    user_id, gang_id, isHost
) VALUES (
    $1, $2, $3
);
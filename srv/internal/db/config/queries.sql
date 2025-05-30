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
    user_id, gang_id, isHost, associated_at
) VALUES (
    $1, $2, $3, CURRENT_TIMESTAMP
);

-- name: SearchGangs :many
SELECT * FROM gangs
WHERE name ILIKE '%' || $1 || '%'
ORDER BY name
LIMIT 10;

-- name: GetGangByName :one
SELECT * FROM gangs
WHERE name = $1;

-- name: CreateVideo :one
INSERT INTO videos (
    video_id, title, description, thumbnail_url, channel_name
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING *;

-- name: GetVideoByVideoId :one
SELECT * FROM videos
WHERE video_id = $1;

-- name: CreateVideoSubmission :one
INSERT INTO video_submissions (
    user_id, gang_id, video_id
) VALUES (
    $1, $2, $3
)
RETURNING *;

-- name: GetVideosSubmittedByGangIdAndUserId :many
SELECT vs.*, v.title, v.description, v.thumbnail_url, v.channel_name
FROM video_submissions vs
JOIN videos v ON vs.video_id = v.video_id
WHERE vs.gang_id = $1
AND vs.user_id = $2
ORDER BY vs.created_at DESC;

-- name: GetUsersByNameAndGangId :many
SELECT u.* FROM users u
JOIN users_gangs ug ON u.id = ug.user_id
WHERE u.name ILIKE $1
AND ug.gang_id = $2;

-- name: UpdateUserAvatar :exec
UPDATE users
SET avatar_path = $2
WHERE id = $1;

-- name: UpdateUserLastLogin :exec
UPDATE users
SET last_login = CURRENT_TIMESTAMP
WHERE id = $1;

-- name: DeleteVideoSubmission :exec
DELETE FROM video_submissions
WHERE user_id = $1
AND gang_id = $2
AND video_id = $3;
-- name: CreateRefereshToken :one
INSERT INTO refresh_tokens(token, created_at, updated_at, expires_at, user_id)
VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: GetUserFromRefreshToken :one
SELECT * FROM refresh_tokens WHERE token = $1;

-- name: RevokeToken :exec
UPDATE refresh_tokens
    SET revoked_at = $1 WHERE token = $2;
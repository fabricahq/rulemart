-- UpsertAccount adds the GitHub user's account, or updates its login and avatar to what GitHub reports now.
-- name: UpsertAccount :one
INSERT INTO accounts (github_user_id, github_login, avatar_url)
VALUES (@github_user_id, @github_login, @avatar_url)
ON CONFLICT (github_user_id) DO UPDATE
SET github_login = EXCLUDED.github_login, avatar_url = EXCLUDED.avatar_url, signed_in_at = now()
RETURNING id, github_user_id, github_login, avatar_url, created_at;

-- name: CreateSession :one
INSERT INTO sessions (token_hash, account_id, expires_at)
VALUES (@token_hash, @account_id, now() + make_interval(secs => @lifetime_seconds::bigint))
RETURNING expires_at;

-- name: DeleteSession :execrows
DELETE FROM sessions WHERE token_hash = @token_hash;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at <= now();

-- DeleteOldestSessions keeps the account's newest sessions, at most keep of them.
-- name: DeleteOldestSessions :execrows
DELETE FROM sessions s
WHERE s.account_id = @account_id
  AND s.id NOT IN (SELECT n.id FROM sessions n WHERE n.account_id = @account_id ORDER BY n.id DESC LIMIT @keep::bigint);

-- DeleteAccountSessions ends every session of the account signed in with the live session whose token hashes to
-- token_hash, in one statement, so a session that ended in the meantime ends nothing.
-- name: DeleteAccountSessions :execrows
DELETE FROM sessions
WHERE account_id = (SELECT s.account_id FROM sessions s WHERE s.token_hash = @token_hash AND s.expires_at > now());

-- DeleteAccount deletes the account signed in with the live session whose token hashes to token_hash, in one
-- statement, so a session that ended in the meantime deletes nothing.
-- name: DeleteAccount :execrows
DELETE FROM accounts
WHERE id = (SELECT s.account_id FROM sessions s WHERE s.token_hash = @token_hash AND s.expires_at > now());

-- GetSessionAccount returns the account signed in with the session whose token hashes to token_hash, while the
-- session lasts.
-- name: GetSessionAccount :one
SELECT a.id, a.github_user_id, a.github_login, a.avatar_url, a.created_at
FROM sessions s
JOIN accounts a ON a.id = s.account_id
WHERE s.token_hash = @token_hash AND s.expires_at > now();

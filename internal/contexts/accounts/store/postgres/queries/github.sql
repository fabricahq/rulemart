-- GitHub: what Rulemart read of an account's GitHub account, and the installations of the GitHub App it reads private
-- repositories through.

-- name: GetSnapshot :one
SELECT tried_at, snapshot FROM github_snapshots WHERE account_id = @account_id;

-- SaveSnapshot keeps snapshot as the account's, tried at tried_at, replacing the one it had.
-- name: SaveSnapshot :exec
INSERT INTO github_snapshots (account_id, tried_at, snapshot)
VALUES (@account_id, @tried_at, @snapshot)
ON CONFLICT (account_id) DO UPDATE SET tried_at = EXCLUDED.tried_at, snapshot = EXCLUDED.snapshot;

-- name: DeleteSnapshot :exec
DELETE FROM github_snapshots WHERE account_id = @account_id;

-- name: ListInstallations :many
SELECT installation_id, github_account FROM github_installations
WHERE account_id = @account_id
ORDER BY created_at, installation_id;

-- AddInstallation records that the account reads private repositories through the installation. Recording it again
-- changes nothing.
-- name: AddInstallation :exec
INSERT INTO github_installations (account_id, installation_id, github_account)
VALUES (@account_id, @installation_id, @github_account)
ON CONFLICT (account_id, installation_id) DO NOTHING;

-- name: DeleteAccountInstallations :exec
DELETE FROM github_installations WHERE account_id = @account_id;

-- DeleteInstallation forgets the installation for every account that read through it, and discards their snapshots,
-- which may name private repositories it can no longer read. It returns how many accounts read through it.
-- name: DeleteInstallation :execrows
WITH gone AS (
    DELETE FROM github_installations WHERE installation_id = @installation_id RETURNING account_id
)
DELETE FROM github_snapshots WHERE account_id IN (SELECT account_id FROM gone);

-- DiscardInstallationSnapshots discards the snapshots of the accounts that read through the installation, whose
-- repositories changed, so their next page reads GitHub again.
-- name: DiscardInstallationSnapshots :execrows
DELETE FROM github_snapshots
WHERE account_id IN (SELECT account_id FROM github_installations WHERE installation_id = @installation_id);

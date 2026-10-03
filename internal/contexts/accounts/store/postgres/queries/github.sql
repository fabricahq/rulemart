-- GitHub: what Rulemart read of an account's GitHub account, and the installations of the GitHub App it reads private
-- repositories through.

-- name: GetSnapshot :one
SELECT snapshot FROM github_snapshots WHERE account_id = @account_id;

-- ClaimGitHubRead claims a read of the account's GitHub account beginning at now, unless one began after
-- tried_after, and returns the account's GitHub generation, or no row when it didn't claim the read. It locks the
-- account's row, as every change to the generation or the snapshot does, so a later statement of the same transaction
-- reads the snapshot as of the claim.
-- name: ClaimGitHubRead :one
UPDATE accounts SET github_tried_at = @now
WHERE id = @account_id AND (github_tried_at IS NULL OR github_tried_at <= @tried_after)
RETURNING github_generation;

-- SaveSnapshot keeps snapshot as the account's, replacing the one it had, unless the account's
-- GitHub generation is no longer generation. It locks the account's row, so a change to the generation waits for it, or
-- it for the change, and then sees the change. It returns 1 when it kept the snapshot, and 0 when it didn't.
-- name: SaveSnapshot :execrows
INSERT INTO github_snapshots (account_id, snapshot)
SELECT id, @snapshot FROM accounts WHERE id = @account_id AND github_generation = @generation
FOR UPDATE
ON CONFLICT (account_id) DO UPDATE SET snapshot = EXCLUDED.snapshot;

-- AdvanceGitHubGeneration notes a change to the account's access, which a read under way mustn't undo. Run it before
-- discarding the snapshot, in the same transaction, so a save that waited for it sees it. The next read needn't wait
-- out the minute since the last.
-- name: AdvanceGitHubGeneration :exec
UPDATE accounts SET github_generation = github_generation + 1, github_tried_at = NULL WHERE id = @account_id;

-- name: DeleteSnapshot :exec
DELETE FROM github_snapshots WHERE account_id = @account_id;

-- AdvanceInstallationGenerations notes a change to the access of every account that reads through the installation,
-- as AdvanceGitHubGeneration does.
-- name: AdvanceInstallationGenerations :exec
UPDATE accounts SET github_generation = github_generation + 1, github_tried_at = NULL
WHERE id IN (SELECT account_id FROM github_installations WHERE installation_id = @installation_id);

-- name: ListInstallations :many
SELECT installation_id, github_account FROM github_installations
WHERE account_id = @account_id
ORDER BY created_at, installation_id;

-- AddInstallation records that the account reads private repositories through the installation. Recording it again
-- changes nothing. It returns 1 when it recorded the installation, and 0 when the account had it.
-- name: AddInstallation :execrows
INSERT INTO github_installations (account_id, installation_id, github_account)
VALUES (@account_id, @installation_id, @github_account)
ON CONFLICT (account_id, installation_id) DO NOTHING;

-- name: DeleteAccountInstallations :exec
DELETE FROM github_installations WHERE account_id = @account_id;

-- name: DeleteAccountInstallation :exec
DELETE FROM github_installations WHERE account_id = @account_id AND installation_id = @installation_id;

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

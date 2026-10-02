-- Carts: the web function adds and removes an account's cart items, and reads its cart with the catalog. A library is
-- vetted when vetted, the release's vetted keys as host:repository ID, holds its key, and listed when a listing names
-- it.

-- LockCart holds the account's row until the transaction ends, so two items added at once can't both pass the cart's
-- limit, and returns no row when there's no such account.
-- name: LockCart :one
SELECT id FROM accounts WHERE id = @account_id::bigint FOR UPDATE;

-- FindCartTarget returns the vetted or listed library owner/name, matched without regard to case, with whether vetted
-- holds it, and the ID of its item of kind at path as the library spells it, matched without regard to case and
-- preferring the item spelled exactly so: a current rule, or a group that holds current rules. found is false when the
-- library has no such item; a whole library's path is empty.
-- name: FindCartTarget :one
SELECT l.id AS library_id, l.owner, l.name,
       (l.host || ':' || l.host_repository_id = ANY (@vetted::text[]))::boolean AS vetted,
       (item.path IS NOT NULL)::boolean AS found, coalesce(item.path, '')::text AS path
FROM libraries l
CROSS JOIN LATERAL (
    SELECT CASE @kind::text
        WHEN 'library' THEN ''
        WHEN 'group' THEN (
            SELECT g.path FROM library_groups g
            WHERE g.library_id = l.id AND lower(g.path) = lower(@path::text)
              AND EXISTS (SELECT 1 FROM rules r WHERE r.group_id = g.id AND r.retired_in_release_id IS NULL)
            ORDER BY g.path = @path::text DESC, g.path LIMIT 1
        )
        WHEN 'rule' THEN (
            SELECT r.path FROM rules r
            WHERE r.library_id = l.id AND lower(r.path) = lower(@path::text) AND r.retired_in_release_id IS NULL
            ORDER BY r.path = @path::text DESC, r.path LIMIT 1
        )
    END AS path
) item
WHERE l.host = @host AND lower(l.owner) = lower(@owner) AND lower(l.name) = lower(@name)
  AND (
      l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
      OR EXISTS (SELECT 1 FROM listings s WHERE s.host = l.host AND s.host_repository_id = l.host_repository_id)
  );

-- CartHolds reports whether the account's cart holds the library's item of kind at path.
-- name: CartHolds :one
SELECT EXISTS (
    SELECT 1 FROM cart_items
    WHERE account_id = @account_id::bigint AND library_id = @library_id::bigint AND kind = @kind AND path = @path
)::boolean AS holds;

-- AddCartItem adds the library's item of kind at path to the account's cart, confirmed as unvetted now when confirmed
-- is true. An item the cart holds already keeps its place, and gains the confirmation it lacked.
-- name: AddCartItem :exec
INSERT INTO cart_items (account_id, library_id, kind, path, unvetted_confirmed_at)
VALUES (@account_id::bigint, @library_id::bigint, @kind, @path, CASE WHEN @confirmed::boolean THEN now() END)
ON CONFLICT (account_id, library_id, kind, path) DO UPDATE
SET unvetted_confirmed_at = coalesce(cart_items.unvetted_confirmed_at, excluded.unvetted_confirmed_at);

-- RemoveCartItem removes the account's item of kind at path from the library owner/name, each matched without regard
-- to case.
-- name: RemoveCartItem :exec
DELETE FROM cart_items c
USING libraries l
WHERE c.library_id = l.id AND c.account_id = @account_id::bigint
  AND l.host = @host AND lower(l.owner) = lower(@owner) AND lower(l.name) = lower(@name)
  AND c.kind = @kind AND lower(c.path) = lower(@path);

-- EmptyCart removes every item from the account's cart.
-- name: EmptyCart :exec
DELETE FROM cart_items WHERE account_id = @account_id::bigint;

-- CountCartItems returns how many items the account's cart holds.
-- name: CountCartItems :one
SELECT count(*) FROM cart_items WHERE account_id = @account_id::bigint;

-- ListLibraryCartItems returns the account's items from the library owner/name, matched without regard to case.
-- name: ListLibraryCartItems :many
SELECT l.owner, l.name, c.kind, c.path
FROM cart_items c
JOIN libraries l ON l.id = c.library_id
WHERE c.account_id = @account_id::bigint
  AND l.host = @host AND lower(l.owner) = lower(@owner) AND lower(l.name) = lower(@name)
ORDER BY c.kind, c.path COLLATE "C";

-- ListCartItems returns the account's cart items, by library in owner and name order, the whole library first, then
-- groups, then rules, each in ID order. Each has its library, whether vetted holds it and a listing names it, and its
-- latest release; a rule's group, its newest version's title, and the release that retired it, if one did, when the
-- library still has it; and the current rules of a group or whole library.
-- name: ListCartItems :many
SELECT c.kind, c.path, (c.unvetted_confirmed_at IS NOT NULL)::boolean AS confirmed, c.added_at,
       l.owner, l.name, l.owner_avatar_url,
       (l.host || ':' || l.host_repository_id = ANY (@vetted::text[]))::boolean AS vetted,
       EXISTS (SELECT 1 FROM listings s WHERE s.host = l.host AND s.host_repository_id = l.host_repository_id)::boolean
           AS listed,
       latest.number AS latest_release,
       (rule.group_path IS NOT NULL)::boolean AS rule_found,
       coalesce(rule.group_path, '')::text AS rule_group,
       coalesce(rule.title, '')::text AS rule_title,
       coalesce(rule.retired_in, 0)::integer AS rule_retired_in,
       coalesce(current.rule_count, 0)::integer AS current_rules
FROM cart_items c
JOIN libraries l ON l.id = c.library_id
JOIN LATERAL (
    SELECT number FROM library_releases WHERE library_id = l.id ORDER BY number DESC LIMIT 1
) latest ON true
LEFT JOIN LATERAL (
    SELECT g.path AS group_path, retired.number AS retired_in, (
        SELECT v.title FROM rule_versions v JOIN library_releases p ON p.id = v.release_id
        WHERE v.rule_id = r.id ORDER BY p.number DESC LIMIT 1
    ) AS title
    FROM rules r
    JOIN library_groups g ON g.id = r.group_id
    LEFT JOIN library_releases retired ON retired.id = r.retired_in_release_id
    WHERE c.kind = 'rule' AND r.library_id = l.id AND r.path = c.path
) rule ON true
LEFT JOIN LATERAL (
    SELECT count(*) AS rule_count
    FROM rules r
    JOIN library_groups g ON g.id = r.group_id
    WHERE c.kind <> 'rule' AND r.library_id = l.id AND r.retired_in_release_id IS NULL
      AND (c.kind = 'library' OR g.path = c.path)
) current ON true
WHERE c.account_id = @account_id::bigint
ORDER BY lower(l.owner), lower(l.name), l.id, CASE c.kind WHEN 'library' THEN 0 WHEN 'group' THEN 1 ELSE 2 END,
         c.path COLLATE "C";

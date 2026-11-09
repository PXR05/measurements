-- name: ListConfigResources :many
SELECT id, body
FROM resources
WHERE status = 200 AND id IN (
    SELECT value FROM json_each(sqlc.arg(resource_ids))
);

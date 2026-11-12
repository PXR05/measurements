-- name: ListIEMs :many
SELECT brand_key,
    iem_key,
    brand AS brand_name,
    name,
    count(DISTINCT source_id) AS source_count
FROM phone_catalog
WHERE instr(iem_key, lower(sqlc.arg(search_text))) > 0
    AND (sqlc.arg(brands) = '[]' OR brand_key IN (SELECT value FROM json_each(sqlc.arg(brands))))
GROUP BY brand_key, iem_key
ORDER BY
    CASE WHEN sqlc.arg(direction) = 'asc' THEN
        CASE sqlc.arg(sort)
            WHEN 'name' THEN iem_key
            WHEN 'brand' THEN brand_key
            WHEN 'sources' THEN count(DISTINCT source_id)
        END
    END ASC,
    CASE WHEN sqlc.arg(direction) = 'desc' THEN
        CASE sqlc.arg(sort)
            WHEN 'name' THEN iem_key
            WHEN 'brand' THEN brand_key
            WHEN 'sources' THEN count(DISTINCT source_id)
        END
    END DESC,
    brand_key, iem_key
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountIEMs :one
SELECT count(*) FROM (
    SELECT 1
    FROM phone_catalog
    WHERE instr(iem_key, lower(sqlc.arg(search_text))) > 0
        AND (sqlc.arg(brands) = '[]' OR brand_key IN (SELECT value FROM json_each(sqlc.arg(brands))))
    GROUP BY brand_key, iem_key
);

-- name: GetIEM :one
SELECT brand_key,
    iem_key,
    brand AS brand_name,
    name,
    count(DISTINCT source_id) AS source_count
FROM phone_catalog
WHERE brand_key = sqlc.arg(brand_key) AND iem_key = sqlc.arg(iem_key)
GROUP BY brand_key, iem_key;

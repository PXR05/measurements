-- name: ListBrands :many
SELECT brand_key,
    brand AS name,
    count(DISTINCT iem_key) AS iem_count,
    count(DISTINCT source_id) AS source_count
FROM phone_catalog
WHERE instr(brand_key, lower(sqlc.arg(search_text))) > 0
GROUP BY brand_key
ORDER BY
    CASE WHEN sqlc.arg(direction) = 'asc' THEN brand_key END ASC,
    CASE WHEN sqlc.arg(direction) = 'desc' THEN brand_key END DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountBrands :one
SELECT count(DISTINCT brand_key)
FROM phone_catalog
WHERE instr(brand_key, lower(sqlc.arg(search_text))) > 0;

-- name: BrandExists :one
SELECT EXISTS(
    SELECT 1 FROM phone_catalog
    WHERE brand_key = sqlc.arg(brand_key)
);

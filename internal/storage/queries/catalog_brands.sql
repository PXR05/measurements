-- name: ListBrands :many
WITH params AS (
    SELECT CAST(sqlc.arg(search_text) AS TEXT) AS search_text,
        CAST(sqlc.arg(direction) AS TEXT) AS direction,
        CAST(sqlc.arg(page_limit) AS INTEGER) AS page_limit,
        CAST(sqlc.arg(page_offset) AS INTEGER) AS page_offset
)
SELECT brand_key,
    CAST(min(trim(brand)) AS TEXT) AS name,
    count(DISTINCT iem_key) AS iem_count,
    count(DISTINCT source_key) AS source_count,
    count(DISTINCT CASE WHEN point_count > 0 THEN measurement_id END) AS measurement_count
FROM params CROSS JOIN api_current_catalog
WHERE history = 0 AND instr(brand_key, lower(params.search_text)) > 0
GROUP BY brand_key
ORDER BY
    CASE WHEN direction = 'asc' THEN brand_key END ASC,
    CASE WHEN direction = 'desc' THEN brand_key END DESC
LIMIT (SELECT page_limit FROM params)
OFFSET (SELECT page_offset FROM params);

-- name: CountBrands :one
WITH params AS (
    SELECT CAST(sqlc.arg(search_text) AS TEXT) AS search_text
)
SELECT count(DISTINCT brand_key)
FROM params CROSS JOIN api_current_phone_catalog
WHERE history = 0 AND instr(brand_key, lower(params.search_text)) > 0;

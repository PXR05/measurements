-- name: ListBrands :many
WITH params AS (
    SELECT CAST(sqlc.arg(search_text) AS TEXT) AS search_text,
        CAST(sqlc.arg(direction) AS TEXT) AS direction,
        CAST(sqlc.arg(page_limit) AS INTEGER) AS page_limit,
        CAST(sqlc.arg(page_offset) AS INTEGER) AS page_offset
), catalog AS (
    SELECT brand_key, brand, iem_key, source_key, record_id
    FROM params CROSS JOIN api_current_phone_catalog
    WHERE instr(brand_key, lower(params.search_text)) > 0
    LIMIT -1
), page AS (
    SELECT brand_key,
        CAST(min(trim(brand)) AS TEXT) AS name,
        count(DISTINCT iem_key) AS iem_count,
        count(DISTINCT source_key) AS source_count
    FROM catalog
    GROUP BY brand_key
    ORDER BY
        CASE WHEN (SELECT direction FROM params) = 'asc' THEN brand_key END ASC,
        CASE WHEN (SELECT direction FROM params) = 'desc' THEN brand_key END DESC
    LIMIT (SELECT page_limit FROM params) OFFSET (SELECT page_offset FROM params)
)
SELECT page.brand_key, page.name, page.iem_count, page.source_count,
    (SELECT count(DISTINCT m.id)
     FROM catalog phone
     CROSS JOIN variants v ON v.phone_id = phone.record_id
     CROSS JOIN measurements m ON m.variant_id = v.id AND m.id = (
         SELECT latest.id FROM measurements latest
         JOIN resources r ON r.id = latest.resource_id
         WHERE latest.variant_id = v.id AND latest.channel = m.channel AND latest.sample = m.sample
         ORDER BY julianday(r.fetched_at) DESC, latest.id DESC
         LIMIT 1
     )
     WHERE phone.brand_key = page.brand_key AND m.point_count > 0) AS measurement_count
FROM page CROSS JOIN params
ORDER BY
    CASE WHEN direction = 'asc' THEN page.brand_key END ASC,
    CASE WHEN direction = 'desc' THEN page.brand_key END DESC;

-- name: CountBrands :one
WITH params AS (
    SELECT CAST(sqlc.arg(search_text) AS TEXT) AS search_text
)
SELECT count(DISTINCT brand_key)
FROM params CROSS JOIN api_current_phone_catalog
WHERE history = 0 AND instr(brand_key, lower(params.search_text)) > 0;

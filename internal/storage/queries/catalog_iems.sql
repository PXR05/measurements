-- name: ListIEMs :many
WITH params AS (
    SELECT CAST(sqlc.arg(search_text) AS TEXT) AS search_text,
        CAST(sqlc.arg(brands) AS TEXT) AS brands,
        CAST(sqlc.narg(has_measurements) AS INTEGER) AS has_measurements,
        CAST(sqlc.arg(sort) AS TEXT) AS sort,
        CAST(sqlc.arg(direction) AS TEXT) AS direction,
        CAST(sqlc.arg(page_limit) AS INTEGER) AS page_limit,
        CAST(sqlc.arg(page_offset) AS INTEGER) AS page_offset
), catalog AS (
    SELECT brand_key, iem_key, brand, name, source_key, record_id
    FROM params CROSS JOIN api_current_phone_catalog
    WHERE instr(lower(name), lower(params.search_text)) > 0
        AND (params.brands = '[]' OR brand_key IN (SELECT value FROM json_each(params.brands)))
    LIMIT -1
), summaries AS (
    SELECT brand_key, iem_key,
        CAST(min(trim(brand)) AS TEXT) AS brand_name,
        CAST(min(trim(name)) AS TEXT) AS name,
        count(DISTINCT source_key) AS source_count
    FROM params CROSS JOIN catalog
    GROUP BY brand_key, iem_key
), ordered AS (
    SELECT summaries.*,
        CASE params.sort
            WHEN 'name' THEN name
            WHEN 'brand' THEN brand_name
            WHEN 'sources' THEN source_count
        END AS sort_value
    FROM summaries CROSS JOIN params
), page AS (
    SELECT ordered.* FROM ordered CROSS JOIN params
    ORDER BY
        CASE WHEN direction = 'asc' THEN sort_value END COLLATE NOCASE ASC,
        CASE WHEN direction = 'desc' THEN sort_value END COLLATE NOCASE DESC,
        brand_key, iem_key
    LIMIT (SELECT page_limit FROM params) OFFSET (SELECT page_offset FROM params)
)
SELECT page.brand_key, page.iem_key, page.brand_name, page.name, page.source_count,
    (SELECT count(DISTINCT v.id)
     FROM catalog phone
     JOIN variants v ON v.phone_id = phone.record_id
     WHERE phone.brand_key = page.brand_key AND phone.iem_key = page.iem_key) AS variant_count,
    CAST((
        SELECT count(DISTINCT m.id)
        FROM catalog c
        JOIN variants v ON v.phone_id = c.record_id
        CROSS JOIN measurements m ON m.variant_id = v.id AND m.id = (
            SELECT latest.id FROM measurements latest
            JOIN resources r ON r.id = latest.resource_id
            WHERE latest.variant_id = v.id AND latest.channel = m.channel AND latest.sample = m.sample
            ORDER BY julianday(r.fetched_at) DESC, latest.id DESC LIMIT 1
        )
        WHERE c.brand_key = page.brand_key AND c.iem_key = page.iem_key
            AND m.point_count > 0
    ) AS INTEGER) AS measurement_count
FROM page CROSS JOIN params
ORDER BY
    CASE WHEN direction = 'asc' THEN sort_value END COLLATE NOCASE ASC,
    CASE WHEN direction = 'desc' THEN sort_value END COLLATE NOCASE DESC,
    page.brand_key, page.iem_key;

-- name: CountIEMs :one
WITH params AS (
    SELECT CAST(sqlc.arg(search_text) AS TEXT) AS search_text,
        CAST(sqlc.arg(brands) AS TEXT) AS brands,
        CAST(sqlc.narg(has_measurements) AS INTEGER) AS has_measurements
)
SELECT CAST(CASE WHEN (SELECT has_measurements FROM params) IS NULL THEN (
    SELECT count(*) FROM (
        SELECT brand_key, iem_key
        FROM params CROSS JOIN api_current_phone_catalog
        WHERE instr(lower(name), lower(params.search_text)) > 0
            AND (params.brands = '[]' OR brand_key IN (SELECT value FROM json_each(params.brands)))
        GROUP BY brand_key, iem_key
    )
) ELSE (
    SELECT count(*) FROM (
        SELECT brand_key, iem_key
        FROM params CROSS JOIN api_current_catalog
        WHERE instr(lower(name), lower(params.search_text)) > 0
            AND (params.brands = '[]' OR brand_key IN (SELECT value FROM json_each(params.brands)))
        GROUP BY brand_key, iem_key
        HAVING (count(CASE WHEN point_count > 0 THEN 1 END) > 0) = params.has_measurements
    )
) END AS INTEGER) AS total;

-- name: GetIEM :one
SELECT brand_key,
    iem_key,
    CAST(min(trim(brand)) AS TEXT) AS brand_name,
    CAST(min(trim(name)) AS TEXT) AS name,
    count(DISTINCT source_key) AS source_count,
    count(DISTINCT CASE WHEN variant_id > 0 THEN variant_id END) AS variant_count,
    count(DISTINCT CASE WHEN point_count > 0 THEN measurement_id END) AS measurement_count
FROM api_current_catalog
WHERE history = 0 AND brand_key = sqlc.arg(brand_key) AND iem_key = sqlc.arg(iem_key)
GROUP BY brand_key, iem_key;

-- name: ListIEMsByMeasurements :many
WITH params AS (
    SELECT CAST(sqlc.arg(search_text) AS TEXT) AS search_text,
        CAST(sqlc.arg(brands) AS TEXT) AS brands,
        CAST(sqlc.narg(has_measurements) AS INTEGER) AS has_measurements,
        CAST(sqlc.arg(sort) AS TEXT) AS sort,
        CAST(sqlc.arg(direction) AS TEXT) AS direction,
        CAST(sqlc.arg(page_limit) AS INTEGER) AS page_limit,
        CAST(sqlc.arg(page_offset) AS INTEGER) AS page_offset
)
SELECT result.* FROM (
    SELECT brand_key, iem_key,
        CAST(min(trim(brand)) AS TEXT) AS brand_name,
        CAST(min(trim(name)) AS TEXT) AS name,
        count(DISTINCT source_key) AS source_count,
        count(DISTINCT CASE WHEN variant_id > 0 THEN variant_id END) AS variant_count,
        count(DISTINCT CASE WHEN point_count > 0 THEN measurement_id END) AS measurement_count
    FROM api_current_catalog
    WHERE instr(lower(name), lower((SELECT search_text FROM params))) > 0
        AND ((SELECT brands FROM params) = '[]' OR brand_key IN (SELECT value FROM json_each((SELECT brands FROM params))))
    GROUP BY brand_key, iem_key
    HAVING (SELECT has_measurements FROM params) IS NULL
        OR (count(CASE WHEN point_count > 0 THEN 1 END) > 0) = (SELECT has_measurements FROM params)
) result CROSS JOIN params
ORDER BY
    CASE WHEN direction = 'asc' THEN
        CASE sort WHEN 'name' THEN name WHEN 'brand' THEN brand_name
            WHEN 'sources' THEN source_count WHEN 'measurements' THEN measurement_count END
    END COLLATE NOCASE ASC,
    CASE WHEN direction = 'desc' THEN
        CASE sort WHEN 'name' THEN name WHEN 'brand' THEN brand_name
            WHEN 'sources' THEN source_count WHEN 'measurements' THEN measurement_count END
    END COLLATE NOCASE DESC,
    brand_key, iem_key
LIMIT (SELECT page_limit FROM params) OFFSET (SELECT page_offset FROM params);


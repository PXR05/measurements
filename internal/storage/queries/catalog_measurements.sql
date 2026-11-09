-- name: ListMeasurements :many
WITH params AS (
    SELECT CAST(sqlc.arg(brand_key) AS TEXT) AS brand,
        CAST(sqlc.arg(iem_key) AS TEXT) AS iem,
        CAST(sqlc.arg(sources) AS TEXT) AS sources,
        CAST(sqlc.arg(types) AS TEXT) AS types,
        CAST(sqlc.arg(channels) AS TEXT) AS channels,
        CAST(sqlc.arg(samples) AS TEXT) AS samples,
        CAST(sqlc.arg(min_points) AS INTEGER) AS min_points,
        CAST(sqlc.arg(sort) AS TEXT) AS sort,
        CAST(sqlc.arg(direction) AS TEXT) AS direction,
        CAST(sqlc.arg(page_limit) AS INTEGER) AS page_limit,
        CAST(sqlc.arg(page_offset) AS INTEGER) AS page_offset
)
SELECT measurement_id AS id,
    brand_key,
    iem_key,
    api_current_catalog.brand AS brand_name,
    name,
    source_id,
    source AS source_username,
    source_name,
    type AS source_type,
    folder AS source_folder,
    data_url AS source_data_url,
    variant_id,
    file_stem AS variant_file_stem,
    suffix AS variant_suffix,
    channel,
    sample,
    http_status,
    point_count,
    parse_issues,
    url,
    fetched_at,
    (SELECT pb.config_resource_id
     FROM phones p
     JOIN brands b ON b.id = p.brand_id
     JOIN phone_books pb ON pb.id = b.phone_book_id
     WHERE p.id = api_current_catalog.record_id) AS config_resource_id
FROM params CROSS JOIN api_current_catalog
WHERE history = 0 AND brand_key = params.brand AND iem_key = params.iem
    AND measurement_id > 0 AND http_status BETWEEN 200 AND 299 AND point_count >= params.min_points
    AND (params.sources = '[]' OR source COLLATE NOCASE IN (SELECT value FROM json_each(params.sources)))
    AND (params.types = '[]' OR type COLLATE NOCASE IN (SELECT value FROM json_each(params.types)))
    AND (params.channels = '[]' OR channel COLLATE NOCASE IN (SELECT value FROM json_each(params.channels)))
    AND (params.samples = '[]' OR sample COLLATE NOCASE IN (SELECT value FROM json_each(params.samples)))
ORDER BY
    CASE WHEN sort = 'source' AND direction = 'asc' THEN source END COLLATE NOCASE ASC,
    CASE WHEN sort = 'source' AND direction = 'desc' THEN source END COLLATE NOCASE DESC,
    CASE WHEN sort = 'channel' AND direction = 'asc' THEN channel END COLLATE NOCASE ASC,
    CASE WHEN sort = 'channel' AND direction = 'desc' THEN channel END COLLATE NOCASE DESC,
    CASE WHEN sort = 'sample' AND direction = 'asc' THEN sample END COLLATE NOCASE ASC,
    CASE WHEN sort = 'sample' AND direction = 'desc' THEN sample END COLLATE NOCASE DESC,
    measurement_id
LIMIT (SELECT page_limit FROM params) OFFSET (SELECT page_offset FROM params);

-- name: CountMeasurements :one
WITH params AS (
    SELECT CAST(sqlc.arg(brand_key) AS TEXT) AS brand,
        CAST(sqlc.arg(iem_key) AS TEXT) AS iem,
        CAST(sqlc.arg(sources) AS TEXT) AS sources,
        CAST(sqlc.arg(types) AS TEXT) AS types,
        CAST(sqlc.arg(channels) AS TEXT) AS channels,
        CAST(sqlc.arg(samples) AS TEXT) AS samples,
        CAST(sqlc.arg(min_points) AS INTEGER) AS min_points
)
SELECT count(*)
FROM params CROSS JOIN api_current_catalog
WHERE history = 0 AND brand_key = params.brand AND iem_key = params.iem
    AND measurement_id > 0 AND http_status BETWEEN 200 AND 299 AND point_count >= params.min_points
    AND (params.sources = '[]' OR source COLLATE NOCASE IN (SELECT value FROM json_each(params.sources)))
    AND (params.types = '[]' OR type COLLATE NOCASE IN (SELECT value FROM json_each(params.types)))
    AND (params.channels = '[]' OR channel COLLATE NOCASE IN (SELECT value FROM json_each(params.channels)))
    AND (params.samples = '[]' OR sample COLLATE NOCASE IN (SELECT value FROM json_each(params.samples)));

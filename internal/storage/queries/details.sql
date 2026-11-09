-- name: ListIEMSources :many
SELECT source_id,
    source AS source_username,
    source_name,
    type AS source_type,
    folder AS source_folder,
    data_url AS source_data_url,
    record_id,
    brand,
    name,
    price,
    review_score,
    review_link,
    shop_link,
    count(DISTINCT CASE WHEN variant_id > 0 THEN variant_id END) AS variant_count,
    count(DISTINCT CASE WHEN point_count > 0 THEN measurement_id END) AS measurement_count
FROM api_current_catalog
WHERE history = 0 AND brand_key = sqlc.arg(brand_key) AND iem_key = sqlc.arg(iem_key)
GROUP BY source_id, record_id
ORDER BY source, type, source_id, record_id;

-- name: MeasurementExists :one
SELECT EXISTS(
    SELECT 1 FROM api_catalog
    WHERE measurement_id = ? AND history = 1 AND point_count > 0 AND http_status BETWEEN 200 AND 299
);

-- name: GetMeasurementRaw :one
SELECT r.body, r.sha256, r.fetched_at
FROM resources r
WHERE r.id = (
  SELECT resource_id
  FROM api_catalog
  WHERE measurement_id = ? AND point_count > 0 AND http_status BETWEEN 200 AND 299 AND history = 1 LIMIT 1
);


-- name: ListPoints :many
WITH params AS (
    SELECT CAST(sqlc.arg(measurement_id) AS INTEGER) AS p_measurement_id,
    CAST(sqlc.narg(min_frequency) AS REAL) AS p_min_frequency,
    CAST(sqlc.narg(max_frequency) AS REAL) AS p_max_frequency,
    CAST(sqlc.arg(sort) AS TEXT) AS p_sort,
    CAST(sqlc.arg(direction) AS TEXT) AS p_direction,
    CAST(sqlc.arg(page_limit) AS INTEGER) AS p_page_limit,
    CAST(sqlc.arg(page_offset) AS INTEGER) AS p_page_offset
)
SELECT measurement_points.* FROM params CROSS JOIN measurement_points
WHERE measurement_id = p_measurement_id
  AND (p_min_frequency IS NULL OR frequency_hz >= p_min_frequency)
  AND (p_max_frequency IS NULL OR frequency_hz <= p_max_frequency)
ORDER BY
  CASE WHEN p_sort = 'frequency' AND p_direction = 'asc' THEN frequency_hz END ASC,
  CASE WHEN p_sort = 'frequency' AND p_direction = 'desc' THEN frequency_hz END DESC,
  CASE WHEN p_sort = 'index' AND p_direction = 'desc' THEN ordinal END DESC,
  ordinal ASC
LIMIT (SELECT p_page_limit FROM params) OFFSET (SELECT p_page_offset FROM params);


-- name: CountPoints :one
WITH params AS (
    SELECT CAST(sqlc.arg(measurement_id) AS INTEGER) AS p_measurement_id,
    CAST(sqlc.narg(min_frequency) AS REAL) AS p_min_frequency,
    CAST(sqlc.narg(max_frequency) AS REAL) AS p_max_frequency
)
SELECT count(*) FROM params CROSS JOIN measurement_points
WHERE measurement_id = p_measurement_id
  AND (p_min_frequency IS NULL OR frequency_hz >= p_min_frequency)
  AND (p_max_frequency IS NULL OR frequency_hz <= p_max_frequency);

-- name: ListIEMSources :many
SELECT record_id,
    source_id,
    source AS username,
    source_name AS name,
    type,
    folder,
    data_url,
    config_resource_id,
    raw_brand AS brand_name,
    raw_name AS iem_name,
    price,
    review_score,
    review_link,
    shop_link
FROM phone_catalog
WHERE brand_key = sqlc.arg(brand_key) AND iem_key = sqlc.arg(iem_key)
ORDER BY source COLLATE NOCASE, type COLLATE NOCASE, source_id, record_id;

-- name: ListIEMMeasurements :many
SELECT m.id,
    p.record_id,
    v.suffix AS label,
    v.file_stem,
    m.channel,
    m.sample,
    m.point_count,
    mc.norm_offset_db,
    r.url,
    r.fetched_at
FROM phone_catalog p
JOIN variants v ON v.phone_id = p.record_id
JOIN measurements m ON m.variant_id = v.id
JOIN resources r ON r.id = m.resource_id
LEFT JOIN measurement_curves mc ON mc.measurement_id = m.id AND length(mc.curve) > 0
WHERE p.brand_key = sqlc.arg(brand_key) AND p.iem_key = sqlc.arg(iem_key)
    AND m.point_count > 0 AND r.status BETWEEN 200 AND 299
    AND m.id = (
        SELECT max(m2.id) FROM measurements m2
        WHERE m2.variant_id = v.id AND m2.channel = m.channel AND m2.sample = m.sample
    )
ORDER BY v.id, m.channel COLLATE NOCASE, m.sample COLLATE NOCASE, m.id;

-- name: MeasurementExists :one
SELECT EXISTS(
    SELECT 1 FROM measurements m
    JOIN resources r ON r.id = m.resource_id
    WHERE m.id = ? AND m.point_count > 0 AND r.status BETWEEN 200 AND 299
);

-- name: GetMeasurementRaw :one
SELECT r.body, r.sha256, r.fetched_at
FROM measurements m
JOIN resources r ON r.id = m.resource_id
WHERE m.id = ? AND m.point_count > 0 AND r.status BETWEEN 200 AND 299;

-- name: ListPoints :many
SELECT measurement_id, ordinal, line_number, frequency_hz, spl_db, phase_degrees, values_json
FROM measurement_points
WHERE measurement_id = sqlc.arg(measurement_id)
  AND (sqlc.narg(min_frequency) IS NULL OR frequency_hz >= sqlc.narg(min_frequency))
  AND (sqlc.narg(max_frequency) IS NULL OR frequency_hz <= sqlc.narg(max_frequency))
ORDER BY
  CASE WHEN sqlc.arg(sort) = 'frequency' AND sqlc.arg(direction) = 'asc' THEN frequency_hz END ASC,
  CASE WHEN sqlc.arg(sort) = 'frequency' AND sqlc.arg(direction) = 'desc' THEN frequency_hz END DESC,
  CASE WHEN sqlc.arg(sort) = 'index' AND sqlc.arg(direction) = 'desc' THEN ordinal END DESC,
  ordinal ASC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountPoints :one
SELECT count(*)
FROM measurement_points
WHERE measurement_id = sqlc.arg(measurement_id)
  AND (sqlc.narg(min_frequency) IS NULL OR frequency_hz >= sqlc.narg(min_frequency))
  AND (sqlc.narg(max_frequency) IS NULL OR frequency_hz <= sqlc.narg(max_frequency));

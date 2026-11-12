-- name: HasIEMConsensus :one
SELECT EXISTS(SELECT 1 FROM iem_consensus LIMIT 1);

-- name: ListUncomputedMeasurements :many
SELECT m.id, r.body
FROM phone_catalog p
CROSS JOIN variants v ON v.phone_id = p.record_id
CROSS JOIN measurements m ON m.variant_id = v.id
CROSS JOIN resources r ON r.id = m.resource_id
WHERE m.point_count > 0 AND r.status BETWEEN 200 AND 299
  AND NOT EXISTS (SELECT 1 FROM measurement_curves mc WHERE mc.measurement_id = m.id);

-- name: UpsertMeasurementCurvesBatch :exec
INSERT INTO measurement_curves (measurement_id, norm_offset_db, curve)
SELECT
    CAST(json_extract(value, '$[0]') AS INTEGER),
    CAST(json_extract(value, '$[1]') AS REAL),
    unhex(json_extract(value, '$[2]'))
FROM json_each(sqlc.arg(rows_json))
ON CONFLICT (measurement_id) DO UPDATE SET
    norm_offset_db = excluded.norm_offset_db,
    curve = excluded.curve;

-- name: ListCatalogIEMSummaries :many
SELECT brand_key,
    iem_key,
    brand,
    name,
    count(DISTINCT source_id) AS source_count,
    avg(price_usd) AS price_usd
FROM phone_catalog
GROUP BY brand_key, iem_key;

-- name: ListCatalogMeasurementsForAnalytics :many
SELECT p.brand_key,
    p.iem_key,
    p.record_id,
    p.type,
    v.ordinal AS variant_ordinal,
    mc.curve
FROM phone_catalog p
CROSS JOIN variants v ON v.phone_id = p.record_id
CROSS JOIN measurements m ON m.variant_id = v.id
CROSS JOIN measurement_curves mc ON mc.measurement_id = m.id
WHERE m.point_count > 0 AND length(mc.curve) > 0
  AND m.id = (
      SELECT max(m2.id) FROM measurements m2
      WHERE m2.variant_id = v.id AND m2.channel = m.channel AND m2.sample = m.sample
  )
ORDER BY p.brand_key, p.iem_key, p.record_id, v.ordinal, m.id;

-- name: ClearIEMConsensus :exec
DELETE FROM iem_consensus;

-- name: InsertIEMConsensusBatch :exec
INSERT INTO iem_consensus (
    brand_key, iem_key, rig_type, source_count, measurement_count, spread_db,
    median_curve, min_curve, max_curve, p25_curve, p75_curve
)
SELECT
    json_extract(value, '$[0]'),
    json_extract(value, '$[1]'),
    json_extract(value, '$[2]'),
    CAST(json_extract(value, '$[3]') AS INTEGER),
    CAST(json_extract(value, '$[4]') AS INTEGER),
    CAST(json_extract(value, '$[5]') AS REAL),
    unhex(json_extract(value, '$[6]')),
    unhex(json_extract(value, '$[7]')),
    unhex(json_extract(value, '$[8]')),
    unhex(json_extract(value, '$[9]')),
    unhex(json_extract(value, '$[10]'))
FROM json_each(sqlc.arg(rows_json));

-- name: ClearIEMSimilarities :exec
DELETE FROM iem_similarities;

-- name: InsertIEMSimilaritiesBatch :exec
INSERT INTO iem_similarities (
    brand_key, iem_key, rig_type, rank,
    similar_brand_key, similar_iem_key, similar_brand, similar_name,
    similar_source_count, similar_price_usd, distance_db
)
SELECT
    json_extract(value, '$[0]'),
    json_extract(value, '$[1]'),
    json_extract(value, '$[2]'),
    CAST(json_extract(value, '$[3]') AS INTEGER),
    json_extract(value, '$[4]'),
    json_extract(value, '$[5]'),
    json_extract(value, '$[6]'),
    json_extract(value, '$[7]'),
    CAST(json_extract(value, '$[8]') AS INTEGER),
    CAST(json_extract(value, '$[9]') AS REAL),
    CAST(json_extract(value, '$[10]') AS REAL)
FROM json_each(sqlc.arg(rows_json));

-- name: ListIEMConsensus :many
SELECT brand_key, iem_key, rig_type, source_count, measurement_count, spread_db,
    median_curve, min_curve, max_curve, p25_curve, p75_curve
FROM iem_consensus
WHERE brand_key = sqlc.arg(brand_key) AND iem_key = sqlc.arg(iem_key)
ORDER BY rig_type;

-- name: ListIEMSimilarities :many
SELECT rig_type, rank, similar_brand_key, similar_iem_key, similar_brand, similar_name,
    similar_source_count, similar_price_usd, distance_db
FROM iem_similarities
WHERE brand_key = sqlc.arg(brand_key) AND iem_key = sqlc.arg(iem_key)
  AND (sqlc.arg(rig_type) = '' OR rig_type = sqlc.arg(rig_type))
  AND (sqlc.narg(max_price_usd) IS NULL OR (similar_price_usd IS NOT NULL AND similar_price_usd <= sqlc.narg(max_price_usd)))
ORDER BY distance_db ASC, rank ASC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountIEMSimilarities :one
SELECT count(*)
FROM iem_similarities
WHERE brand_key = sqlc.arg(brand_key) AND iem_key = sqlc.arg(iem_key)
  AND (sqlc.arg(rig_type) = '' OR rig_type = sqlc.arg(rig_type))
  AND (sqlc.narg(max_price_usd) IS NULL OR (similar_price_usd IS NOT NULL AND similar_price_usd <= sqlc.narg(max_price_usd)));

-- name: GetMeasurementCurve :one
SELECT measurement_id, norm_offset_db, curve
FROM measurement_curves
WHERE measurement_id = sqlc.arg(measurement_id) AND length(curve) > 0;

-- name: HasPhoneCatalog :one
SELECT EXISTS(SELECT 1 FROM phone_catalog LIMIT 1);

-- name: ClearPhoneCatalog :exec
DELETE FROM phone_catalog;

-- name: PopulatePhoneCatalog :exec
INSERT INTO phone_catalog (
    record_id, source_id, source, source_name, type, folder, data_url, config_resource_id,
    brand_key, iem_key, brand, name,
    raw_brand_key, raw_iem_key, raw_brand, raw_name,
    price, review_score, review_link, shop_link
)
WITH latest_books AS (
    SELECT pb.id, pb.source_id, pb.config_resource_id,
        row_number() OVER (
            PARTITION BY s.data_url, s.type
            ORDER BY julianday(r.fetched_at) DESC, pb.id DESC
        ) AS position
    FROM phone_books pb
    JOIN sources s ON s.id = pb.source_id
    JOIN resources r ON r.id = pb.resource_id
), raw_phones AS (
    SELECT p.id AS record_id,
        s.id AS source_id, st.username AS source, st.name AS source_name,
        s.type, s.folder, s.data_url, pb.config_resource_id,
        trim(b.name, char(9)||char(10)||char(13)||' ') AS raw_brand,
        trim(p.name, char(9)||char(10)||char(13)||' ') AS raw_name,
        CAST(lower(trim(b.name, char(9)||char(10)||char(13)||' ')) AS TEXT) AS raw_brand_key,
        CAST(lower(trim(p.name, char(9)||char(10)||char(13)||' ')) AS TEXT) AS raw_iem_key,
        p.price, p.review_score, p.review_link, p.shop_link
    FROM latest_books pb
    CROSS JOIN sources s ON s.id = pb.source_id
    CROSS JOIN sites st ON st.id = s.site_id
    CROSS JOIN brands b ON b.phone_book_id = pb.id
    CROSS JOIN phones p ON p.brand_id = b.id
    WHERE pb.position = 1
      AND trim(p.name, char(9)||char(10)||char(13)||' ') <> ''
      AND (lower(trim(s.type)) IN ('iem', 'iems') OR (
          lower(trim(s.type)) = '5128'
          AND instr(lower(s.data_url), '/headphone') = 0
          AND instr(lower(s.data_url), '/headset') = 0
          AND instr(lower(s.data_url), '/earbud') = 0
      ))
), resolved AS (
    SELECT phone.*,
        coalesce(a.canonical_brand, ba.canonical_brand, phone.raw_brand_key) AS brand_key,
        coalesce(a.canonical_name, phone.raw_iem_key) AS iem_key
    FROM raw_phones phone
    LEFT JOIN brand_identity_aliases ba ON ba.brand_key = phone.raw_brand_key
    LEFT JOIN iem_identity_aliases a ON a.brand_key = coalesce(ba.canonical_brand, phone.raw_brand_key)
        AND a.iem_key = phone.raw_iem_key
), canonical_brands AS (
    SELECT brand_key,
        coalesce(
            min(CASE WHEN raw_brand_key = brand_key THEN raw_brand END),
            min(raw_brand)
        ) AS brand_name
    FROM resolved
    GROUP BY brand_key
)
SELECT r.record_id, r.source_id, r.source, r.source_name,
    r.type, r.folder, r.data_url, r.config_resource_id,
    r.brand_key, r.iem_key,
    cb.brand_name AS brand,
    coalesce(
        min(CASE WHEN r.raw_iem_key = r.iem_key THEN r.raw_name END) OVER (PARTITION BY r.brand_key, r.iem_key),
        min(r.raw_name) OVER (PARTITION BY r.brand_key, r.iem_key)
    ) AS name,
    r.raw_brand_key, r.raw_iem_key, r.raw_brand, r.raw_name,
    r.price, r.review_score, r.review_link, r.shop_link
FROM resolved r
JOIN canonical_brands cb ON cb.brand_key = r.brand_key;

-- name: GetCanonicalBrand :one
SELECT canonical_brand
FROM brand_identity_aliases
WHERE brand_key = sqlc.arg(brand_key);

-- name: GetCanonicalIEM :one
SELECT canonical_brand, canonical_name
FROM iem_identity_aliases
WHERE brand_key = sqlc.arg(brand_key) AND iem_key = sqlc.arg(iem_key);

-- name: ResolveBrandsJSON :one
SELECT CAST(json_group_array(coalesce(a.canonical_brand, j.value)) AS TEXT)
FROM json_each(sqlc.arg(brands)) j
LEFT JOIN brand_identity_aliases a ON a.brand_key = j.value;

-- name: IEMExists :one
SELECT EXISTS(
    SELECT 1 FROM phone_catalog
    WHERE brand_key = sqlc.arg(brand_key) AND iem_key = sqlc.arg(iem_key)
);

-- name: ListRawPhoneCatalog :many
SELECT raw_brand_key, raw_iem_key, raw_brand, raw_name, source, type, record_id
FROM phone_catalog
ORDER BY raw_brand_key, raw_iem_key, source, record_id;

-- name: ListIdentityDecisions :many
SELECT id, kind, left_brand, left_name, right_brand, right_name
FROM iem_identity_decisions
ORDER BY id;

-- name: CountIdentityDecisions :one
SELECT count(*)
FROM iem_identity_decisions;

-- name: InsertIdentityDecision :exec
INSERT INTO iem_identity_decisions (kind, left_brand, left_name, right_brand, right_name)
VALUES (sqlc.arg(kind), sqlc.arg(left_brand), sqlc.arg(left_name), sqlc.arg(right_brand), sqlc.arg(right_name));

-- name: DeleteLatestIdentityDecision :exec
DELETE FROM iem_identity_decisions
WHERE id = (SELECT max(id) FROM iem_identity_decisions);

-- name: ClearIEMAliases :exec
DELETE FROM iem_identity_aliases;

-- name: ClearBrandAliases :exec
DELETE FROM brand_identity_aliases;

-- name: InsertBrandAlias :exec
INSERT INTO brand_identity_aliases (brand_key, canonical_brand)
VALUES (sqlc.arg(brand_key), sqlc.arg(canonical_brand));

-- name: InsertIEMAlias :exec
INSERT INTO iem_identity_aliases (brand_key, iem_key, canonical_brand, canonical_name)
VALUES (sqlc.arg(brand_key), sqlc.arg(iem_key), sqlc.arg(canonical_brand), sqlc.arg(canonical_name));

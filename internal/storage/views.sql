CREATE INDEX IF NOT EXISTS resource_timestamps ON resources(id, fetched_at);

CREATE TABLE IF NOT EXISTS iem_identity_decisions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    kind TEXT NOT NULL CHECK(kind IN ('merge', 'dismiss')),
    left_brand TEXT NOT NULL, left_name TEXT NOT NULL,
    right_brand TEXT NOT NULL, right_name TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS iem_identity_aliases (
    brand_key TEXT NOT NULL, iem_key TEXT NOT NULL,
    canonical_brand TEXT NOT NULL, canonical_name TEXT NOT NULL,
    PRIMARY KEY (brand_key, iem_key)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS brand_identity_aliases (
    brand_key TEXT PRIMARY KEY,
    canonical_brand TEXT NOT NULL
) WITHOUT ROWID;

CREATE VIEW IF NOT EXISTS api_ranked_sources AS
SELECT s.*, row_number() OVER (
    PARTITION BY s.data_url, s.type
    ORDER BY julianday(r.fetched_at) DESC, s.id DESC
) AS position
FROM sources s
JOIN sites st ON st.id = s.site_id
JOIN resources r ON r.id = st.registry_resource_id;

CREATE VIEW IF NOT EXISTS api_ranked_books AS
SELECT pb.*, s.data_url, s.type, row_number() OVER (
    PARTITION BY s.data_url, s.type
    ORDER BY julianday(r.fetched_at) DESC, pb.id DESC
) AS position
FROM phone_books pb
JOIN sources s ON s.id = pb.source_id
JOIN resources r ON r.id = pb.resource_id;

CREATE VIEW IF NOT EXISTS api_current_phones AS
SELECT 0 AS history,
    s.id AS source_id, st.username AS source, st.name AS source_name,
    s.type, s.folder, s.data_url, s.delta_ready,
    b.name AS brand, p.id AS record_id, p.name, p.price,
    p.review_score, p.review_link, p.shop_link
FROM api_ranked_sources s
CROSS JOIN api_ranked_books pb ON pb.position = 1
    AND pb.data_url = s.data_url AND pb.type = s.type
CROSS JOIN sites st ON st.id = s.site_id
CROSS JOIN brands b ON b.phone_book_id = pb.id
CROSS JOIN phones p ON p.brand_id = b.id
WHERE s.position = 1;

CREATE VIEW IF NOT EXISTS api_current_records AS
SELECT phone.*,
    CAST(coalesce(v.id, 0) AS INTEGER) AS variant_id,
    CAST(coalesce(v.file_stem, '') AS TEXT) AS file_stem,
    CAST(coalesce(v.suffix, '') AS TEXT) AS suffix,
    CAST(coalesce(m.id, 0) AS INTEGER) AS measurement_id,
    CAST(coalesce(m.channel, '') AS TEXT) AS channel,
    CAST(coalesce(m.sample, '') AS TEXT) AS sample,
    CAST(coalesce(m.resource_id, 0) AS INTEGER) AS resource_id,
    CAST(coalesce(m.point_count, 0) AS INTEGER) AS point_count,
    CAST(coalesce(nullif(m.parse_issues_json, 'null'), '[]') AS TEXT) AS parse_issues,
    CAST(coalesce(r.url, '') AS TEXT) AS url,
    CAST(coalesce(r.status, 0) AS INTEGER) AS http_status,
    CAST(coalesce(r.fetched_at, '') AS TEXT) AS fetched_at
FROM api_current_phones phone
LEFT JOIN variants v ON v.phone_id = phone.record_id
LEFT JOIN measurements m ON m.variant_id = v.id AND m.id = (
    SELECT latest.id FROM measurements latest
    JOIN resources lr ON lr.id = latest.resource_id
    WHERE latest.variant_id = v.id AND latest.channel = m.channel AND latest.sample = m.sample
    ORDER BY julianday(lr.fetched_at) DESC, latest.id DESC
    LIMIT 1
)
LEFT JOIN resources r ON r.id = m.resource_id
;

CREATE VIEW IF NOT EXISTS api_source_records AS
SELECT * FROM api_current_records
UNION ALL
SELECT 1 AS history,
    s.id AS source_id, st.username AS source, st.name AS source_name,
    s.type, s.folder, s.data_url, s.delta_ready,
    b.name AS brand, p.id AS record_id, p.name, p.price,
    p.review_score, p.review_link, p.shop_link,
    CAST(coalesce(v.id, 0) AS INTEGER) AS variant_id,
    CAST(coalesce(v.file_stem, '') AS TEXT) AS file_stem,
    CAST(coalesce(v.suffix, '') AS TEXT) AS suffix,
    CAST(coalesce(m.id, 0) AS INTEGER) AS measurement_id,
    CAST(coalesce(m.channel, '') AS TEXT) AS channel,
    CAST(coalesce(m.sample, '') AS TEXT) AS sample,
    CAST(coalesce(m.resource_id, 0) AS INTEGER) AS resource_id,
    CAST(coalesce(m.point_count, 0) AS INTEGER) AS point_count,
    CAST(coalesce(nullif(m.parse_issues_json, 'null'), '[]') AS TEXT) AS parse_issues,
    CAST(coalesce(r.url, '') AS TEXT) AS url,
    CAST(coalesce(r.status, 0) AS INTEGER) AS http_status,
    CAST(coalesce(r.fetched_at, '') AS TEXT) AS fetched_at
FROM sources s
JOIN phone_books pb ON pb.source_id = s.id
JOIN sites st ON st.id = s.site_id
JOIN brands b ON b.phone_book_id = pb.id
JOIN phones p ON p.brand_id = b.id
LEFT JOIN variants v ON v.phone_id = p.id
LEFT JOIN measurements m ON m.variant_id = v.id
LEFT JOIN resources r ON r.id = m.resource_id;

CREATE VIEW IF NOT EXISTS api_catalog AS
SELECT phone.*,
    CAST(json_array(data_url, type) AS TEXT) AS source_key,
    CAST(coalesce(a.canonical_brand, ba.canonical_brand, lower(trim(brand, char(9)||char(10)||char(13)||' '))) AS TEXT) AS brand_key,
    CAST(coalesce(a.canonical_name, lower(trim(name, char(9)||char(10)||char(13)||' '))) AS TEXT) AS iem_key
FROM api_source_records phone
LEFT JOIN brand_identity_aliases ba ON ba.brand_key = lower(trim(brand, char(9)||char(10)||char(13)||' '))
LEFT JOIN iem_identity_aliases a ON a.brand_key = coalesce(ba.canonical_brand, lower(trim(brand, char(9)||char(10)||char(13)||' ')))
    AND a.iem_key = lower(trim(name, char(9)||char(10)||char(13)||' '))
WHERE trim(name, char(9)||char(10)||char(13)||' ') <> ''
  AND (lower(trim(type)) IN ('iem', 'iems') OR (
      lower(trim(type)) = '5128'
      AND instr(lower(data_url), '/headphone') = 0
      AND instr(lower(data_url), '/headset') = 0
      AND instr(lower(data_url), '/earbud') = 0
  ));

CREATE VIEW IF NOT EXISTS api_current_catalog AS
SELECT phone.*,
    CAST(json_array(data_url, type) AS TEXT) AS source_key,
    CAST(coalesce(a.canonical_brand, ba.canonical_brand, lower(trim(brand, char(9)||char(10)||char(13)||' '))) AS TEXT) AS brand_key,
    CAST(coalesce(a.canonical_name, lower(trim(name, char(9)||char(10)||char(13)||' '))) AS TEXT) AS iem_key
FROM api_current_records phone
LEFT JOIN brand_identity_aliases ba ON ba.brand_key = lower(trim(brand, char(9)||char(10)||char(13)||' '))
LEFT JOIN iem_identity_aliases a ON a.brand_key = coalesce(ba.canonical_brand, lower(trim(brand, char(9)||char(10)||char(13)||' ')))
    AND a.iem_key = lower(trim(name, char(9)||char(10)||char(13)||' '))
WHERE trim(name, char(9)||char(10)||char(13)||' ') <> ''
  AND (lower(trim(type)) IN ('iem', 'iems') OR (
      lower(trim(type)) = '5128'
      AND instr(lower(data_url), '/headphone') = 0
      AND instr(lower(data_url), '/headset') = 0
      AND instr(lower(data_url), '/earbud') = 0
  ));

CREATE VIEW IF NOT EXISTS api_raw_phone_catalog AS
SELECT *,
    CAST(json_array(data_url, type) AS TEXT) AS source_key,
    CAST(lower(trim(brand, char(9)||char(10)||char(13)||' ')) AS TEXT) AS brand_key,
    CAST(lower(trim(name, char(9)||char(10)||char(13)||' ')) AS TEXT) AS iem_key
FROM api_current_phones
WHERE trim(name, char(9)||char(10)||char(13)||' ') <> ''
  AND (lower(trim(type)) IN ('iem', 'iems') OR (
      lower(trim(type)) = '5128'
      AND instr(lower(data_url), '/headphone') = 0
      AND instr(lower(data_url), '/headset') = 0
      AND instr(lower(data_url), '/earbud') = 0
  ));

CREATE VIEW IF NOT EXISTS api_current_phone_catalog AS
SELECT phone.history, phone.source_id, phone.source, phone.source_name,
    phone.type, phone.folder, phone.data_url, phone.delta_ready,
    phone.brand, phone.record_id, phone.name, phone.price,
    phone.review_score, phone.review_link, phone.shop_link, phone.source_key,
    CAST(coalesce(a.canonical_brand, ba.canonical_brand, phone.brand_key) AS TEXT) AS brand_key,
    CAST(coalesce(a.canonical_name, phone.iem_key) AS TEXT) AS iem_key
FROM api_raw_phone_catalog phone
LEFT JOIN brand_identity_aliases ba ON ba.brand_key = phone.brand_key
LEFT JOIN iem_identity_aliases a ON a.brand_key = coalesce(ba.canonical_brand, phone.brand_key) AND a.iem_key = phone.iem_key;

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

CREATE VIEW IF NOT EXISTS api_raw_phone_catalog AS
WITH latest_books AS (
    SELECT pb.id, pb.source_id, pb.config_resource_id,
        row_number() OVER (
            PARTITION BY s.data_url, s.type
            ORDER BY julianday(r.fetched_at) DESC, pb.id DESC
        ) AS position
    FROM phone_books pb
    JOIN sources s ON s.id = pb.source_id
    JOIN resources r ON r.id = pb.resource_id
)
SELECT s.id AS source_id, st.username AS source, st.name AS source_name,
    s.type, s.folder, s.data_url, pb.config_resource_id,
    b.name AS brand, p.id AS record_id, p.name, p.price,
    p.review_score, p.review_link, p.shop_link,
    CAST(lower(trim(b.name, char(9)||char(10)||char(13)||' ')) AS TEXT) AS brand_key,
    CAST(lower(trim(p.name, char(9)||char(10)||char(13)||' ')) AS TEXT) AS iem_key
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
  ));

CREATE TABLE IF NOT EXISTS api_current_phone_catalog (
    source_id INTEGER NOT NULL,
    source TEXT NOT NULL,
    source_name TEXT NOT NULL,
    type TEXT NOT NULL,
    folder TEXT NOT NULL,
    data_url TEXT NOT NULL,
    config_resource_id INTEGER,
    brand TEXT NOT NULL,
    raw_brand TEXT NOT NULL,
    record_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    raw_name TEXT NOT NULL,
    price TEXT NOT NULL,
    review_score TEXT NOT NULL,
    review_link TEXT NOT NULL,
    shop_link TEXT NOT NULL,
    brand_key TEXT NOT NULL,
    iem_key TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS api_catalog_brand_iem ON api_current_phone_catalog(brand_key, iem_key, source_id, brand, name);
CREATE INDEX IF NOT EXISTS api_catalog_iem ON api_current_phone_catalog(iem_key, brand_key, source_id, brand, name);

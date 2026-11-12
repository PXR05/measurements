PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS runs (
 id INTEGER PRIMARY KEY,
 started_at TEXT NOT NULL,
 finished_at TEXT,
 status TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS resources (
 id INTEGER PRIMARY KEY,
 url TEXT NOT NULL,
 sha256 TEXT NOT NULL,
 status INTEGER NOT NULL,
 final_url TEXT NOT NULL,
 fetched_at TEXT NOT NULL,
 headers_json TEXT NOT NULL,
 cache_path TEXT NOT NULL,
 body BLOB NOT NULL,
 UNIQUE(url, sha256, status)
);
CREATE TABLE IF NOT EXISTS issues (
 id INTEGER PRIMARY KEY,
 run_id INTEGER NOT NULL REFERENCES runs(id),
 url TEXT NOT NULL,
 message TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sites (
 id INTEGER PRIMARY KEY,
 registry_resource_id INTEGER NOT NULL REFERENCES resources(id),
 ordinal INTEGER NOT NULL,
 username TEXT NOT NULL,
 name TEXT NOT NULL,
 url_type TEXT NOT NULL,
 alt_domain TEXT NOT NULL,
 raw_json TEXT NOT NULL,
 UNIQUE(registry_resource_id, ordinal)
);
CREATE TABLE IF NOT EXISTS sources (
 id INTEGER PRIMARY KEY,
 site_id INTEGER NOT NULL REFERENCES sites(id),
 ordinal INTEGER NOT NULL,
 type TEXT NOT NULL,
 folder TEXT NOT NULL,
 delta_ready INTEGER NOT NULL,
 data_url TEXT NOT NULL,
 raw_json TEXT NOT NULL,
 UNIQUE(site_id, ordinal)
);
CREATE TABLE IF NOT EXISTS phone_books (
 id INTEGER PRIMARY KEY,
 source_id INTEGER NOT NULL REFERENCES sources(id),
 resource_id INTEGER NOT NULL REFERENCES resources(id),
 config_resource_id INTEGER REFERENCES resources(id),
 UNIQUE(source_id, resource_id, config_resource_id)
);
CREATE TABLE IF NOT EXISTS brands (
 id INTEGER PRIMARY KEY,
 phone_book_id INTEGER NOT NULL REFERENCES phone_books(id),
 ordinal INTEGER NOT NULL,
 name TEXT NOT NULL,
 raw_json TEXT NOT NULL,
 UNIQUE(phone_book_id, ordinal)
);
CREATE UNIQUE INDEX IF NOT EXISTS phone_book_snapshot ON phone_books(source_id,resource_id,COALESCE(config_resource_id,0));
CREATE TABLE IF NOT EXISTS phones (
 id INTEGER PRIMARY KEY,
 brand_id INTEGER NOT NULL REFERENCES brands(id),
 ordinal INTEGER NOT NULL,
 name TEXT NOT NULL,
 review_score TEXT NOT NULL,
 review_link TEXT NOT NULL,
 price TEXT NOT NULL,
 shop_link TEXT NOT NULL,
 raw_json TEXT NOT NULL,
 UNIQUE(brand_id, ordinal)
);
CREATE TABLE IF NOT EXISTS variants (
 id INTEGER PRIMARY KEY,
 phone_id INTEGER NOT NULL REFERENCES phones(id),
 ordinal INTEGER NOT NULL,
 file_stem TEXT NOT NULL,
 suffix TEXT NOT NULL,
 UNIQUE(phone_id, ordinal)
);
CREATE TABLE IF NOT EXISTS measurements (
 id INTEGER PRIMARY KEY,
 variant_id INTEGER NOT NULL REFERENCES variants(id),
 channel TEXT NOT NULL,
 sample TEXT NOT NULL,
 resource_id INTEGER NOT NULL REFERENCES resources(id),
 parse_issues_json TEXT NOT NULL,
 point_count INTEGER NOT NULL,
 UNIQUE(variant_id, channel, sample, resource_id)
);
CREATE TABLE IF NOT EXISTS measurement_points (
 measurement_id INTEGER NOT NULL REFERENCES measurements(id),
 ordinal INTEGER NOT NULL,
 line_number INTEGER NOT NULL,
 frequency_hz REAL NOT NULL,
 spl_db REAL NOT NULL,
 phase_degrees REAL,
 values_json TEXT NOT NULL,
 PRIMARY KEY(measurement_id, ordinal)
) WITHOUT ROWID;
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
CREATE TABLE IF NOT EXISTS phone_catalog (
 record_id INTEGER PRIMARY KEY REFERENCES phones(id),
 source_id INTEGER NOT NULL REFERENCES sources(id),
 source TEXT NOT NULL,
 source_name TEXT NOT NULL,
 type TEXT NOT NULL,
 folder TEXT NOT NULL,
 data_url TEXT NOT NULL,
 config_resource_id INTEGER REFERENCES resources(id),
 brand_key TEXT NOT NULL,
 iem_key TEXT NOT NULL,
 brand TEXT NOT NULL,
 name TEXT NOT NULL,
 raw_brand_key TEXT NOT NULL,
 raw_iem_key TEXT NOT NULL,
 raw_brand TEXT NOT NULL,
 raw_name TEXT NOT NULL,
 price TEXT NOT NULL,
 review_score TEXT NOT NULL,
 review_link TEXT NOT NULL,
 shop_link TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS resource_timestamps ON resources(id, fetched_at);
CREATE INDEX IF NOT EXISTS phones_name ON phones(name);
CREATE INDEX IF NOT EXISTS measurement_frequency ON measurement_points(measurement_id,frequency_hz);
CREATE INDEX IF NOT EXISTS phone_catalog_brand_iem ON phone_catalog(brand_key, iem_key, source_id, brand, name);
CREATE INDEX IF NOT EXISTS phone_catalog_iem ON phone_catalog(iem_key, brand_key, source_id, brand, name);
PRAGMA user_version = 1;

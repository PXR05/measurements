-- name: UpsertResource :one
INSERT INTO resources (url, sha256, status, final_url, fetched_at, headers_json, cache_path, body)
VALUES (@url, @sha256, @status, @final_url, @fetched_at, @headers_json, @cache_path, @body)
ON CONFLICT (url, sha256, status) DO UPDATE SET url = excluded.url
RETURNING id;

-- name: UpsertSite :one
INSERT INTO sites (registry_resource_id, ordinal, username, name, url_type, alt_domain, raw_json)
VALUES (@registry_resource_id, @ordinal, @username, @name, @url_type, @alt_domain, @raw_json)
ON CONFLICT (registry_resource_id, ordinal) DO UPDATE SET username = excluded.username
RETURNING id;

-- name: UpsertSource :one
INSERT INTO sources (site_id, ordinal, type, folder, delta_ready, data_url, raw_json)
VALUES (@site_id, @ordinal, @type, @folder, @delta_ready, @data_url, @raw_json)
ON CONFLICT (site_id, ordinal) DO UPDATE SET data_url = excluded.data_url
RETURNING id;

-- name: UpsertPhoneBook :one
INSERT INTO phone_books (source_id, resource_id, config_resource_id)
VALUES (@source_id, @resource_id, @config_resource_id)
ON CONFLICT DO UPDATE SET source_id = excluded.source_id
RETURNING id;

-- name: UpsertBrand :one
INSERT INTO brands (phone_book_id, ordinal, name, raw_json)
VALUES (@phone_book_id, @ordinal, @name, @raw_json)
ON CONFLICT (phone_book_id, ordinal) DO UPDATE SET name = excluded.name
RETURNING id;

-- name: UpsertPhone :one
INSERT INTO phones (brand_id, ordinal, name, review_score, review_link, price, shop_link, raw_json)
VALUES (@brand_id, @ordinal, @name, @review_score, @review_link, @price, @shop_link, @raw_json)
ON CONFLICT (brand_id, ordinal) DO UPDATE SET name = excluded.name
RETURNING id;

-- name: UpsertVariant :one
INSERT INTO variants (phone_id, ordinal, file_stem, suffix)
VALUES (@phone_id, @ordinal, @file_stem, @suffix)
ON CONFLICT (phone_id, ordinal) DO UPDATE SET file_stem = excluded.file_stem
RETURNING id;

-- name: CreateRun :one
INSERT INTO runs (started_at, status) VALUES (?, 'running') RETURNING id;

-- name: FinishRun :exec
UPDATE runs SET finished_at = @finished_at, status = @status WHERE id = @id;

-- name: CreateIssue :exec
INSERT INTO issues (run_id, url, message) VALUES (?, ?, ?);

-- name: HasMeasurement :one
SELECT EXISTS(SELECT 1 FROM measurements WHERE variant_id = @variant_id
    AND channel = @channel AND sample = @sample AND resource_id = @resource_id);

-- name: CreateMeasurement :one
INSERT INTO measurements (variant_id, channel, sample, resource_id, parse_issues_json, point_count)
VALUES (?, ?, ?, ?, ?, ?) RETURNING id;

-- name: InsertPoint :exec
INSERT INTO measurement_points (measurement_id, ordinal, line_number, frequency_hz, spl_db, phase_degrees, values_json)
VALUES (?, ?, ?, ?, ?, ?, ?);

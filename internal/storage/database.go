package storage

import (
	"context"
	"database/sql"
	_ "embed"
	"net/url"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed views.sql
var views string

const populateCatalogSQL = `
DELETE FROM api_current_phone_catalog;
INSERT INTO api_current_phone_catalog
WITH resolved AS (
    SELECT phone.source_id, phone.source, phone.source_name,
        phone.type, phone.folder, phone.data_url, phone.config_resource_id,
        trim(phone.brand, char(9)||char(10)||char(13)||' ') AS raw_brand,
        trim(phone.name, char(9)||char(10)||char(13)||' ') AS raw_name,
        phone.record_id, phone.price,
        phone.review_score, phone.review_link, phone.shop_link,
        phone.brand_key AS raw_brand_key,
        phone.iem_key AS raw_iem_key,
        coalesce(a.canonical_brand, ba.canonical_brand, phone.brand_key) AS brand_key,
        coalesce(a.canonical_name, phone.iem_key) AS iem_key
    FROM api_raw_phone_catalog phone
    LEFT JOIN brand_identity_aliases ba ON ba.brand_key = phone.brand_key
    LEFT JOIN iem_identity_aliases a ON a.brand_key = coalesce(ba.canonical_brand, phone.brand_key)
        AND a.iem_key = phone.iem_key
), canonical_brands AS (
    SELECT brand_key,
        coalesce(
            min(CASE WHEN raw_brand_key = brand_key THEN raw_brand END),
            min(raw_brand)
        ) AS brand_name
    FROM resolved
    GROUP BY brand_key
)
SELECT r.source_id, r.source, r.source_name,
    r.type, r.folder, r.data_url, r.config_resource_id,
    cb.brand_name AS brand,
    r.raw_brand,
    r.record_id,
    coalesce(
        min(CASE WHEN r.raw_iem_key = r.iem_key THEN r.raw_name END) OVER (PARTITION BY r.brand_key, r.iem_key),
        min(r.raw_name) OVER (PARTITION BY r.brand_key, r.iem_key)
    ) AS name,
    r.raw_name,
    r.price,
    r.review_score,
    r.review_link,
    r.shop_link,
    r.brand_key,
    r.iem_key
FROM resolved r
JOIN canonical_brands cb ON cb.brand_key = r.brand_key;
`

func Open(path string, readOnly bool) (*sql.DB, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	name := filepath.ToSlash(absolute)
	if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}

	dsn := url.URL{Scheme: "file", Path: name}
	options := dsn.Query()
	if readOnly {
		options.Set("mode", "ro")
	}
	options.Add("_pragma", "busy_timeout(5000)")

	dsn.RawQuery = options.Encode()

	return sql.Open("sqlite", dsn.String())
}

func RefreshCatalog(ctx context.Context, exec interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}) error {
	_, err := exec.ExecContext(ctx, populateCatalogSQL)
	return err
}

func InstallViews(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, name := range []string{
		"api_current_measurements",
		"api_catalog", "api_source_records", "api_current_catalog", "api_current_records",
		"api_raw_phone_catalog", "api_current_phones",
		"api_ranked_measurements", "api_ranked_books", "api_ranked_sources",
	} {
		if _, err = tx.ExecContext(ctx, "DROP VIEW IF EXISTS "+name); err != nil {
			return err
		}
	}
	_, _ = tx.ExecContext(ctx, "DROP VIEW IF EXISTS api_current_phone_catalog")
	if _, err = tx.ExecContext(ctx, "DROP TABLE IF EXISTS api_current_phone_catalog"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, views); err != nil {
		return err
	}
	if err = RefreshCatalog(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func EnsureViews(ctx context.Context, path string) error {
	writer, err := Open(path, false)
	if err != nil {
		return err
	}

	defer writer.Close()

	return InstallViews(ctx, writer)
}

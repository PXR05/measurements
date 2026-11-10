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

func InstallViews(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, name := range []string{
		"api_catalog", "api_source_records", "api_current_catalog", "api_current_records",
		"api_current_phone_catalog", "api_raw_phone_catalog", "api_current_phones",
		"api_ranked_measurements", "api_ranked_books", "api_ranked_sources",
	} {
		if _, err = tx.ExecContext(ctx, "DROP VIEW IF EXISTS "+name); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, views); err != nil {
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

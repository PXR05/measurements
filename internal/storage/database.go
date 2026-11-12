package storage

import (
	"context"
	"database/sql"
	_ "embed"
	"net/url"
	"path/filepath"
	"strings"

	"measurements/internal/storage/dbgen"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

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

func InitSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL; "+schema); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := dbgen.New(tx)
	hasCatalog, err := q.HasPhoneCatalog(ctx)
	if err != nil {
		return err
	}
	hasConsensus, err := q.HasIEMConsensus(ctx)
	if err != nil {
		return err
	}
	if !hasCatalog || !hasConsensus {
		if err = RefreshCatalog(ctx, tx); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func RefreshCatalog(ctx context.Context, db dbgen.DBTX) error {
	q := dbgen.New(db)
	if err := q.ClearPhoneCatalog(ctx); err != nil {
		return err
	}
	if err := q.PopulatePhoneCatalog(ctx); err != nil {
		return err
	}
	return RefreshAnalytics(ctx, db)
}

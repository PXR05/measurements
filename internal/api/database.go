package api

import (
	"context"
	"database/sql"
	"fmt"

	"measurements/internal/storage"
)

func OpenDatabase(ctx context.Context, path string) (*sql.DB, error) {
	db, err := storage.Open(path, true)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)

	var version int
	if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		db.Close()
		return nil, fmt.Errorf("open seed database (run cmd/seed first): %w", err)
	}

	if version != 1 {
		db.Close()
		return nil, fmt.Errorf("unsupported seed database schema %d; expected 1", version)
	}

	return db, nil
}

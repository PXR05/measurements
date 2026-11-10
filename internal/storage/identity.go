package storage

import (
	"context"
	"database/sql"
	"errors"
)

func ResolveIEM(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, brand, name string) (string, string, error) {
	var canonicalBrand, canonicalName string
	err := db.QueryRowContext(ctx, `SELECT canonical_brand FROM brand_identity_aliases WHERE brand_key=?`, brand).Scan(&canonicalBrand)
	if err == nil {
		brand = canonicalBrand
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", "", err
	}
	err = db.QueryRowContext(ctx, `SELECT canonical_brand,canonical_name FROM iem_identity_aliases WHERE brand_key=? AND iem_key=?`, brand, name).Scan(&canonicalBrand, &canonicalName)
	if errors.Is(err, sql.ErrNoRows) {
		return brand, name, nil
	}
	return canonicalBrand, canonicalName, err
}

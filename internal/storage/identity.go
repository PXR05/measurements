package storage

import (
	"context"
	"database/sql"
	"errors"
)

func ResolveBrand(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, brand string) (string, error) {
	var canonicalBrand string
	err := db.QueryRowContext(ctx, `SELECT canonical_brand FROM brand_identity_aliases WHERE brand_key=?`, brand).Scan(&canonicalBrand)
	if errors.Is(err, sql.ErrNoRows) {
		return brand, nil
	}
	return canonicalBrand, err
}

func ResolveIEM(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, brand, name string) (string, string, error) {
	var err error
	brand, err = ResolveBrand(ctx, db, brand)
	if err != nil {
		return "", "", err
	}
	var canonicalBrand, canonicalName string
	err = db.QueryRowContext(ctx, `SELECT canonical_brand,canonical_name FROM iem_identity_aliases WHERE brand_key=? AND iem_key=?`, brand, name).Scan(&canonicalBrand, &canonicalName)
	if errors.Is(err, sql.ErrNoRows) {
		return brand, name, nil
	}
	return canonicalBrand, canonicalName, err
}

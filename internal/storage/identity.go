package storage

import (
	"context"
	"database/sql"
	"errors"

	"measurements/internal/storage/dbgen"
)

func ResolveBrand(ctx context.Context, db dbgen.DBTX, brand string) (string, error) {
	canonicalBrand, err := dbgen.New(db).GetCanonicalBrand(ctx, brand)
	if errors.Is(err, sql.ErrNoRows) {
		return brand, nil
	}
	return canonicalBrand, err
}

func ResolveIEM(ctx context.Context, db dbgen.DBTX, brand, name string) (string, string, error) {
	var err error
	brand, err = ResolveBrand(ctx, db, brand)
	if err != nil {
		return "", "", err
	}
	canonical, err := dbgen.New(db).GetCanonicalIEM(ctx, dbgen.GetCanonicalIEMParams{
		BrandKey: brand,
		IEMKey:   name,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return brand, name, nil
	}
	return canonical.CanonicalBrand, canonical.CanonicalName, err
}

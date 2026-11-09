package brands

import (
	"context"

	"measurements/internal/storage/dbgen"
)

func fetchBrands(ctx context.Context, q *dbgen.Queries, params dbgen.ListBrandsParams) ([]Brand, int64, error) {
	rows, err := q.ListBrands(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountBrands(ctx, params.SearchText)
	if err != nil {
		return nil, 0, err
	}
	items := make([]Brand, 0, len(rows))
	for _, row := range rows {
		items = append(items, brandResponse(row))
	}
	return items, total, nil
}

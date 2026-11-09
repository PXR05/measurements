package shared

import (
	"context"
	"database/sql"

	"measurements/internal/storage/dbgen"
)

type Page[T any] struct {
	Data       []T        `json:"data"`
	Pagination Pagination `json:"pagination"`
}

type Pagination struct {
	Limit   int   `json:"limit"`
	Offset  int   `json:"offset"`
	Total   int64 `json:"total"`
	HasMore bool  `json:"has_more"`
}

func ReadPage[P, T any](
	ctx context.Context,
	db *sql.DB,
	options ListOptions,
	params P,
	fetch func(context.Context, *dbgen.Queries, P) ([]T, int64, error),
) (Page[T], error) {
	result := Page[T]{
		Data:       make([]T, 0),
		Pagination: Pagination{Limit: options.Limit, Offset: options.Offset},
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	items, total, err := fetch(ctx, dbgen.New(tx), params)
	if err != nil {
		return result, err
	}
	if items != nil {
		result.Data = items
	}
	result.Pagination.Total = total
	result.Pagination.HasMore = int64(options.Offset+len(items)) < total
	return result, tx.Commit()
}

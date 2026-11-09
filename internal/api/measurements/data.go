package measurements

import (
	"context"

	"measurements/internal/storage/dbgen"
)

func fetchPoints(ctx context.Context, q *dbgen.Queries, params dbgen.ListPointsParams) ([]Point, int64, error) {
	rows, err := q.ListPoints(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountPoints(ctx, dbgen.CountPointsParams{
		MeasurementID: params.MeasurementID, MinFrequency: params.MinFrequency, MaxFrequency: params.MaxFrequency,
	})
	if err != nil {
		return nil, 0, err
	}
	items := make([]Point, 0, len(rows))
	for _, row := range rows {
		items = append(items, pointResponse(row))
	}
	return items, total, nil
}

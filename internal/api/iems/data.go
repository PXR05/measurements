package iems

import (
	"context"

	"measurements/internal/storage/dbgen"
)

func fetchIEMs(ctx context.Context, q *dbgen.Queries, params dbgen.ListIEMsParams) ([]IEM, int64, error) {
	var rows []dbgen.ListIEMsRow
	var err error
	if params.Sort == "measurements" || params.HasMeasurements.Valid {
		matches, fetchErr := q.ListIEMsByMeasurements(ctx, dbgen.ListIEMsByMeasurementsParams(params))
		if fetchErr != nil {
			return nil, 0, fetchErr
		}
		for _, row := range matches {
			rows = append(rows, dbgen.ListIEMsRow(row))
		}
	} else {
		rows, err = q.ListIEMs(ctx, params)
		if err != nil {
			return nil, 0, err
		}
	}
	total, err := q.CountIEMs(ctx, dbgen.CountIEMsParams{
		SearchText: params.SearchText, Brands: params.Brands, HasMeasurements: params.HasMeasurements,
	})
	if err != nil {
		return nil, 0, err
	}
	items := make([]IEM, 0, len(rows))
	for _, row := range rows {
		items = append(items, iemResponse(row))
	}
	return items, total, nil
}

func fetchMeasurements(ctx context.Context, q *dbgen.Queries, params dbgen.ListMeasurementsParams) ([]Measurement, int64, error) {
	rows, err := q.ListMeasurements(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	configurations, err := loadConfigurations(ctx, q, rows)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountMeasurements(ctx, dbgen.CountMeasurementsParams{
		BrandKey: params.BrandKey, IEMKey: params.IEMKey,
		Sources: params.Sources, Types: params.Types, Channels: params.Channels,
		Samples: params.Samples, MinPoints: params.MinPoints,
	})
	if err != nil {
		return nil, 0, err
	}
	items := make([]Measurement, 0, len(rows))
	for _, row := range rows {
		item := measurementResponse(row)
		config := configurations[row.ConfigResourceID.Int64]
		item.RigDescription = config.rigDescription
		item.MeasurementCalibrationFile = config.calibrationFile
		items = append(items, item)
	}
	return items, total, nil
}

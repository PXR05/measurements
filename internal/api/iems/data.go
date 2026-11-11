package iems

import (
	"context"

	"measurements/internal/storage/dbgen"
)

func fetchIEMs(ctx context.Context, q *dbgen.Queries, params dbgen.ListIEMsParams) ([]IEM, int64, error) {
	rows, err := q.ListIEMs(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountIEMs(ctx, dbgen.CountIEMsParams{
		SearchText: params.SearchText, Brands: params.Brands,
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

func fetchIEMDetail(ctx context.Context, q *dbgen.Queries, brand, name string) (IEMDetail, error) {
	var detail IEMDetail
	item, err := q.GetIEM(ctx, dbgen.GetIEMParams{BrandKey: brand, IEMKey: name})
	if err != nil {
		return detail, err
	}
	sourceRows, err := q.ListIEMSources(ctx, dbgen.ListIEMSourcesParams{BrandKey: brand, IEMKey: name})
	if err != nil {
		return detail, err
	}
	configurations, err := loadConfigurations(ctx, q, sourceRows)
	if err != nil {
		return detail, err
	}
	measurementRows, err := q.ListIEMMeasurements(ctx, dbgen.ListIEMMeasurementsParams{
		BrandKey: brand,
		IEMKey:   name,
	})
	if err != nil {
		return detail, err
	}
	byRecord := make(map[int64][]Measurement, len(sourceRows))
	for _, row := range measurementRows {
		byRecord[row.RecordID] = append(byRecord[row.RecordID], measurementResponse(row))
	}
	detail = IEMDetail{
		IEM:     iemResponse(dbgen.ListIEMsRow(item)),
		Sources: make([]Source, 0, len(sourceRows)),
	}
	for _, row := range sourceRows {
		detail.Sources = append(detail.Sources, sourceResponse(row, configurations[row.ConfigResourceID.Int64], byRecord[row.RecordID]))
	}
	return detail, nil
}

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

func fetchSimilarIEMs(ctx context.Context, q *dbgen.Queries, params dbgen.ListIEMSimilaritiesParams) ([]SimilarIEM, int64, error) {
	rows, err := q.ListIEMSimilarities(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountIEMSimilarities(ctx, dbgen.CountIEMSimilaritiesParams{
		BrandKey:    params.BrandKey,
		IEMKey:      params.IEMKey,
		RigType:     params.RigType,
		MaxPriceUSD: params.MaxPriceUSD,
	})
	if err != nil {
		return nil, 0, err
	}
	items := make([]SimilarIEM, 0, len(rows))
	for _, row := range rows {
		items = append(items, similarResponse(row))
	}
	return items, total, nil
}

func fetchIEMDetail(ctx context.Context, q *dbgen.Queries, brand, name string) (IEMDetail, error) {
	var detail IEMDetail
	item, err := q.GetIEM(ctx, dbgen.GetIEMParams{BrandKey: brand, IEMKey: name})
	if err != nil {
		return detail, err
	}
	consensusRows, err := q.ListIEMConsensus(ctx, dbgen.ListIEMConsensusParams{BrandKey: brand, IEMKey: name})
	if err != nil {
		return detail, err
	}
	similarRows, err := q.ListIEMSimilarities(ctx, dbgen.ListIEMSimilaritiesParams{
		BrandKey:  brand,
		IEMKey:    name,
		PageLimit: 10,
	})
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
	consensus := make([]ConsensusCurve, 0, len(consensusRows))
	for _, row := range consensusRows {
		if c, ok := consensusResponse(row); ok {
			consensus = append(consensus, c)
		}
	}
	similar := make([]SimilarIEM, 0, len(similarRows))
	for _, row := range similarRows {
		similar = append(similar, similarResponse(row))
	}
	detail = IEMDetail{
		IEM:       iemResponse(dbgen.ListIEMsRow(item)),
		Consensus: consensus,
		Similar:   similar,
		Sources:   make([]Source, 0, len(sourceRows)),
	}
	for _, row := range sourceRows {
		detail.Sources = append(detail.Sources, sourceResponse(row, configurations[row.ConfigResourceID.Int64], byRecord[row.RecordID]))
	}
	return detail, nil
}

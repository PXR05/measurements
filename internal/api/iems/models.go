package iems

import (
	"math"

	"measurements/internal/api/brands"
	"measurements/internal/storage"
	"measurements/internal/storage/dbgen"
)

type IEMReference struct {
	ID    string           `json:"id"`
	Name  string           `json:"name"`
	Brand brands.Reference `json:"brand"`
}

type IEM struct {
	IEMReference
	PriceUSD    *float64 `json:"price_usd"`
	SourceCount int      `json:"source_count"`
}

type IEMDetail struct {
	IEM
	Consensus []ConsensusCurve `json:"consensus"`
	Similar   []SimilarIEM     `json:"similar"`
	Sources   []Source         `json:"sources"`
}

type ConsensusCurve struct {
	RigType          string           `json:"rig_type"`
	SourceCount      int              `json:"source_count"`
	MeasurementCount int              `json:"measurement_count"`
	SpreadDB         float64          `json:"spread_db"`
	Points           []ConsensusPoint `json:"points"`
}

type ConsensusPoint struct {
	FrequencyHz float64 `json:"frequency_hz"`
	MedianDB    float64 `json:"median_db"`
	MinDB       float64 `json:"min_db"`
	MaxDB       float64 `json:"max_db"`
	P25DB       float64 `json:"p25_db"`
	P75DB       float64 `json:"p75_db"`
}

type SimilarIEM struct {
	IEM
	RigType    string  `json:"rig_type"`
	DistanceDB float64 `json:"distance_db"`
}

type Source struct {
	ID              int64         `json:"id"`
	Username        string        `json:"username"`
	Name            string        `json:"name"`
	Type            string        `json:"type"`
	Folder          string        `json:"folder"`
	DataURL         string        `json:"data_url"`
	RigDescription  *string       `json:"rig_description"`
	CalibrationFile *string       `json:"calibration_file"`
	BrandName       string        `json:"brand_name"`
	IEMName         string        `json:"iem_name"`
	Price           string        `json:"price"`
	ReviewScore     string        `json:"review_score"`
	ReviewLink      string        `json:"review_link"`
	ShopLink        string        `json:"shop_link"`
	Measurements    []Measurement `json:"measurements"`
}

type Measurement struct {
	ID           int64    `json:"id"`
	Label        string   `json:"label"`
	FileStem     string   `json:"file_stem"`
	Channel      string   `json:"channel"`
	Sample       string   `json:"sample"`
	PointCount   int      `json:"point_count"`
	NormOffsetDB *float64 `json:"norm_offset_db"`
	URL          string   `json:"url"`
	FetchedAt    string   `json:"fetched_at"`
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func iemResponse(r dbgen.ListIEMsRow) IEM {
	var price *float64
	if r.PriceUSD.Valid {
		p := round2(r.PriceUSD.Float64)
		price = &p
	}
	return IEM{
		IEMReference: IEMReference{
			ID:   iemID(r.BrandKey, r.IEMKey),
			Name: r.Name,
			Brand: brands.Reference{
				ID:   brands.ID(r.BrandKey),
				Name: r.BrandName,
			},
		},
		PriceUSD:    price,
		SourceCount: int(r.SourceCount),
	}
}

func consensusResponse(r dbgen.IemConsensus) (ConsensusCurve, bool) {
	median, ok1 := storage.DecodeCurve(r.MedianCurve)
	minCurve, ok2 := storage.DecodeCurve(r.MinCurve)
	maxCurve, ok3 := storage.DecodeCurve(r.MaxCurve)
	p25Curve, ok4 := storage.DecodeCurve(r.P25Curve)
	p75Curve, ok5 := storage.DecodeCurve(r.P75Curve)
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
		return ConsensusCurve{}, false
	}
	points := make([]ConsensusPoint, storage.GridSize)
	for i := 0; i < storage.GridSize; i++ {
		points[i] = ConsensusPoint{
			FrequencyHz: storage.GridFrequency(i),
			MedianDB:    round2(float64(median[i])),
			MinDB:       round2(float64(minCurve[i])),
			MaxDB:       round2(float64(maxCurve[i])),
			P25DB:       round2(float64(p25Curve[i])),
			P75DB:       round2(float64(p75Curve[i])),
		}
	}
	return ConsensusCurve{
		RigType:          r.RigType,
		SourceCount:      int(r.SourceCount),
		MeasurementCount: int(r.MeasurementCount),
		SpreadDB:         round2(r.SpreadDB),
		Points:           points,
	}, true
}

func similarResponse(r dbgen.ListIEMSimilaritiesRow) SimilarIEM {
	var price *float64
	if r.SimilarPriceUSD.Valid {
		p := round2(r.SimilarPriceUSD.Float64)
		price = &p
	}
	return SimilarIEM{
		IEM: IEM{
			IEMReference: IEMReference{
				ID:   iemID(r.SimilarBrandKey, r.SimilarIEMKey),
				Name: r.SimilarName,
				Brand: brands.Reference{
					ID:   brands.ID(r.SimilarBrandKey),
					Name: r.SimilarBrand,
				},
			},
			PriceUSD:    price,
			SourceCount: int(r.SimilarSourceCount),
		},
		RigType:    r.RigType,
		DistanceDB: round2(r.DistanceDB),
	}
}

func sourceResponse(r dbgen.ListIEMSourcesRow, cfg sourceConfiguration, measurements []Measurement) Source {
	if measurements == nil {
		measurements = make([]Measurement, 0)
	}
	return Source{
		ID:              r.SourceID,
		Username:        r.Username,
		Name:            r.Name,
		Type:            r.Type,
		Folder:          r.Folder,
		DataURL:         r.DataURL,
		RigDescription:  cfg.rigDescription,
		CalibrationFile: cfg.calibrationFile,
		BrandName:       r.BrandName,
		IEMName:         r.IEMName,
		Price:           r.Price,
		ReviewScore:     r.ReviewScore,
		ReviewLink:      r.ReviewLink,
		ShopLink:        r.ShopLink,
		Measurements:    measurements,
	}
}

func measurementResponse(r dbgen.ListIEMMeasurementsRow) Measurement {
	var offset *float64
	if r.NormOffsetDB.Valid {
		v := round2(r.NormOffsetDB.Float64)
		offset = &v
	}
	return Measurement{
		ID:           r.ID,
		Label:        r.Label,
		FileStem:     r.FileStem,
		Channel:      r.Channel,
		Sample:       r.Sample,
		PointCount:   int(r.PointCount),
		NormOffsetDB: offset,
		URL:          r.URL,
		FetchedAt:    r.FetchedAt,
	}
}

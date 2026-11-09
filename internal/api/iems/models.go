package iems

import (
	"encoding/json"

	"measurements/internal/api/brands"
	"measurements/internal/storage/dbgen"
)

type IEMReference struct {
	ID    string           `json:"id"`
	Name  string           `json:"name"`
	Brand brands.Reference `json:"brand"`
}

type IEM struct {
	IEMReference
	SourceCount      int `json:"source_count"`
	VariantCount     int `json:"variant_count"`
	MeasurementCount int `json:"measurement_count"`
}

type IEMSource struct {
	Source           Source `json:"source"`
	RecordID         int64  `json:"record_id"`
	Brand            string `json:"brand"`
	Name             string `json:"name"`
	Price            string `json:"price"`
	ReviewScore      string `json:"review_score"`
	ReviewLink       string `json:"review_link"`
	ShopLink         string `json:"shop_link"`
	VariantCount     int    `json:"variant_count"`
	MeasurementCount int    `json:"measurement_count"`
}

type IEMDetail struct {
	IEM
	Sources []IEMSource `json:"sources"`
}

type Measurement struct {
	ID                         int64           `json:"id"`
	IEM                        IEMReference    `json:"iem"`
	Source                     Source          `json:"source"`
	Variant                    Variant         `json:"variant"`
	Channel                    string          `json:"channel"`
	Sample                     string          `json:"sample"`
	HTTPStatus                 int             `json:"http_status"`
	PointCount                 int             `json:"point_count"`
	ParseIssues                json.RawMessage `json:"parse_issues"`
	URL                        string          `json:"url"`
	FetchedAt                  string          `json:"fetched_at"`
	RigDescription             *string         `json:"rig_description"`
	MeasurementCalibrationFile *string         `json:"measurement_calibration_file"`
}

type Source struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Folder   string `json:"folder"`
	DataURL  string `json:"data_url"`
}

type Variant struct {
	ID       int64  `json:"id"`
	FileStem string `json:"file_stem"`
	Suffix   string `json:"suffix"`
}

func iemResponse(r dbgen.ListIEMsRow) IEM {
	return IEM{
		IEMReference: IEMReference{
			ID:   iemID(r.BrandKey, r.IEMKey),
			Name: r.Name,
			Brand: brands.Reference{
				ID:   brands.ID(r.BrandKey),
				Name: r.BrandName,
			},
		},
		SourceCount:      int(r.SourceCount),
		VariantCount:     int(r.VariantCount),
		MeasurementCount: int(r.MeasurementCount),
	}
}

func sourceResponse(r dbgen.ListIEMSourcesRow) IEMSource {
	item := IEMSource{
		Source: Source{
			ID:       r.SourceID,
			Username: r.SourceUsername,
			Name:     r.SourceName,
			Type:     r.SourceType,
			Folder:   r.SourceFolder,
			DataURL:  r.SourceDataUrl,
		},
		RecordID:         r.RecordID,
		Brand:            r.Brand,
		Name:             r.Name,
		Price:            r.Price,
		ReviewScore:      r.ReviewScore,
		ReviewLink:       r.ReviewLink,
		ShopLink:         r.ShopLink,
		VariantCount:     int(r.VariantCount),
		MeasurementCount: int(r.MeasurementCount),
	}

	return item
}

func measurementResponse(r dbgen.ListMeasurementsRow) Measurement {
	return Measurement{
		ID: r.ID,
		IEM: IEMReference{
			ID:   iemID(r.BrandKey, r.IEMKey),
			Name: r.Name,
			Brand: brands.Reference{
				ID:   brands.ID(r.BrandKey),
				Name: r.BrandName,
			},
		},
		Source: Source{
			ID:       r.SourceID,
			Username: r.SourceUsername,
			Name:     r.SourceName,
			Type:     r.SourceType,
			Folder:   r.SourceFolder,
			DataURL:  r.SourceDataUrl,
		},
		Variant: Variant{
			ID:       r.VariantID,
			FileStem: r.VariantFileStem,
			Suffix:   r.VariantSuffix,
		},
		Channel:     r.Channel,
		Sample:      r.Sample,
		HTTPStatus:  int(r.HTTPStatus),
		PointCount:  int(r.PointCount),
		ParseIssues: json.RawMessage(r.ParseIssues),
		URL:         r.URL,
		FetchedAt:   r.FetchedAt,
	}
}

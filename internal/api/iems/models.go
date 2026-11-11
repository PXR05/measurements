package iems

import (
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
	SourceCount int `json:"source_count"`
}

type IEMDetail struct {
	IEM
	Sources []Source `json:"sources"`
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
	ID         int64  `json:"id"`
	Label      string `json:"label"`
	FileStem   string `json:"file_stem"`
	Channel    string `json:"channel"`
	Sample     string `json:"sample"`
	PointCount int    `json:"point_count"`
	URL        string `json:"url"`
	FetchedAt  string `json:"fetched_at"`
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
		SourceCount: int(r.SourceCount),
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
	return Measurement{
		ID:         r.ID,
		Label:      r.Label,
		FileStem:   r.FileStem,
		Channel:    r.Channel,
		Sample:     r.Sample,
		PointCount: int(r.PointCount),
		URL:        r.URL,
		FetchedAt:  r.FetchedAt,
	}
}

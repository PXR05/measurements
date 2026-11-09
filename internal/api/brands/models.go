package brands

import (
	"encoding/base64"

	"measurements/internal/storage/dbgen"
)

type Reference struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Brand struct {
	Reference
	IEMCount         int `json:"iem_count"`
	SourceCount      int `json:"source_count"`
	MeasurementCount int `json:"measurement_count"`
}

func brandResponse(r dbgen.ListBrandsRow) Brand {
	return Brand{
		Reference:        Reference{ID: ID(r.BrandKey), Name: r.Name},
		IEMCount:         int(r.IEMCount),
		SourceCount:      int(r.SourceCount),
		MeasurementCount: int(r.MeasurementCount),
	}
}

func ID(name string) string {
	return "brand_" + base64.RawURLEncoding.EncodeToString([]byte(name))
}

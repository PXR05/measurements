package brands

import (
	"encoding/base64"
	"strings"

	"measurements/internal/api/shared"
	"measurements/internal/storage/dbgen"
)

type Reference struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Brand struct {
	Reference
	IEMCount    int `json:"iem_count"`
	SourceCount int `json:"source_count"`
}

func brandResponse(r dbgen.ListBrandsRow) Brand {
	return Brand{
		Reference:   Reference{ID: ID(r.BrandKey), Name: r.Name},
		IEMCount:    int(r.IEMCount),
		SourceCount: int(r.SourceCount),
	}
}

func ID(name string) string {
	return "brand_" + base64.RawURLEncoding.EncodeToString([]byte(name))
}

func ParseID(id string) (string, error) {
	if !strings.HasPrefix(id, "brand_") || len(id) > 4096 {
		return "", shared.Invalid("invalid brand id")
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(id, "brand_"))
	if err != nil {
		return "", shared.Invalid("invalid brand id")
	}
	name := string(data)
	if name == "" || name != shared.NormalizeName(name) || ID(name) != id {
		return "", shared.Invalid("invalid brand id")
	}
	return name, nil
}

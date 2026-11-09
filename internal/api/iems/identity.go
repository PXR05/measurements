package iems

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"measurements/internal/api/shared"
)

func iemID(brand, name string) string {
	data, _ := json.Marshal([]string{brand, name})
	return "iem_" + base64.RawURLEncoding.EncodeToString(data)
}

func parseIEMID(id string) (brand, name string, err error) {
	if !strings.HasPrefix(id, "iem_") || len(id) > 4096 {
		return "", "", shared.Invalid("shared.Invalid IEM id")
	}

	data, decodeErr := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(id, "iem_"))

	var parts []string
	if decodeErr != nil || json.Unmarshal(data, &parts) != nil || len(parts) != 2 {
		return "", "", shared.Invalid("shared.Invalid IEM id")
	}

	if parts[0] != shared.NormalizeName(parts[0]) || parts[1] != shared.NormalizeName(parts[1]) || parts[1] == "" || iemID(parts[0], parts[1]) != id {
		return "", "", shared.Invalid("shared.Invalid IEM id")
	}

	return parts[0], parts[1], nil
}

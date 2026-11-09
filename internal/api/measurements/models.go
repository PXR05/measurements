package measurements

import (
	"encoding/json"

	"measurements/internal/storage/dbgen"
)

type Point struct {
	Index       int             `json:"index"`
	LineNumber  int             `json:"line_number"`
	FrequencyHz float64         `json:"frequency_hz"`
	SPLDB       float64         `json:"spl_db"`
	Phase       *float64        `json:"phase_degrees"`
	Values      json.RawMessage `json:"values"`
}

func pointResponse(r dbgen.MeasurementPoint) Point {
	point := Point{
		Index:       int(r.Ordinal),
		LineNumber:  int(r.LineNumber),
		FrequencyHz: r.FrequencyHz,
		SPLDB:       r.SPLDB,
		Values:      json.RawMessage(r.ValuesJSON),
	}
	if r.PhaseDegrees.Valid {
		point.Phase = &r.PhaseDegrees.Float64
	}
	return point
}

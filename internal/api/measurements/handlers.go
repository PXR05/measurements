package measurements

import (
	"bytes"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"measurements/internal/api/shared"
	"measurements/internal/storage/dbgen"
)

type Handler struct {
	DB      *sql.DB
	Respond *shared.Responder
}

func (a *Handler) Points(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if err := shared.ValidateKeys(values, "min_frequency", "max_frequency"); err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}

	id, err := shared.PositiveID(r.PathValue("id"), "id")
	if err == nil {
		var exists bool
		exists, err = dbgen.New(a.DB).MeasurementExists(r.Context(), id)
		if err == nil && !exists {
			err = sql.ErrNoRows
		}
	}

	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}

	options, err := shared.ParseListOptions(values, map[string]string{"index": "index", "frequency": "frequency"}, "index", 1000, 10000)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}

	var minimum, maximum sql.NullFloat64
	if values.Has("min_frequency") {
		minimum.Float64, err = shared.FiniteNumber(values, "min_frequency")
		minimum.Valid = true
		if err != nil {
			a.Respond.WriteError(w, r, err)
			return
		}
	}

	if values.Has("max_frequency") {
		maximum.Float64, err = shared.FiniteNumber(values, "max_frequency")
		maximum.Valid = true
		if err != nil {
			a.Respond.WriteError(w, r, err)
			return
		}
	}

	if minimum.Valid && maximum.Valid && minimum.Float64 > maximum.Float64 {
		a.Respond.WriteError(w, r, shared.Invalid("min_frequency must not exceed max_frequency"))
		return
	}

	params := dbgen.ListPointsParams{
		MeasurementID: id, MinFrequency: minimum, MaxFrequency: maximum,
		Sort: options.Sort, Direction: shared.LowerOrder(options.Order),
		PageLimit: int64(options.Limit), PageOffset: int64(options.Offset),
	}
	result, err := shared.ReadPage(r.Context(), a.DB, options, params, fetchPoints)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}

	a.Respond.WriteJSON(w, 200, result)
}

func (a *Handler) Raw(w http.ResponseWriter, r *http.Request) {
	if err := shared.ValidateOnlyKeys(r.URL.Query()); err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}

	id, err := shared.PositiveID(r.PathValue("id"), "id")
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}

	resource, err := dbgen.New(a.DB).GetMeasurementRaw(r.Context(), id)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("ETag", `"`+resource.SHA256+`"`)
	modified, _ := time.Parse(time.RFC3339Nano, resource.FetchedAt)
	http.ServeContent(w, r, fmt.Sprintf("measurement-%d.txt", id), modified, bytes.NewReader(resource.Body))
}

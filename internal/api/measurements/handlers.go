package measurements

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"measurements/internal/api/shared"
	"measurements/internal/storage"
	"measurements/internal/storage/dbgen"
)

type Handler struct {
	DB      *sql.DB
	Respond *shared.Responder
}

func (a *Handler) Points(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if err := shared.ValidateKeys(values, "min_frequency", "max_frequency", "normalized"); err != nil {
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

	normalized, err := shared.BoolValue(values, "normalized")
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

	if normalized {
		mc, err := dbgen.New(a.DB).GetMeasurementCurve(r.Context(), id)
		if err != nil {
			a.Respond.WriteError(w, r, err)
			return
		}
		vals, ok := storage.DecodeCurve(mc.Curve)
		if !ok {
			a.Respond.WriteError(w, r, sql.ErrNoRows)
			return
		}
		all := make([]Point, 0, storage.GridSize)
		for i, v := range vals {
			freq := storage.GridFrequency(i)
			if minimum.Valid && freq < minimum.Float64 {
				continue
			}
			if maximum.Valid && freq > maximum.Float64 {
				continue
			}
			spl := math.Round(float64(v)*100) / 100
			raw, _ := json.Marshal([]float64{freq, spl})
			all = append(all, Point{
				Index:       i,
				LineNumber:  i + 1,
				FrequencyHz: freq,
				SPLDB:       spl,
				Values:      raw,
			})
		}
		if options.Order == "DESC" {
			for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
				all[i], all[j] = all[j], all[i]
			}
		}
		total := len(all)
		start := min(options.Offset, total)
		end := min(start+options.Limit, total)
		page := shared.Page[Point]{
			Data: all[start:end],
			Pagination: shared.Pagination{
				Limit:   options.Limit,
				Offset:  options.Offset,
				Total:   int64(total),
				HasMore: end < total,
			},
		}
		a.Respond.WriteJSON(w, http.StatusOK, page)
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

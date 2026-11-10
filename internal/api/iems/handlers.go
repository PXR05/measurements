package iems

import (
	"database/sql"
	"net/http"

	"measurements/internal/api/shared"
	"measurements/internal/storage"
	"measurements/internal/storage/dbgen"
)

type Handler struct {
	DB      *sql.DB
	Respond *shared.Responder
}

var iemSorts = map[string]string{
	"name": "name", "brand": "brand", "sources": "sources", "measurements": "measurements",
}

var measurementSorts = map[string]string{
	"source": "source", "channel": "channel", "sample": "sample",
}

func (a *Handler) List(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if err := shared.ValidateKeys(values, "q", "brand", "has_measurements"); err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	options, err := shared.ParseListOptions(values, iemSorts, "name", 50, 200)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	search, err := shared.SearchText(values)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	brands, err := shared.StringSelection(values, "brand", true)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}

	if brands != "[]" {
		if err = a.DB.QueryRowContext(r.Context(), `SELECT json_group_array(coalesce(a.canonical_brand,j.value)) FROM json_each(?) j LEFT JOIN brand_identity_aliases a ON a.brand_key=j.value`, brands).Scan(&brands); err != nil {
			a.Respond.WriteError(w, r, err)
			return
		}
	}

	var available sql.NullInt64
	if values.Has("has_measurements") {
		value, err := shared.BoolValue(values, "has_measurements")
		if err != nil {
			a.Respond.WriteError(w, r, err)
			return
		}
		available = sql.NullInt64{
			Int64: shared.BoolInt(value),
			Valid: true,
		}
	}
	params := dbgen.ListIEMsParams{
		SearchText: search, Brands: brands, HasMeasurements: available,
		Sort: options.Sort, Direction: shared.LowerOrder(options.Order),
		PageLimit: int64(options.Limit), PageOffset: int64(options.Offset),
	}
	result, err := shared.ReadPage(r.Context(), a.DB, options, params, fetchIEMs)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	a.Respond.WriteJSON(w, http.StatusOK, result)
}

func (a *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	if err := shared.ValidateOnlyKeys(r.URL.Query()); err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	brand, name, err := parseIEMID(r.PathValue("id"))
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	defer tx.Rollback()
	brand, name, err = storage.ResolveIEM(r.Context(), tx, brand, name)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	queries := dbgen.New(tx)
	item, err := queries.GetIEM(r.Context(), dbgen.GetIEMParams{BrandKey: brand, IEMKey: name})
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	sources, err := queries.ListIEMSources(r.Context(), dbgen.ListIEMSourcesParams{BrandKey: brand, IEMKey: name})
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	detail := IEMDetail{IEM: iemResponse(dbgen.ListIEMsRow(item)), Sources: make([]IEMSource, 0, len(sources))}
	for _, source := range sources {
		detail.Sources = append(detail.Sources, sourceResponse(source))
	}
	if err = tx.Commit(); err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	a.Respond.WriteJSON(w, http.StatusOK, map[string]any{"data": detail})
}

func (a *Handler) Measurements(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if err := shared.ValidateKeys(values, "source", "type", "channel", "sample", "min_points"); err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	options, err := shared.ParseListOptions(values, measurementSorts, "source", 50, 200)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	brand, name, err := parseIEMID(r.PathValue("iemID"))
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	brand, name, err = storage.ResolveIEM(r.Context(), a.DB, brand, name)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	if _, err = dbgen.New(a.DB).GetIEM(r.Context(), dbgen.GetIEMParams{BrandKey: brand, IEMKey: name}); err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	sources, err := shared.StringSelection(values, "source", false)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	types, err := shared.StringSelection(values, "type", false)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	channels, err := shared.StringSelection(values, "channel", false)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	samples, err := shared.StringSelection(values, "sample", false)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	minimum, err := shared.IntegerValue(values, "min_points", 1)
	if err == nil && minimum < 1 {
		err = shared.Invalid("min_points must be positive")
	}
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	params := dbgen.ListMeasurementsParams{
		BrandKey:   brand,
		IEMKey:     name,
		Sources:    sources,
		Types:      types,
		Channels:   channels,
		Samples:    samples,
		MinPoints:  int64(minimum),
		Sort:       options.Sort,
		Direction:  shared.LowerOrder(options.Order),
		PageLimit:  int64(options.Limit),
		PageOffset: int64(options.Offset),
	}
	result, err := shared.ReadPage(r.Context(), a.DB, options, params, fetchMeasurements)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	a.Respond.WriteJSON(w, http.StatusOK, result)
}

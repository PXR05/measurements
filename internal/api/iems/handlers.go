package iems

import (
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"strings"

	"measurements/internal/api/brands"
	"measurements/internal/api/shared"
	"measurements/internal/storage"
	"measurements/internal/storage/dbgen"
)

type Handler struct {
	DB      *sql.DB
	Respond *shared.Responder
}

var iemSorts = map[string]string{
	"name": "name", "brand": "brand", "sources": "sources", "price": "price",
}

func (a *Handler) List(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if err := shared.ValidateKeys(values, "q", "brand"); err != nil {
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
	brandFilter, err := shared.StringSelection(values, "brand", true)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}

	if brandFilter != "[]" {
		if brandFilter, err = dbgen.New(a.DB).ResolveBrandsJSON(r.Context(), brandFilter); err != nil {
			a.Respond.WriteError(w, r, err)
			return
		}
	}

	params := dbgen.ListIEMsParams{
		SearchText: search, Brands: brandFilter,
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

func (a *Handler) ListByBrand(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if err := shared.ValidateKeys(values, "q"); err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	brand, err := brands.ParseID(r.PathValue("id"))
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	brand, err = storage.ResolveBrand(r.Context(), a.DB, brand)
	if err == nil {
		var exists bool
		exists, err = dbgen.New(a.DB).BrandExists(r.Context(), brand)
		if err == nil && !exists {
			err = sql.ErrNoRows
		}
	}
	if err != nil {
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
	brandFilter, _ := json.Marshal([]string{brand})
	params := dbgen.ListIEMsParams{
		SearchText: search, Brands: string(brandFilter),
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
	detail, err := fetchIEMDetail(r.Context(), dbgen.New(tx), brand, name)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	if err = tx.Commit(); err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	a.Respond.WriteJSON(w, http.StatusOK, map[string]any{"data": detail})
}

func (a *Handler) Similar(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if err := shared.ValidateOnlyKeys(values, "rig", "max_price", "cheaper", "limit", "offset"); err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	brand, name, err := parseIEMID(r.PathValue("id"))
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	brand, name, err = storage.ResolveIEM(r.Context(), a.DB, brand, name)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	target, err := dbgen.New(a.DB).GetIEM(r.Context(), dbgen.GetIEMParams{BrandKey: brand, IEMKey: name})
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	options, err := shared.ParseListOptions(values, map[string]string{"distance": "distance"}, "distance", 10, 25)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	var rig string
	if values.Has("rig") {
		if len(values["rig"]) != 1 {
			a.Respond.WriteError(w, r, shared.Invalid("rig must be provided once"))
			return
		}
		rig = strings.TrimSpace(values.Get("rig"))
		if rig != "711" && rig != "5128" {
			a.Respond.WriteError(w, r, shared.Invalid("rig must be 711 or 5128"))
			return
		}
	}
	var maxPrice sql.NullFloat64
	if values.Has("max_price") {
		v, err := shared.FiniteNumber(values, "max_price")
		if err != nil {
			a.Respond.WriteError(w, r, err)
			return
		}
		maxPrice = sql.NullFloat64{Float64: v, Valid: true}
	}
	cheaper, err := shared.BoolValue(values, "cheaper")
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	if cheaper {
		if !target.PriceUSD.Valid {
			maxPrice = sql.NullFloat64{Float64: -1, Valid: true}
		} else {
			limitPrice := math.Nextafter(round2(target.PriceUSD.Float64), 0)
			if !maxPrice.Valid || limitPrice < maxPrice.Float64 {
				maxPrice = sql.NullFloat64{Float64: limitPrice, Valid: true}
			}
		}
	}
	params := dbgen.ListIEMSimilaritiesParams{
		BrandKey:    brand,
		IEMKey:      name,
		RigType:     rig,
		MaxPriceUSD: maxPrice,
		PageLimit:   int64(options.Limit),
		PageOffset:  int64(options.Offset),
	}
	result, err := shared.ReadPage(r.Context(), a.DB, options, params, fetchSimilarIEMs)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	a.Respond.WriteJSON(w, http.StatusOK, result)
}

package iems

import (
	"database/sql"
	"encoding/json"
	"net/http"

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
	"name": "name", "brand": "brand", "sources": "sources",
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
		if err = a.DB.QueryRowContext(r.Context(), `SELECT json_group_array(coalesce(a.canonical_brand,j.value)) FROM json_each(?) j LEFT JOIN brand_identity_aliases a ON a.brand_key=j.value`, brandFilter).Scan(&brandFilter); err != nil {
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

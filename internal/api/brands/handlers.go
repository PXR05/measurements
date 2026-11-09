package brands

import (
	"database/sql"
	"net/http"

	"measurements/internal/api/shared"
	"measurements/internal/storage/dbgen"
)

type Handler struct {
	DB      *sql.DB
	Respond *shared.Responder
}

func (a *Handler) List(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if err := shared.ValidateOnlyKeys(values, "q", "limit", "offset", "order"); err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	options, err := shared.ParseListOptions(values, map[string]string{"name": "name"}, "name", 50, 200)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	search, err := shared.SearchText(values)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	params := dbgen.ListBrandsParams{
		SearchText: search, Direction: shared.LowerOrder(options.Order),
		PageLimit: int64(options.Limit), PageOffset: int64(options.Offset),
	}
	result, err := shared.ReadPage(r.Context(), a.DB, options, params, fetchBrands)
	if err != nil {
		a.Respond.WriteError(w, r, err)
		return
	}
	a.Respond.WriteJSON(w, http.StatusOK, result)
}

package api

import (
	"database/sql"
	"log"
	"net/http"
	"net/url"

	"measurements/internal/api/brands"
	"measurements/internal/api/iems"
	"measurements/internal/api/measurements"
	"measurements/internal/api/shared"
)

type API struct {
	db      *sql.DB
	respond *shared.Responder
	mux     *http.ServeMux
}

func New(db *sql.DB, logger *log.Logger) *API {
	if logger == nil {
		logger = log.Default()
	}
	a := &API{db: db, respond: &shared.Responder{Logger: logger}, mux: http.NewServeMux()}
	a.routes()
	return a
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if len(r.URL.RawQuery) > 8192 {
		a.respond.WriteError(w, r, shared.Invalid("query string must not exceed 8192 bytes"))
		return
	}

	if _, err := url.ParseQuery(r.URL.RawQuery); err != nil {
		a.respond.WriteError(w, r, shared.Invalid("malformed query string"))
		return
	}

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		a.respond.WriteError(w, r, &shared.RequestError{Status: 405, Message: "method not allowed"})
		return
	}

	a.mux.ServeHTTP(w, r)
}

func (a *API) routes() {
	iem := iems.Handler{DB: a.db, Respond: a.respond}
	brand := brands.Handler{DB: a.db, Respond: a.respond}
	measurement := measurements.Handler{DB: a.db, Respond: a.respond}
	a.mux.HandleFunc("GET /health", a.health)
	a.mux.HandleFunc("GET /api/v1/{$}", a.index)
	a.mux.HandleFunc("GET /api/v1/iems", iem.List)
	a.mux.HandleFunc("GET /api/v1/iems/{id}", iem.Detail)
	a.mux.HandleFunc("GET /api/v1/brands", brand.List)
	a.mux.HandleFunc("GET /api/v1/brands/{id}", iem.ListByBrand)
	a.mux.HandleFunc("GET /api/v1/measurements/{id}/points", measurement.Points)
	a.mux.HandleFunc("GET /api/v1/measurements/{id}/raw", measurement.Raw)
	a.mux.HandleFunc("/", a.notFound)
}

func (a *API) notFound(w http.ResponseWriter, r *http.Request) {
	a.respond.WriteError(w, r, sql.ErrNoRows)
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if err := shared.ValidateOnlyKeys(r.URL.Query()); err != nil {
		a.respond.WriteError(w, r, err)
		return
	}
	if err := a.db.PingContext(r.Context()); err != nil {
		a.respond.WriteError(w, r, err)
		return
	}

	a.respond.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) index(w http.ResponseWriter, r *http.Request) {
	if err := shared.ValidateOnlyKeys(r.URL.Query()); err != nil {
		a.respond.WriteError(w, r, err)
		return
	}
	a.respond.WriteJSON(w, http.StatusOK, map[string]any{
		"version": "v1",
		"routes": []string{
			"GET /api/v1/brands",
			"GET /api/v1/brands/{id}",
			"GET /api/v1/iems",
			"GET /api/v1/iems/{id}",
			"GET /api/v1/measurements/{id}/points",
			"GET /api/v1/measurements/{id}/raw",
		},
	})
}

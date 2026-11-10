package identity

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

//go:embed review.html
var page []byte

//go:embed review.css review.js
var assets embed.FS

type reviewer struct {
	store *Store
	mu    sync.Mutex
}

func NewHandler(s *Store) http.Handler {
	h := &reviewer{store: s}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(page)
	})
	mux.HandleFunc("GET /api/review", h.review)
	mux.Handle("GET /review.css", http.FileServer(http.FS(assets)))
	mux.Handle("GET /review.js", http.FileServer(http.FS(assets)))
	mux.HandleFunc("GET /api/export", h.export)
	mux.HandleFunc("POST /api/decision", h.decide)
	mux.HandleFunc("POST /api/undo", h.undo)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; base-uri 'none'")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			origin := r.Header.Get("Origin")
			if origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host != r.Host {
					http.Error(w, "origin not allowed", http.StatusForbidden)
					return
				}
			}
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" || r.Header.Get("X-IEM-Review") != "1" {
				http.Error(w, "review header required", http.StatusForbidden)
				return
			}
		}
		h.mu.Lock()
		defer h.mu.Unlock()
		mux.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode review response: %v", err)
	}
}
func fail(w http.ResponseWriter, err error) {
	log.Printf("review: %v", err)
	writeJSON(w, 500, map[string]string{"error": "Could not load catalog or save review. Check the server log."})
}

func (h *reviewer) review(w http.ResponseWriter, r *http.Request) {
	threshold := .8
	var err error
	if text := r.URL.Query().Get("threshold"); text != "" {
		threshold, err = strconv.ParseFloat(text, 64)
		if err != nil || !(threshold >= .55 && threshold <= 1) {
			http.Error(w, "threshold must be between 0.55 and 1", 400)
			return
		}
	}
	offset := 0
	if text := r.URL.Query().Get("offset"); text != "" {
		offset, err = strconv.Atoi(text)
		if err != nil || offset < 0 {
			http.Error(w, "invalid offset", 400)
			return
		}
	}
	ds, err := h.store.Decisions(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	records, err := h.store.catalog(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	scope := r.URL.Query().Get("scope")
	if scope != "" && scope != "iem" && scope != "brand" {
		http.Error(w, "invalid review scope", 400)
		return
	}
	group := products
	if scope == "brand" {
		group = brandProducts
	}
	items, roots, err := group(records, ds)
	if err != nil {
		fail(w, err)
		return
	}
	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	matches := func(p Product) bool {
		if search == "" {
			return true
		}
		if strings.Contains(strings.ToLower(p.Key.Brand+" "+p.Key.Name), search) {
			return true
		}
		for _, alias := range p.Aliases {
			if strings.Contains(strings.ToLower(alias), search) {
				return true
			}
		}
		for _, rec := range p.Records {
			if strings.Contains(strings.ToLower(rec.Brand+" "+rec.Name+" "+rec.Source), search) {
				return true
			}
		}
		return false
	}
	if r.URL.Query().Get("mode") == "products" {
		found := []Product{}
		for _, p := range items {
			if matches(p) {
				found = append(found, p)
			}
			if len(found) == 50 {
				break
			}
		}
		writeJSON(w, 200, found)
		return
	}
	pending := []Candidate{}
	for _, c := range candidates(items, ds, roots) {
		if c.Score >= threshold && (matches(c.Left) || matches(c.Right)) {
			pending = append(pending, c)
		}
	}
	total := len(pending)
	start := min(offset, total)
	end := start + min(30, total-start)
	history := ds[max(0, len(ds)-30):]
	writeJSON(w, 200, map[string]any{"candidates": pending[start:end], "total": total, "products": len(items), "records": len(records), "decisions": len(ds), "history": history})
}

func (h *reviewer) decide(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var d Decision
	if err := decoder.Decode(&d); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid decision JSON"})
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeJSON(w, 400, map[string]string{"error": "Expected one decision"})
		return
	}
	if err := h.store.Decide(r.Context(), d); err != nil {
		writeJSON(w, 409, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (h *reviewer) undo(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Undo(r.Context()); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (h *reviewer) export(w http.ResponseWriter, r *http.Request) {
	ds, err := h.store.Decisions(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "iem-identities.json"))
	writeJSON(w, 200, Backup{Version: 2, Decisions: ds})
}

// Package server exposes the AIC usage data over HTTP for the web UI.
package server

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/mazrean/gh-copilot-usage/internal/billing"
	"github.com/mazrean/gh-copilot-usage/internal/store"
)

//go:embed web
var webFS embed.FS

// New builds the HTTP handler. bc may be nil if gh is unauthenticated;
// in that case /api/monthly reports an error payload instead of failing startup.
func New(st *store.Store, bc *billing.Client) http.Handler {
	mux := http.NewServeMux()

	staticFS, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err) // embed misconfiguration, not a runtime condition
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	mux.HandleFunc("/api/usage", func(w http.ResponseWriter, r *http.Request) {
		dim := store.Dimension(r.URL.Query().Get("dimension"))
		if dim == "" {
			dim = store.DimModel
		}
		gran := store.Granularity(r.URL.Query().Get("granularity"))
		if gran == "" {
			gran = store.GranDay
		}

		usage, err := st.Aggregate(r.Context(), dim, gran)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, usage)
	})

	mux.HandleFunc("/api/monthly", func(w http.ResponseWriter, r *http.Request) {
		if bc == nil {
			writeError(w, http.StatusServiceUnavailable, errNoBillingClient)
			return
		}
		now := time.Now()
		year := queryInt(r, "year", now.Year())
		month := queryInt(r, "month", int(now.Month()))

		monthly, err := bc.Monthly(r.Context(), year, month)
		if err != nil {
			slog.Warn("monthly billing fetch failed", "err", err)
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, monthly)
	})

	return mux
}

var errNoBillingClient = errors.New("gh is not authenticated; monthly billing unavailable")

func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

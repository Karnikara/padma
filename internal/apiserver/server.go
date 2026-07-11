// Package apiserver assembles padma's HTTP API: it mounts the query handlers
// today and gains the webhook-subscription and merchant routes in later gates.
package apiserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/karnikara/padma/internal/merchant"
	"github.com/karnikara/padma/internal/platform/httpx"
	"github.com/karnikara/padma/internal/query"
	"github.com/karnikara/padma/internal/webhook"
)

// Router builds the top-level HTTP handler backed by pool. fiber is the Fiber
// node RPC client used by the merchant backend (order invoices, refunds).
func Router(pool *pgxpool.Pool, fiber merchant.FiberRPC) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	queryH := query.NewHandler(query.NewRepo(pool))
	webhookH := webhook.NewEndpointHandler(pool)
	merchantH := merchant.NewHandler(merchant.NewService(pool, fiber), pool)
	r.Route("/api", func(r chi.Router) {
		queryH.Routes(r)
		webhookH.Routes(r)
		merchantH.Routes(r)
	})
	return r
}

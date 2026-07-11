package webhook

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/karnikara/padma/internal/platform/auth"
	"github.com/karnikara/padma/internal/platform/httpx"
	"github.com/karnikara/padma/internal/platform/id"
)

// EndpointHandler serves webhook subscription management (#10).
type EndpointHandler struct {
	pool *pgxpool.Pool
}

// NewEndpointHandler returns a handler over pool.
func NewEndpointHandler(pool *pgxpool.Pool) *EndpointHandler {
	return &EndpointHandler{pool: pool}
}

// Routes mounts the subscription endpoints on a chi router.
func (h *EndpointHandler) Routes(r chi.Router) {
	r.Post("/webhooks", h.create)
	r.Get("/webhooks", h.list)
	r.Delete("/webhooks/{id}", h.remove)
	r.Post("/webhooks/{id}/test", h.test)
	r.Post("/webhooks/{id}/replay/{delivery_id}", h.replay)
}

type createReq struct {
	URL        string   `json:"url"`
	EventTypes []string `json:"event_types"`
}

type endpointResp struct {
	ID         string   `json:"id"`
	URL        string   `json:"url"`
	Secret     string   `json:"secret,omitempty"` // returned only on create
	EventTypes []string `json:"event_types"`
	Active     bool     `json:"active"`
}

func (h *EndpointHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if req.URL == "" || len(req.EventTypes) == 0 {
		httpx.Error(w, http.StatusBadRequest, "url and event_types are required")
		return
	}
	epID := id.DeriveWithPrefix("whk_", "endpoint", req.URL) // stable per URL; harmless if re-created
	secret := auth.NewWebhookSecret()
	_, err := h.pool.Exec(r.Context(), `
		INSERT INTO webhook_endpoints (id, url, secret, event_types, active)
		VALUES ($1, $2, $3, $4, true)
		ON CONFLICT (id) DO UPDATE SET url = EXCLUDED.url, event_types = EXCLUDED.event_types, active = true`,
		epID, req.URL, secret, req.EventTypes)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, endpointResp{
		ID: epID, URL: req.URL, Secret: secret, EventTypes: req.EventTypes, Active: true,
	})
}

func (h *EndpointHandler) list(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, url, event_types, active FROM webhook_endpoints ORDER BY created_at DESC`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	out := []endpointResp{}
	for rows.Next() {
		var e endpointResp
		if err := rows.Scan(&e.ID, &e.URL, &e.EventTypes, &e.Active); err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, e)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *EndpointHandler) remove(w http.ResponseWriter, r *http.Request) {
	// Soft-delete: deactivate so historical deliveries keep their FK.
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE webhook_endpoints SET active = false WHERE id = $1`, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, http.StatusNotFound, "endpoint not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *EndpointHandler) test(w http.ResponseWriter, r *http.Request) {
	epID := chi.URLParam(r, "id")
	var url, secret string
	err := h.pool.QueryRow(r.Context(),
		`SELECT url, secret FROM webhook_endpoints WHERE id = $1 AND active`, epID).Scan(&url, &secret)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusNotFound, "endpoint not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	// A test needs an event row to satisfy the delivery FK; insert a synthetic one.
	ctx := r.Context()
	evtID := id.New()
	ping, _ := json.Marshal(map[string]any{"type": "webhook.test", "endpoint_id": epID})
	if _, err := h.pool.Exec(ctx, `
		INSERT INTO events (id, type, fiber_ref, data, occurred_at, checkpoint)
		VALUES ($1, 'webhook.test', $2, $3::jsonb, now(), 0)`, evtID, epID, ping); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	dlvID := id.New()
	if _, err := h.pool.Exec(ctx, `
		INSERT INTO webhook_deliveries (id, endpoint_id, event_id, url, secret, payload, status, next_attempt_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, 'pending', now())`,
		dlvID, epID, evtID, url, secret, ping); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]string{"delivery_id": dlvID, "status": "queued"})
}

func (h *EndpointHandler) replay(w http.ResponseWriter, r *http.Request) {
	dlvID := chi.URLParam(r, "delivery_id")
	tag, err := h.pool.Exec(r.Context(), `
		UPDATE webhook_deliveries
		SET status = 'pending', attempts = 0, next_attempt_at = now(), last_error = NULL
		WHERE id = $1 AND endpoint_id = $2`, dlvID, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, http.StatusNotFound, "delivery not found for endpoint")
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]string{"delivery_id": dlvID, "status": "requeued"})
}

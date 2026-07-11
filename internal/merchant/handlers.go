package merchant

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/karnikara/padma/internal/platform/auth"
	"github.com/karnikara/padma/internal/platform/httpx"
)

type ctxKey int

const merchantIDKey ctxKey = iota

// Handler serves the merchant backend endpoints (#3), guarded by API-key auth.
type Handler struct {
	svc  *Service
	pool *pgxpool.Pool
}

// NewHandler returns a merchant Handler.
func NewHandler(svc *Service, pool *pgxpool.Pool) *Handler {
	return &Handler{svc: svc, pool: pool}
}

// Routes mounts the merchant endpoints (all requiring a merchant API key).
func (h *Handler) Routes(r chi.Router) {
	r.Route("/merchant", func(r chi.Router) {
		r.Use(h.authMerchant)
		r.Post("/orders", h.createOrder)
		r.Get("/orders/{id}", h.getOrder)
		r.Post("/orders/{id}/refund", h.refund)
		r.Get("/settlements", h.settlements)
		r.Get("/reconciliation", h.reconciliation)
		r.Get("/export", h.export)
	})
}

// authMerchant authenticates the Authorization: Bearer <api-key> header against
// merchants.api_key_hash and injects the merchant id into the request context.
func (h *Handler) authMerchant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		hdr := r.Header.Get("Authorization")
		if len(hdr) <= len(prefix) || hdr[:len(prefix)] != prefix {
			httpx.Error(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		hash := auth.HashAPIKey(hdr[len(prefix):])
		var merchantID string
		err := h.pool.QueryRow(r.Context(),
			`SELECT id FROM merchants WHERE api_key_hash = $1`, hash).Scan(&merchantID)
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusUnauthorized, "invalid api key")
			return
		}
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		ctx := context.WithValue(r.Context(), merchantIDKey, merchantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func merchantID(r *http.Request) string {
	v, _ := r.Context().Value(merchantIDKey).(string)
	return v
}

// writeErr maps domain errors to HTTP status codes.
func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrInvalidState):
		httpx.Error(w, http.StatusBadRequest, err.Error())
	default:
		httpx.Error(w, http.StatusInternalServerError, err.Error())
	}
}

type createOrderReq struct {
	ExternalOrderID string `json:"external_order_id"`
	Amount          string `json:"amount"`
	Asset           string `json:"asset"`
}

func (h *Handler) createOrder(w http.ResponseWriter, r *http.Request) {
	var req createOrderReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	order, inv, err := h.svc.CreateOrder(r.Context(), merchantID(r), req.ExternalOrderID, req.Amount, req.Asset)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"order": order,
		"fiber_invoice": map[string]string{
			"payment_hash":    string(inv.PaymentHash),
			"invoice_address": inv.Address,
		},
	})
}

func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request) {
	order, err := h.svc.GetOrder(r.Context(), merchantID(r), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, order)
}

type refundReq struct {
	Amount string `json:"amount"`
	Reason string `json:"reason"`
}

func (h *Handler) refund(w http.ResponseWriter, r *http.Request) {
	var req refundReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	refund, err := h.svc.Refund(r.Context(), merchantID(r), chi.URLParam(r, "id"), req.Amount, req.Reason)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, refund)
}

func (h *Handler) settlements(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		httpx.Error(w, http.StatusBadRequest, "period (YYYY-MM) is required")
		return
	}
	rows, err := h.svc.Settlement(r.Context(), merchantID(r), period)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"period": period, "settlements": rows})
}

func (h *Handler) reconciliation(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		httpx.Error(w, http.StatusBadRequest, "period (YYYY-MM) is required")
		return
	}
	rec, err := h.svc.Reconciliation(r.Context(), merchantID(r), period)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, rec)
}

func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		httpx.Error(w, http.StatusBadRequest, "period (YYYY-MM) is required")
		return
	}
	name, contentType, data, err := h.svc.Export(r.Context(), merchantID(r), period, r.URL.Query().Get("format"))
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

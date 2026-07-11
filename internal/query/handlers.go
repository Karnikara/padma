package query

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/karnikara/padma/internal/platform/httpx"
)

// Handler serves the read-model REST endpoints (#11).
type Handler struct {
	repo *Repo
}

// NewHandler returns a Handler backed by repo.
func NewHandler(repo *Repo) *Handler {
	return &Handler{repo: repo}
}

// Routes mounts the query endpoints on a chi router.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/payments", h.listPayments)
	r.Get("/payments/{payment_hash}", h.getPayment)
	r.Get("/channels", h.listChannels)
	r.Get("/channels/{channel_id}", h.getChannel)
	r.Get("/events", h.listEvents)
	r.Get("/stats/payments", h.statsPayments)
}

func (h *Handler) listPayments(w http.ResponseWriter, r *http.Request) {
	cur, err := DecodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	from, err := httpx.QueryTime(r, "from")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid from: "+err.Error())
		return
	}
	to, err := httpx.QueryTime(r, "to")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid to: "+err.Error())
		return
	}
	f := PaymentFilter{
		Direction: r.URL.Query().Get("direction"),
		Status:    r.URL.Query().Get("status"),
		Asset:     r.URL.Query().Get("asset"),
		Peer:      r.URL.Query().Get("peer"),
		From:      from,
		To:        to,
	}
	page, err := h.repo.Payments(r.Context(), f, cur, httpx.QueryInt(r, "limit", 50))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

func (h *Handler) getPayment(w http.ResponseWriter, r *http.Request) {
	p, err := h.repo.PaymentByHash(r.Context(), chi.URLParam(r, "payment_hash"))
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "payment not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

func (h *Handler) listChannels(w http.ResponseWriter, r *http.Request) {
	cur, err := DecodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	f := ChannelFilter{
		State: r.URL.Query().Get("state"),
		Peer:  r.URL.Query().Get("peer"),
	}
	page, err := h.repo.Channels(r.Context(), f, cur, httpx.QueryInt(r, "limit", 50))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

func (h *Handler) getChannel(w http.ResponseWriter, r *http.Request) {
	ch, err := h.repo.ChannelByID(r.Context(), chi.URLParam(r, "channel_id"))
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "channel not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, ch)
}

func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	cur, err := DecodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	from, err := httpx.QueryTime(r, "from")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid from: "+err.Error())
		return
	}
	to, err := httpx.QueryTime(r, "to")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid to: "+err.Error())
		return
	}
	f := EventFilter{Type: r.URL.Query().Get("type"), From: from, To: to}
	page, err := h.repo.Events(r.Context(), f, cur, httpx.QueryInt(r, "limit", 50))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

func (h *Handler) statsPayments(w http.ResponseWriter, r *http.Request) {
	from, err := httpx.QueryTime(r, "from")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid from: "+err.Error())
		return
	}
	to, err := httpx.QueryTime(r, "to")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid to: "+err.Error())
		return
	}
	stats, err := h.repo.StatsPayments(r.Context(), from, to)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"group_by": "asset", "buckets": stats})
}

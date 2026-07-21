// Package delivery is the tenant-scoped, read-only content delivery API
// consumed server-side by our Nuxt sites (Etude's Nitro + klanghaus.dk) — the
// Fase 2 surface that lets a tenant's site render its real, published content
// (Etude M5). It serves ONLY published documents.
//
// It is a trusted service-to-service path: the caller authenticates as a
// service (auth.ServiceMiddleware) and conveys the tenant via X-Scope-OrgID.
// The tenant is never taken from an end-user/browser request, and RLS enforces
// isolation in the database regardless.
package delivery

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/auth"
	"github.com/KlangHaus/resonans/internal/content"
)

// maxList caps a catalog response until cursor pagination lands (follow-up).
const maxList = 200

// Deps are the collaborators the delivery API needs.
type Deps struct {
	Store        content.Store
	ServiceToken string // "" in dev/test (X-Scope-OrgID trusted directly)
	Logger       *slog.Logger
}

// Handlers holds the wired dependencies.
type Handlers struct {
	store content.Store
	log   *slog.Logger
}

// NewRouter builds the delivery handler: published-only read routes wrapped in
// the service-auth middleware (which resolves the tenant onto the context).
func NewRouter(d Deps) http.Handler {
	h := &Handlers{store: d.Store, log: d.Logger}

	mux := http.NewServeMux()
	// Catalog: published documents of a type (e.g. /delivery/v1/course).
	mux.HandleFunc("GET /delivery/v1/{type}", h.listPublished)
	// A single published document by type + slug (e.g. a lesson/page).
	mux.HandleFunc("GET /delivery/v1/{type}/{slug}", h.getPublished)

	return auth.ServiceMiddleware(d.ServiceToken, d.Logger)(mux)
}

// tenant resolves the authoritative tenant from the service-auth'd actor.
func (h *Handlers) tenant(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	actor, ok := auth.ActorFromContext(r.Context())
	if !ok || !actor.HasTenant() {
		writeJSON(w, http.StatusForbidden, errBody("no tenant scope"))
		return uuid.Nil, false
	}
	tid, err := uuid.Parse(actor.Tenant)
	if err != nil {
		writeJSON(w, http.StatusForbidden, errBody("invalid tenant scope"))
		return uuid.Nil, false
	}
	return tid, true
}

func (h *Handlers) listPublished(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	docs, err := h.store.List(r.Context(), tid, content.ListFilter{
		Type:   r.PathValue("type"),
		Status: content.StatusPublished,
	})
	if err != nil {
		h.writeErr(w, err)
		return
	}
	truncated := false
	if len(docs) > maxList {
		docs = docs[:maxList]
		truncated = true
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": docs, "truncated": truncated})
}

func (h *Handlers) getPublished(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	d, err := h.store.GetPublished(r.Context(), tid, r.PathValue("type"), r.PathValue("slug"))
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *Handlers) writeErr(w http.ResponseWriter, err error) {
	if errors.Is(err, content.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, errBody("not found"))
		return
	}
	if h.log != nil {
		h.log.Error("delivery_store_error", "err", err)
	}
	writeJSON(w, http.StatusInternalServerError, errBody("internal error"))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func errBody(msg string) map[string]string { return map[string]string{"error": msg} }

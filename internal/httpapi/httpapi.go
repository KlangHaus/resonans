// Package httpapi adapts the content domain to a REST API (Beslutning W:
// content-API to our own Nuxt sites = REST). It owns request decoding, response
// encoding and status-code mapping — no business logic.
//
// Every route is tenant-scoped: the tenant is taken from the verified JWT actor
// (auth middleware), never from request input, so a consumer cannot access
// another tenant's content. The Postgres store additionally enforces this via
// RLS.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/auth"
	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/schema"
)

// Deps are the collaborators the API needs.
type Deps struct {
	Store    content.Store
	Registry *schema.Registry
	Verifier auth.Verifier // nil in dev/test (X-Dev-Tenant header path)
	Logger   *slog.Logger
}

// Handlers holds the wired dependencies for the route handlers.
type Handlers struct {
	store    content.Store
	registry *schema.Registry
	log      *slog.Logger
}

// NewRouter builds the API handler: content routes wrapped in the JWT-auth
// middleware (which resolves the tenant onto the request context).
func NewRouter(d Deps) http.Handler {
	h := &Handlers{store: d.Store, registry: d.Registry, log: d.Logger}

	mux := http.NewServeMux()
	// Schema (editor form/palette source) — authenticated, not tenant-scoped.
	mux.HandleFunc("GET /api/v1/schema", h.schemaAll)
	mux.HandleFunc("GET /api/v1/schema/{type}", h.schemaOne)
	// Documents — tenant-scoped authoring.
	mux.HandleFunc("GET /api/v1/documents", h.listDocuments)
	mux.HandleFunc("GET /api/v1/documents/{id}", h.getDocument)
	mux.HandleFunc("GET /api/v1/content/{type}/{slug}", h.getPublished)
	mux.HandleFunc("POST /api/v1/documents", h.createDocument)
	mux.HandleFunc("PUT /api/v1/documents/{id}", h.updateDocument)
	mux.HandleFunc("POST /api/v1/documents/{id}/publish", h.publishDocument)

	return auth.Middleware(d.Verifier, d.Logger)(mux)
}

// schemaAll serves the full registry (doc-types + block library) so the editor
// can render nav, forms and the block palette.
func (h *Handlers) schemaAll(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"docTypes": h.registry.DocTypes(),
		"blocks":   h.registry.Blocks(),
	})
}

func (h *Handlers) schemaOne(w http.ResponseWriter, r *http.Request) {
	dt, ok := h.registry.DocType(r.PathValue("type"))
	if !ok {
		writeJSON(w, http.StatusNotFound, errBody("unknown doc-type"))
		return
	}
	writeJSON(w, http.StatusOK, dt)
}

// validateBody checks a document body against its doc-type schema. Schema
// errors (unknown type / invalid body) map to 400.
func (h *Handlers) validateBody(w http.ResponseWriter, docType string, body []byte) bool {
	if err := schema.ValidateDocument(h.registry, docType, body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return false
	}
	return true
}

// tenant resolves the authoritative tenant from the verified actor. Missing or
// malformed tenant → 403 (the actor is set by the auth middleware; a request
// without a tenant claim may not touch tenant-scoped content).
func (h *Handlers) tenant(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	actor, ok := auth.ActorFromContext(r.Context())
	if !ok || !actor.HasTenant() {
		writeJSON(w, http.StatusForbidden, errBody("no tenant in context"))
		return uuid.Nil, false
	}
	tid, err := uuid.Parse(actor.Tenant)
	if err != nil {
		writeJSON(w, http.StatusForbidden, errBody("invalid tenant"))
		return uuid.Nil, false
	}
	return tid, true
}

func (h *Handlers) listDocuments(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	f := content.ListFilter{
		Type:   r.URL.Query().Get("type"),
		Status: r.URL.Query().Get("status"),
	}
	docs, err := h.store.List(r.Context(), tid, f)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": docs})
}

func (h *Handlers) getDocument(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid id"))
		return
	}
	d, err := h.store.Get(r.Context(), tid, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
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

func (h *Handlers) createDocument(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	var in content.CreateInput
	if !decodeJSON(w, r, &in) {
		return
	}
	body := in.Body
	if len(body) == 0 {
		body = []byte("{}")
	}
	if !h.validateBody(w, in.Type, body) {
		return
	}
	d, err := h.store.Create(r.Context(), tid, in)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (h *Handlers) updateDocument(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid id"))
		return
	}
	var in content.UpdateInput
	if !decodeJSON(w, r, &in) {
		return
	}
	// A replaced body must satisfy the document's doc-type schema.
	if in.Body != nil {
		existing, err := h.store.Get(r.Context(), tid, id)
		if err != nil {
			h.writeErr(w, err)
			return
		}
		if !h.validateBody(w, existing.Type, *in.Body) {
			return
		}
	}
	d, err := h.store.Update(r.Context(), tid, id, in)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *Handlers) publishDocument(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid id"))
		return
	}
	d, err := h.store.Publish(r.Context(), tid, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// writeErr maps domain errors to HTTP status codes.
func (h *Handlers) writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, content.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errBody("not found"))
	case errors.Is(err, content.ErrSlugConflict):
		writeJSON(w, http.StatusConflict, errBody("slug already exists"))
	case errors.Is(err, content.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, errBody("invalid input"))
	default:
		if h.log != nil {
			h.log.Error("content_store_error", "err", err)
		}
		writeJSON(w, http.StatusInternalServerError, errBody("internal error"))
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)) // 1 MiB cap
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid JSON body"))
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func errBody(msg string) map[string]string { return map[string]string{"error": msg} }

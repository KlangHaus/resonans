// Package httpapi adapts the content domain to a REST API (Beslutning W:
// content-API to our own Nuxt sites = REST). It owns request decoding, response
// encoding and status-code mapping — no business logic.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/editorauth"
)

// Deps are the collaborators the API needs.
type Deps struct {
	Store       content.Store
	DefaultSite string
	EditorToken string
	Logger      *slog.Logger
}

// Handlers holds the wired dependencies for the route handlers.
type Handlers struct {
	store       content.Store
	defaultSite string
	log         *slog.Logger
}

// NewRouter builds the API mux. Read routes are public; write (editor) routes
// are wrapped with the editor-auth middleware.
func NewRouter(d Deps) *http.ServeMux {
	h := &Handlers{store: d.Store, defaultSite: d.DefaultSite, log: d.Logger}
	guard := editorauth.Middleware(d.EditorToken)

	mux := http.NewServeMux()

	// Public read surface.
	mux.HandleFunc("GET /api/v1/entries", h.listEntries)
	mux.HandleFunc("GET /api/v1/entries/{id}", h.getEntry)
	mux.HandleFunc("GET /api/v1/content/{site}/{slug}", h.getPublished)

	// Editor write surface (guarded).
	mux.Handle("POST /api/v1/entries", guard(http.HandlerFunc(h.createEntry)))
	mux.Handle("PUT /api/v1/entries/{id}", guard(http.HandlerFunc(h.updateEntry)))
	mux.Handle("POST /api/v1/entries/{id}/publish", guard(http.HandlerFunc(h.publishEntry)))

	return mux
}

func (h *Handlers) listEntries(w http.ResponseWriter, r *http.Request) {
	site := r.URL.Query().Get("site")
	if site == "" {
		site = h.defaultSite
	}
	f := content.ListFilter{
		Site:   site,
		Type:   r.URL.Query().Get("type"),
		Status: r.URL.Query().Get("status"),
	}
	entries, err := h.store.List(r.Context(), f)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (h *Handlers) getEntry(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid id"))
		return
	}
	e, err := h.store.Get(r.Context(), id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (h *Handlers) getPublished(w http.ResponseWriter, r *http.Request) {
	e, err := h.store.GetPublished(r.Context(), r.PathValue("site"), r.PathValue("slug"))
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (h *Handlers) createEntry(w http.ResponseWriter, r *http.Request) {
	var in content.CreateInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Site == "" {
		in.Site = h.defaultSite
	}
	e, err := h.store.Create(r.Context(), in)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (h *Handlers) updateEntry(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid id"))
		return
	}
	var in content.UpdateInput
	if !decodeJSON(w, r, &in) {
		return
	}
	e, err := h.store.Update(r.Context(), id, in)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (h *Handlers) publishEntry(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid id"))
		return
	}
	e, err := h.store.Publish(r.Context(), id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// writeErr maps domain errors to HTTP status codes.
func (h *Handlers) writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, content.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errBody("not found"))
	case errors.Is(err, content.ErrSlugConflict):
		writeJSON(w, http.StatusConflict, errBody("slug already exists for site"))
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

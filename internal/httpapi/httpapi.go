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
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/auth"
	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/media"
	"github.com/KlangHaus/resonans/internal/preview"
	"github.com/KlangHaus/resonans/internal/schema"
	"github.com/KlangHaus/resonans/internal/settings"
)

// maxUploadBytes caps an asset upload (25 MiB).
const maxUploadBytes = 25 << 20

// Deps are the collaborators the API needs.
type Deps struct {
	Store         content.Store
	Registry      *schema.Registry
	Media         *media.Service
	Video         media.VideoProvider // nil when no video plugin is configured
	Verifier      auth.Verifier       // nil in dev/test (X-Dev-Tenant header path)
	Settings      settings.Store      // per-tenant content profile
	PreviewSecret string              // signs preview tokens; empty disables preview
	Logger        *slog.Logger
}

// Handlers holds the wired dependencies for the route handlers.
type Handlers struct {
	store         content.Store
	registry      *schema.Registry
	media         *media.Service
	video         media.VideoProvider
	settings      settings.Store
	previewSecret []byte
	log           *slog.Logger
}

// NewRouter builds the API handler: content routes wrapped in the JWT-auth
// middleware (which resolves the tenant onto the request context).
func NewRouter(d Deps) http.Handler {
	h := &Handlers{store: d.Store, registry: d.Registry, media: d.Media, video: d.Video, settings: d.Settings, previewSecret: []byte(d.PreviewSecret), log: d.Logger}

	mux := http.NewServeMux()
	// Schema (editor form/palette source) — filtered to the tenant's enabled
	// doc-types (content profile).
	mux.HandleFunc("GET /api/v1/schema", h.schemaAll)
	mux.HandleFunc("GET /api/v1/schema/{type}", h.schemaOne)
	// Per-tenant settings (content profile: enabled doc-types).
	mux.HandleFunc("GET /api/v1/settings", h.getSettings)
	mux.HandleFunc("PUT /api/v1/settings", h.putSettings)
	// Documents — tenant-scoped authoring.
	mux.HandleFunc("GET /api/v1/documents", h.listDocuments)
	mux.HandleFunc("GET /api/v1/documents/{id}", h.getDocument)
	mux.HandleFunc("GET /api/v1/content/{type}/{slug}", h.getPublished)
	mux.HandleFunc("POST /api/v1/documents", h.createDocument)
	mux.HandleFunc("PUT /api/v1/documents/{id}", h.updateDocument)
	mux.HandleFunc("POST /api/v1/documents/{id}/publish", h.publishDocument)
	// Media assets — tenant-scoped.
	mux.HandleFunc("POST /api/v1/assets", h.uploadAsset)
	mux.HandleFunc("GET /api/v1/assets", h.listAssets)
	mux.HandleFunc("GET /api/v1/assets/{id}", h.getAsset)
	// Video plugin (opt-in).
	mux.HandleFunc("GET /api/v1/video/embed", h.videoEmbed)
	mux.HandleFunc("GET /api/v1/video/resolve", h.videoResolve)
	// Version history + rollback (Fase 5 workflow).
	mux.HandleFunc("GET /api/v1/documents/{id}/versions", h.listVersions)
	mux.HandleFunc("POST /api/v1/documents/{id}/rollback/{version}", h.rollbackDocument)
	// Preview token issuance (Fase 5 preview).
	mux.HandleFunc("POST /api/v1/preview-token", h.previewToken)

	return auth.Middleware(d.Verifier, d.Logger)(mux)
}

// previewToken issues a short-lived signed token granting draft-read for one
// (type, slug) in the caller's tenant — used by the editor to render drafts in
// the frontend preview mode.
func (h *Handlers) previewToken(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	if len(h.previewSecret) == 0 {
		writeJSON(w, http.StatusNotFound, errBody("preview not enabled"))
		return
	}
	var in struct {
		Type string `json:"type"`
		Slug string `json:"slug"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Type == "" || in.Slug == "" {
		writeJSON(w, http.StatusBadRequest, errBody("type and slug required"))
		return
	}
	exp := time.Now().Add(15 * time.Minute)
	tok := preview.Sign(h.previewSecret, preview.Claims{
		Tenant: tid.String(), Type: in.Type, Slug: in.Slug, Exp: exp.Unix(),
	})
	writeJSON(w, http.StatusOK, map[string]any{"token": tok, "expiresInSeconds": 900})
}

func (h *Handlers) listVersions(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid id"))
		return
	}
	vs, err := h.store.ListVersions(r.Context(), tid, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": vs})
}

func (h *Handlers) rollbackDocument(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid id"))
		return
	}
	version, err := strconv.Atoi(r.PathValue("version"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid version"))
		return
	}
	d, err := h.store.Rollback(r.Context(), tid, id, version)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

type assetResponse struct {
	media.Asset
	URL string `json:"url"`
}

func (h *Handlers) assetWithURL(a media.Asset) assetResponse {
	return assetResponse{Asset: a, URL: h.media.URL(a, media.Transform{})}
}

func (h *Handlers) uploadAsset(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("expected multipart form field 'file'"))
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("could not read upload"))
		return
	}
	mime := header.Header.Get("Content-Type")
	if mime == "" {
		mime = "application/octet-stream"
	}
	a, err := h.media.Upload(r.Context(), tid, header.Filename, mime, data)
	if err != nil {
		if h.log != nil {
			h.log.Error("asset_upload_error", "err", err)
		}
		writeJSON(w, http.StatusInternalServerError, errBody("upload failed"))
		return
	}
	writeJSON(w, http.StatusCreated, h.assetWithURL(a))
}

func (h *Handlers) listAssets(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	assets, err := h.media.List(r.Context(), tid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("internal error"))
		return
	}
	out := make([]assetResponse, 0, len(assets))
	for _, a := range assets {
		out = append(out, h.assetWithURL(a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"assets": out})
}

func (h *Handlers) getAsset(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid id"))
		return
	}
	a, err := h.media.Get(r.Context(), tid, id)
	if err != nil {
		if errors.Is(err, media.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, errBody("not found"))
			return
		}
		writeJSON(w, http.StatusInternalServerError, errBody("internal error"))
		return
	}
	writeJSON(w, http.StatusOK, h.assetWithURL(a))
}

func (h *Handlers) videoEmbed(w http.ResponseWriter, r *http.Request) {
	if h.video == nil {
		writeJSON(w, http.StatusNotFound, errBody("video plugin not enabled"))
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, errBody("missing id"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"provider": h.video.Name(),
		"embedUrl": h.video.EmbedURL(id),
	})
}

// videoResolve returns the embed URL plus best-effort public metadata (title,
// thumbnail, duration) for the video-heavy authoring experience. If metadata
// resolution fails (private video / provider down) it still returns the embed
// URL so the editor degrades gracefully.
func (h *Handlers) videoResolve(w http.ResponseWriter, r *http.Request) {
	if h.video == nil {
		writeJSON(w, http.StatusNotFound, errBody("video plugin not enabled"))
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, errBody("missing id"))
		return
	}
	resp := map[string]any{"provider": h.video.Name(), "embedUrl": h.video.EmbedURL(id)}
	if meta, err := h.video.Metadata(r.Context(), id); err == nil {
		resp["meta"] = meta
	} else {
		resp["metaAvailable"] = false
		if h.log != nil {
			h.log.Warn("video_metadata_unavailable", "id", id, "err", err)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// schemaAll serves the full registry (doc-types + block library) so the editor
// can render nav, forms and the block palette.
func (h *Handlers) schemaAll(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	enabled, err := h.enabledTypes(r, tid)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	all := h.registry.DocTypes()
	docTypes := make([]schema.DocType, 0, len(all))
	for _, dt := range all {
		if settings.Enabled(enabled, dt.Type) {
			docTypes = append(docTypes, dt)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"docTypes": docTypes,
		"blocks":   h.registry.Blocks(),
	})
}

// enabledTypes reads the tenant's content profile (nil settings store = all).
func (h *Handlers) enabledTypes(r *http.Request, tid uuid.UUID) ([]string, error) {
	if h.settings == nil {
		return nil, nil
	}
	return h.settings.EnabledTypes(r.Context(), tid)
}

// getSettings returns the tenant's content profile plus the full type list so
// the editor can render enable/disable toggles (disabled types are absent from
// /schema, so the settings surface needs the registry's complete set).
func (h *Handlers) getSettings(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	enabled, err := h.enabledTypes(r, tid)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	if enabled == nil {
		enabled = []string{} // JSON contract: always an array, never null
	}
	all := h.registry.DocTypes()
	allTypes := make([]string, 0, len(all))
	for _, dt := range all {
		allTypes = append(allTypes, dt.Type)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabledTypes": enabled,  // empty = all enabled
		"allTypes":     allTypes, // full registry, for the toggle UI
	})
}

// putSettings replaces the tenant's enabled doc-type set. Empty list = enable
// everything. Unknown type names are rejected (registry is the source of truth).
func (h *Handlers) putSettings(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	if h.settings == nil {
		writeJSON(w, http.StatusNotImplemented, errBody("settings store not configured"))
		return
	}
	var in struct {
		EnabledTypes []string `json:"enabledTypes"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	for _, t := range in.EnabledTypes {
		if _, ok := h.registry.DocType(t); !ok {
			writeJSON(w, http.StatusBadRequest, errBody("unknown doc-type: "+t))
			return
		}
	}
	if err := h.settings.SetEnabledTypes(r.Context(), tid, in.EnabledTypes); err != nil {
		h.writeErr(w, err)
		return
	}
	h.getSettings(w, r)
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
	// Content profile: creating a disabled doc-type is rejected (existing docs
	// of a later-disabled type stay readable/editable — no data loss).
	enabled, err := h.enabledTypes(r, tid)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	if !settings.Enabled(enabled, in.Type) {
		writeJSON(w, http.StatusForbidden, errBody("doc-type disabled for this tenant: "+in.Type))
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

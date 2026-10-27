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
	"github.com/KlangHaus/resonans/internal/delivery"
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
	// Assets resolves asset ids in live-preview envelopes, exactly as delivery
	// does, so the frontend renders the draft with the same shape it gets in
	// production. nil = asset fields are left unresolved.
	Assets delivery.AssetResolver
	Logger *slog.Logger
}

// Handlers holds the wired dependencies for the route handlers.
type Handlers struct {
	store         content.Store
	registry      *schema.Registry
	media         *media.Service
	video         media.VideoProvider
	settings      settings.Store
	previewSecret []byte
	assets        delivery.AssetResolver
	log           *slog.Logger
}

// Route is one editor API route and the least role that may call it
// (ADR 0001 §3). The table is the single place a route is registered, so a
// route cannot exist without a role: there is no other way onto the mux.
type Route struct {
	Pattern string
	MinRole auth.Role
	handler http.HandlerFunc
}

// Routes returns the editor API's route table.
func (h *Handlers) Routes() []Route {
	const (
		viewer    = auth.RoleViewer
		editor    = auth.RoleEditor
		publisher = auth.RolePublisher
		admin     = auth.RoleAdmin
	)
	return []Route{
		// Schema (editor form/palette source) — filtered to the tenant's enabled
		// doc-types (content profile).
		{"GET /api/v1/schema", viewer, h.schemaAll},
		{"GET /api/v1/schema/{type}", viewer, h.schemaOne},
		// Per-tenant settings (content profile: enabled doc-types). Changing
		// them changes what every editor in the tenant can create: admin.
		{"GET /api/v1/settings", viewer, h.getSettings},
		{"PUT /api/v1/settings", admin, h.putSettings},
		// Visual editing: the tenant's frontend preview URL template. The URL
		// is framed inside the editor, so who sets it is an admin decision.
		{"GET /api/v1/settings/preview", viewer, h.getPreviewSettings},
		{"PUT /api/v1/settings/preview", admin, h.putPreviewSettings},
		// Documents — tenant-scoped authoring.
		{"GET /api/v1/documents", viewer, h.listDocuments},
		{"GET /api/v1/documents/{id}", viewer, h.getDocument},
		{"GET /api/v1/content/{type}/{slug}", viewer, h.getPublished},
		{"POST /api/v1/documents", editor, h.createDocument},
		{"PUT /api/v1/documents/{id}", editor, h.updateDocument},
		// Publishing puts content in front of the public: publisher.
		{"POST /api/v1/documents/{id}/publish", publisher, h.publishDocument},
		// Soft delete (Allan 2026-09-20): the bin, not an erasure. DELETE puts the
		// document away; restore takes it back. Both change what is published.
		{"DELETE /api/v1/documents/{id}", publisher, h.deleteDocument},
		{"POST /api/v1/documents/{id}/restore", publisher, h.restoreDocument},
		// Media assets — tenant-scoped.
		{"POST /api/v1/assets", editor, h.uploadAsset},
		{"GET /api/v1/assets", viewer, h.listAssets},
		{"GET /api/v1/assets/{id}", viewer, h.getAsset},
		{"DELETE /api/v1/assets/{id}", publisher, h.deleteAsset},
		{"POST /api/v1/assets/{id}/restore", publisher, h.restoreAsset},
		// Video plugin (opt-in).
		{"GET /api/v1/video/embed", viewer, h.videoEmbed},
		{"GET /api/v1/video/resolve", viewer, h.videoResolve},
		// Version history + rollback (Fase 5 workflow). A rollback can replace
		// published content: publisher.
		{"GET /api/v1/documents/{id}/versions", viewer, h.listVersions},
		{"POST /api/v1/documents/{id}/rollback/{version}", publisher, h.rollbackDocument},
		// Preview token issuance (Fase 5 preview) and live preview of an UNSAVED
		// body, pushed by the editor into the preview iframe on every edit.
		{"POST /api/v1/preview-token", editor, h.previewToken},
		{"POST /api/v1/preview/envelope", editor, h.previewEnvelope},
	}
}

// NewRouter builds the API handler: every route from the table, gated on its
// role, inside the JWT-auth middleware (which resolves the actor onto the
// request context).
func NewRouter(d Deps) http.Handler {
	h := &Handlers{store: d.Store, registry: d.Registry, media: d.Media, video: d.Video, settings: d.Settings, previewSecret: []byte(d.PreviewSecret), assets: d.Assets, log: d.Logger}

	mux := http.NewServeMux()
	for _, rt := range h.Routes() {
		mux.HandleFunc(rt.Pattern, auth.RequireRole(rt.MinRole, rt.handler))
	}
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

// previewEnvelope builds the delivery envelope for a body the editor has not
// saved yet. The body is capped by decodeJSON (1 MiB), as for a saved document. Nothing is persisted and nothing is read from the store: the body
// comes from the request and the tenant from the verified actor, so the only
// tenant data it can touch is asset resolution, which is tenant-scoped.
// The doc-type must exist AND be enabled for the tenant, like createDocument.
func (h *Handlers) previewEnvelope(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	var in struct {
		Type   string          `json:"type"`
		Slug   string          `json:"slug"`
		Title  string          `json:"title"`
		Locale string          `json:"locale"`
		Body   json.RawMessage `json:"body"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Type == "" {
		writeJSON(w, http.StatusBadRequest, errBody("type required"))
		return
	}
	if _, ok := h.registry.DocType(in.Type); !ok {
		writeJSON(w, http.StatusBadRequest, errBody("unknown doc-type: "+in.Type))
		return
	}
	enabled, err := h.enabledTypes(r, tid)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	if !settings.Enabled(enabled, in.Type) {
		writeJSON(w, http.StatusBadRequest, errBody("doc-type not enabled: "+in.Type))
		return
	}
	body := in.Body
	if len(body) == 0 {
		body = json.RawMessage("{}")
	}
	var probe map[string]any
	if err := json.Unmarshal(body, &probe); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("body must be a JSON object"))
		return
	}
	// 🔴 Sanitized exactly as a save would be. This envelope is rendered by the
	// tenant's frontend inside the editor (visual editing), and until 2026-10-23
	// it carried the body as typed: `<img src=x onerror=…>` came back unchanged,
	// so every frontend had to remember to clean it. Now the one server-side
	// policy that cleans stored content cleans this too.
	clean, ok := h.sanitizeBody(w, in.Type, body)
	if !ok {
		return
	}
	body = clean
	locale := in.Locale
	if locale == "" {
		locale = delivery.DefaultLocale
	}
	doc := content.Document{TenantID: tid, Type: in.Type, Slug: in.Slug, Title: in.Title, Body: body, Status: content.StatusDraft}
	env := delivery.BuildEnvelope(r.Context(), tid, doc, h.registry, locale, delivery.DefaultLocale, h.assets)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, env)
}

// getPreviewSettings returns the preview URL template and the origin derived
// from it (the Studio's postMessage allow-list of one).
func (h *Handlers) getPreviewSettings(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	if h.settings == nil {
		writeJSON(w, http.StatusNotImplemented, errBody("settings store not configured"))
		return
	}
	tpl, err := h.settings.PreviewURL(r.Context(), tid)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	origin := ""
	if tpl != "" {
		// A stored value that no longer validates (rule tightened later) is
		// reported without an origin, so the Studio refuses to talk to it.
		origin, _ = settings.ValidatePreviewURL(tpl)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"previewUrl":    tpl,
		"previewOrigin": origin,
		"previewReady":  origin != "" && len(h.previewSecret) > 0,
	})
}

// putPreviewSettings validates and stores the template ("" = turn off).
func (h *Handlers) putPreviewSettings(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	if h.settings == nil {
		writeJSON(w, http.StatusNotImplemented, errBody("settings store not configured"))
		return
	}
	var in struct {
		PreviewURL string `json:"previewUrl"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.PreviewURL != "" {
		if _, err := settings.ValidatePreviewURL(in.PreviewURL); err != nil {
			writeJSON(w, http.StatusBadRequest, errBody("previewUrl must be an absolute https URL (http only for localhost) with a fixed host and a {token} placeholder"))
			return
		}
	}
	if err := h.settings.SetPreviewURL(r.Context(), tid, in.PreviewURL); err != nil {
		h.writeErr(w, err)
		return
	}
	h.getPreviewSettings(w, r)
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
	// The client's Content-Type is passed along but not trusted: the media
	// service decides the type from the bytes (KH-1011).
	a, err := h.media.Upload(r.Context(), tid, header.Filename, header.Header.Get("Content-Type"), data)
	if errors.Is(err, media.ErrUnsupportedType) {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]any{
			"error":   "unsupported file type (judged from the file content, not its name)",
			"allowed": media.AllowedTypes(),
		})
		return
	}
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
	// Same rule as documents: anything other than the two known values — a typo
	// included — lists live assets only. The failure direction that matters is
	// never showing deleted content by accident.
	var f media.ListFilter
	switch r.URL.Query().Get("deleted") {
	case "only":
		f.Deleted = media.DeletedOnly
	case "include":
		f.Deleted = media.DeletedInclude
	}
	assets, err := h.media.List(r.Context(), tid, f)
	if err != nil {
		// Through writeMediaErr like every other media route: it answers the
		// same 500, and it is the one that LOGS the cause. Writing the 500
		// here directly left a failing asset library with no trace.
		h.writeMediaErr(w, err)
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
		h.writeMediaErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.assetWithURL(a))
}

// deleteAsset moves an asset to the bin. The bytes stay in storage, which is
// what makes the undo real — see media.Service.SoftDelete for the non-goals.
func (h *Handlers) deleteAsset(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid id"))
		return
	}
	a, err := h.media.SoftDelete(r.Context(), tid, id)
	if err != nil {
		h.writeMediaErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.assetWithURL(a))
}

// restoreAsset brings one back out of the bin.
func (h *Handlers) restoreAsset(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid id"))
		return
	}
	a, err := h.media.Restore(r.Context(), tid, id)
	if err != nil {
		h.writeMediaErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.assetWithURL(a))
}

// writeMediaErr maps asset errors to status codes. Deleted is 409, not 404: an
// editor who deleted a file and then cannot find it needs to know it is one
// click from coming back, and "never existed" is a different answer.
func (h *Handlers) writeMediaErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, media.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errBody("not found"))
	case errors.Is(err, media.ErrDeleted):
		writeJSON(w, http.StatusConflict, errBody("asset is deleted; restore it first"))
	case errors.Is(err, media.ErrNotDeleted):
		writeJSON(w, http.StatusConflict, errBody("asset is not deleted"))
	default:
		if h.log != nil {
			h.log.Error("media_store_error", "err", err)
		}
		writeJSON(w, http.StatusInternalServerError, errBody("internal error"))
	}
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
// sanitizeBody strips everything but formatting markup from rich-text fields
// before a body is stored (defense in depth with the Studio's own sanitizing).
func (h *Handlers) sanitizeBody(w http.ResponseWriter, docType string, body json.RawMessage) (json.RawMessage, bool) {
	clean, err := schema.SanitizeDocument(h.registry, docType, body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return nil, false
	}
	return clean, true
}

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
	// ?deleted=only lists the bin, ?deleted=include lists both. Anything else
	// — including a typo — lists only live documents, because the failure
	// direction that matters is never showing deleted content by accident.
	switch r.URL.Query().Get("deleted") {
	case "only":
		f.Deleted = content.DeletedOnly
	case "include":
		f.Deleted = content.DeletedInclude
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
	if body, ok = h.sanitizeBody(w, in.Type, body); !ok {
		return
	}
	in.Body = body
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
		clean, ok := h.sanitizeBody(w, existing.Type, *in.Body)
		if !ok {
			return
		}
		in.Body = &clean
	}
	d, err := h.store.Update(r.Context(), tid, id, in)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *Handlers) deleteDocument(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid id"))
		return
	}
	// The deleted document is returned, not an empty 204: the editor needs
	// `deletedAt` to show "in the bin since …" and to offer the undo without a
	// second round trip.
	d, err := h.store.SoftDelete(r.Context(), tid, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *Handlers) restoreDocument(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid id"))
		return
	}
	d, err := h.store.Restore(r.Context(), tid, id)
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
	// 🔴 409, NOT 404. "Deleted" and "never existed" are different worlds for
	// the person reading the message: one of them means "press undo".
	case errors.Is(err, content.ErrDeleted):
		writeJSON(w, http.StatusConflict, errBody("document is deleted; restore it first"))
	case errors.Is(err, content.ErrNotDeleted):
		writeJSON(w, http.StatusConflict, errBody("document is not deleted"))
	case errors.Is(err, content.ErrInvalidSlug):
		writeJSON(w, http.StatusBadRequest, errBody("invalid slug: lowercase a-z, 0-9 and single hyphens (1-200 chars)"))
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

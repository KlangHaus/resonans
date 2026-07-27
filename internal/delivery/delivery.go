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
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/auth"
	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/schema"
)

// maxList caps a catalog response until cursor pagination lands (follow-up).
const maxList = 200

// defaultLocale is used when a request omits ?locale and as the i18n fallback.
const defaultLocale = "da"

// Deps are the collaborators the delivery API needs.
type Deps struct {
	Store        content.Store
	Registry     *schema.Registry
	Assets       AssetResolver // may be nil (asset ids then pass through unresolved)
	ServiceToken string        // "" in dev/test (X-Scope-OrgID trusted directly)
	Logger       *slog.Logger
}

// Handlers holds the wired dependencies.
type Handlers struct {
	store    content.Store
	registry *schema.Registry
	assets   AssetResolver
	log      *slog.Logger
}

// NewRouter builds the delivery handler: published-only read routes wrapped in
// the service-auth middleware (which resolves the tenant onto the context).
func NewRouter(d Deps) http.Handler {
	h := &Handlers{store: d.Store, registry: d.Registry, assets: d.Assets, log: d.Logger}

	mux := http.NewServeMux()
	// Published block-schema + per-doc-type allowlists (consumer validation /
	// graceful-unknown / forward-compat). More specific than /{type}, so wins.
	mux.HandleFunc("GET /delivery/v1/schema", h.deliverySchema)
	// Catalog: published documents of a type (e.g. /delivery/v1/course).
	mux.HandleFunc("GET /delivery/v1/{type}", h.listPublished)
	// A single published document by type + slug (e.g. a lesson/page).
	mux.HandleFunc("GET /delivery/v1/{type}/{slug}", h.getPublished)

	return auth.ServiceMiddleware(d.ServiceToken, d.Logger)(mux)
}

// deliverySchema exposes the block library + per-doc-type allowlists so
// consumers can validate + gracefully handle unknown block types. Not
// tenant-scoped (the schema is global/code-defined).
func (h *Handlers) deliverySchema(w http.ResponseWriter, _ *http.Request) {
	docTypes := map[string]any{}
	for _, dt := range h.registry.DocTypes() {
		var allowed []string
		for _, f := range dt.Fields {
			if f.Type == schema.TypeBlocks {
				allowed = f.Allow
			}
		}
		entry := map[string]any{"allowedBlocks": allowed}
		if dt.Type == "quiz" {
			entry["serverSideOnly"] = true
			entry["note"] = "contains answer key (correctOptionIds, explanation) — redact server-side before the browser"
		}
		docTypes[dt.Type] = entry
	}
	blocks := map[string]any{}
	for _, b := range h.registry.Blocks() {
		blocks[b.Name] = map[string]any{"fields": b.Fields}
	}
	writeJSON(w, http.StatusOK, map[string]any{"docTypes": docTypes, "blocks": blocks})
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
	locale := h.locale(r)
	items := make([]map[string]any, 0, len(docs))
	for _, d := range docs {
		env := BuildEnvelope(r.Context(), tid, d, h.registry, locale, defaultLocale, h.assets)
		items = append(items, summarize(d, env))
	}
	// nextCursor omitted in v1 (single page up to maxList); `truncated` signals more.
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "truncated": truncated})
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
	// ETag from the revision counter — Etude caches per tenant/doc/locale/version.
	etag := fmt.Sprintf(`"%s.v%d"`, h.locale(r), d.Version)
	if r.Header.Get("If-None-Match") == etag {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}
	env := BuildEnvelope(r.Context(), tid, d, h.registry, h.locale(r), defaultLocale, h.assets)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	writeJSON(w, http.StatusOK, env)
}

// summarize builds the catalog summary (§D courseSummary shape) from a document
// + its envelope.
func summarize(d content.Document, env Envelope) map[string]any {
	s := map[string]any{
		"id": env.ID, "slug": env.Slug, "type": env.Type,
		"title": env.Data["title"], "updatedAt": d.UpdatedAt,
	}
	if v, ok := env.Data["summary"]; ok {
		s["summary"] = v
	} else if v, ok := env.Data["description"]; ok {
		s["summary"] = v
	}
	if cover, ok := env.Data["coverImage"].(map[string]any); ok {
		thumb := map[string]any{"url": cover["url"]}
		if alt, ok := cover["alt"]; ok {
			thumb["alt"] = alt
		}
		s["thumbnail"] = thumb
	}
	if mods, ok := env.Data["modules"].([]any); ok {
		s["moduleCount"] = len(mods)
	}
	return s
}

// locale returns the requested locale, defaulting when absent.
func (h *Handlers) locale(r *http.Request) string {
	if l := r.URL.Query().Get("locale"); l != "" {
		return l
	}
	return defaultLocale
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

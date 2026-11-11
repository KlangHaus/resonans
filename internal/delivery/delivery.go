// Package delivery is the tenant-scoped, read-only content delivery API that a
// tenant's site calls server-side to render its published content. It serves
// ONLY published documents.
//
// It is a trusted service-to-service path: the caller authenticates as a
// service (auth.ServiceMiddleware) and conveys the tenant via X-Scope-OrgID.
// The tenant is never taken from an end-user/browser request, and RLS enforces
// isolation in the database regardless.
package delivery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/KlangHaus/resonans/internal/auth"
	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/preview"
	"github.com/KlangHaus/resonans/internal/schema"
)

// maxList is the largest catalog page, and the page a request gets when it
// names no ?limit: the one page every catalog response was before pagination,
// so a consumer that never sends a limit sees no change.
const maxList = 200

// DefaultLocale is the locale used when a request omits ?locale and as the i18n
// fallback, unless RESONANS_DEFAULT_LOCALE sets another. Exported for the
// configuration default.
const DefaultLocale = "da"

// Deps are the collaborators the delivery API needs.
type Deps struct {
	Store        content.Store
	Registry     *schema.Registry
	Assets       AssetResolver // may be nil (asset ids then pass through unresolved)
	ServiceToken string        // "" in dev/test, together with TrustHeaderWithoutToken
	// TrustHeaderWithoutToken lets X-Scope-OrgID through with no service token.
	// Dev/test only: app.Run sets it from config.DeliveryDevAuth(). Without it an
	// empty ServiceToken refuses every request.
	TrustHeaderWithoutToken bool
	PreviewSecret           string // verifies preview tokens; empty disables preview
	DefaultLocale           string // "" means DefaultLocale
	Logger                  *slog.Logger
}

// Handlers holds the wired dependencies.
type Handlers struct {
	store         content.Store
	registry      *schema.Registry
	assets        AssetResolver
	previewSecret []byte
	defaultLocale string
	log           *slog.Logger
}

// NewRouter builds the delivery handler: published-only read routes wrapped in
// the service-auth middleware (which resolves the tenant onto the context).
func NewRouter(d Deps) http.Handler {
	h := &Handlers{store: d.Store, registry: d.Registry, assets: d.Assets, previewSecret: []byte(d.PreviewSecret), log: d.Logger}
	h.defaultLocale = d.DefaultLocale
	if h.defaultLocale == "" {
		h.defaultLocale = DefaultLocale
	}

	mux := http.NewServeMux()
	// Published block-schema + per-doc-type allowlists (consumer validation /
	// graceful-unknown / forward-compat). More specific than /{type}, so wins.
	mux.HandleFunc("GET /delivery/v1/schema", h.deliverySchema)
	// Draft preview by preview-token (editor preview-mode). More specific than
	// /{type}/{slug}, so wins.
	mux.HandleFunc("GET /delivery/v1/preview/{type}/{slug}", h.previewDraft)
	// Catalog: published documents of a type (e.g. /delivery/v1/course).
	mux.HandleFunc("GET /delivery/v1/{type}", h.listPublished)
	// A single published document by type + slug (e.g. a lesson/page).
	mux.HandleFunc("GET /delivery/v1/{type}/{slug}", h.getPublished)
	// The same document by type + id, for a consumer that holds a reference
	// ({type, id}) and not a slug. by-id comes first, not after {type}: Go's mux
	// refuses /{type}/by-id/{id} next to /preview/{type}/{slug}, since both
	// match /preview/by-id/x and neither is more specific.
	mux.HandleFunc("GET /delivery/v1/by-id/{type}/{id}", h.getPublishedByID)

	return auth.ServiceMiddleware(d.ServiceToken, d.TrustHeaderWithoutToken, d.Logger)(mux)
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
	limit, after, msg := pageParams(r)
	if msg != "" {
		writeJSON(w, http.StatusBadRequest, errBody(msg))
		return
	}
	// Editorial order: most recently published first, so a re-published draft
	// rises to the top (publishedAt = what the reader sees). The store sorts and
	// pages, so a page is a slice of that order and not of the stored one. One
	// document more than the page tells whether another page follows.
	docs, err := h.store.List(r.Context(), tid, content.ListFilter{
		Type:   r.PathValue("type"),
		Status: content.StatusPublished,
		Order:  content.OrderPublished,
		After:  after,
		Limit:  limit + 1,
	})
	if err != nil {
		h.writeErr(w, err)
		return
	}
	more := len(docs) > limit
	if more {
		docs = docs[:limit]
	}
	locale := h.locale(r)
	resp := map[string]any{}
	if more {
		resp["nextCursor"] = encodeCursor(docs[len(docs)-1])
	}
	// `truncated` predates nextCursor and says the same thing: another page
	// follows. It stays for consumers that read it.
	resp["truncated"] = more
	items := make([]map[string]any, 0, len(docs))
	for _, d := range docs {
		env := BuildEnvelope(r.Context(), tid, d, h.registry, locale, h.defaultLocale, h.assets)
		items = append(items, summarize(d, env))
	}
	resp["items"] = items
	q := r.URL.Query()
	h.writeRevalidated(w, r, resp, locale, h.defaultLocale, q.Get("limit"), q.Get("cursor"))
}

// writeRevalidated answers with v, or 304 when the request already holds its
// ETag. The ETag is a hash of the exact bytes sent and of the request values
// they were built for. A hash of the stored rows could not see what the body
// takes from elsewhere: an asset binned or restored changes the body while the
// document row stays the same, and a cache would be told 304 for the old body.
// The price is that a 304 builds the whole body before it answers.
func (h *Handlers) writeRevalidated(w http.ResponseWriter, r *http.Request, v any, request ...string) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		h.writeErr(w, err)
		return
	}
	etag := bodyETag(buf.Bytes(), request...)
	setRevalidate(w, etag)
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// bodyETag is a weak ETag over body and the request values it was built for.
// The request values keep two representations apart even when their bytes are
// equal, for example two locales of a document with no translated field.
func bodyETag(body []byte, request ...string) string {
	key := make([]byte, 0, len(body)+64)
	for _, v := range request {
		key = append(key, v...)
		key = append(key, 0)
	}
	sum := sha256.Sum256(append(key, body...))
	return `W/"` + base64.RawURLEncoding.EncodeToString(sum[:18]) + `"`
}

// setRevalidate marks a delivery response cacheable only with revalidation: a
// cache keeps it, and asks again with etag every time.
func setRevalidate(w http.ResponseWriter, etag string) {
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
}

// etagMatches is If-None-Match's weak comparison (RFC 9110 §13.1.2): any
// listed tag, or *, matches when it equals etag with any W/ prefix ignored.
func etagMatches(header, etag string) bool {
	want := strings.TrimPrefix(etag, "W/")
	for _, tag := range strings.Split(header, ",") {
		tag = strings.TrimSpace(tag)
		if tag == "*" || strings.TrimPrefix(tag, "W/") == want {
			return true
		}
	}
	return false
}

// pageParams reads ?limit and ?cursor. A bad value is a message for a 400, not
// a silent default: a consumer whose cursor is ignored would read page one again
// and again.
func pageParams(r *http.Request) (limit int, after *content.ListCursor, msg string) {
	q := r.URL.Query()
	limit = maxList
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxList {
			return 0, nil, fmt.Sprintf("limit must be a whole number from 1 to %d", maxList)
		}
		limit = n
	}
	if v := q.Get("cursor"); v != "" {
		c, ok := decodeCursor(v)
		if !ok {
			return 0, nil, "cursor is not one this API returned"
		}
		after = &c
	}
	return limit, after, ""
}

// encodeCursor is the opaque position after d in the catalog order: its publish
// time and id. Consumers pass it back unchanged; its form is not part of the API.
func encodeCursor(d content.Document) string {
	at := ""
	if k := d.SortKey(content.OrderPublished); !k.IsZero() {
		at = k.UTC().Format(time.RFC3339Nano)
	}
	return base64.RawURLEncoding.EncodeToString([]byte(at + "|" + d.ID.String()))
}

func decodeCursor(s string) (content.ListCursor, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return content.ListCursor{}, false
	}
	at, id, found := strings.Cut(string(raw), "|")
	if !found {
		return content.ListCursor{}, false
	}
	c := content.ListCursor{}
	if c.ID, err = uuid.Parse(id); err != nil {
		return content.ListCursor{}, false
	}
	if at != "" {
		if c.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return content.ListCursor{}, false
		}
	}
	return c, true
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
	h.writeDocument(w, r, tid, d)
}

func (h *Handlers) getPublishedByID(w http.ResponseWriter, r *http.Request) {
	tid, ok := h.tenant(w, r)
	if !ok {
		return
	}
	d, err := h.publishedByID(r.Context(), tid, r.PathValue("id"))
	if err == nil && d.Type != r.PathValue("type") {
		// The id names a document of another type: not this route's document.
		err = content.ErrNotFound
	}
	if err != nil {
		h.writeErr(w, err)
		return
	}
	h.writeDocument(w, r, tid, d)
}

// writeDocument answers with one published document, or 304 when the request
// already holds its ETag. Both single-document routes end here, so a document
// has one ETag and one body whichever key fetched it.
func (h *Handlers) writeDocument(w http.ResponseWriter, r *http.Request, tid uuid.UUID, d content.Document) {
	locale := h.locale(r)
	env := BuildEnvelope(r.Context(), tid, d, h.registry, locale, h.defaultLocale, h.assets)
	// ?expand=modules.lessons: replace a course's module refs with a nested tree
	// (module → lesson metadata) resolved in one call (no N+1 in SSR).
	if d.Type == "course" && strings.Contains(r.URL.Query().Get("expand"), "modules") {
		h.expandCourseTree(r.Context(), tid, env, locale)
	}
	h.writeRevalidated(w, r, env, locale, h.defaultLocale)
}

// expandCourseTree replaces course.data.modules (an array of module refs) with
// resolved nodes {id,slug,title,order,lessons:[{id,slug,title,order,durationEstimate}]}.
// Unpublished / missing refs are skipped. Mutates env.Data in place.
func (h *Handlers) expandCourseTree(ctx context.Context, tid uuid.UUID, env Envelope, locale string) {
	rawModules, ok := env.Data["modules"].([]any)
	if !ok {
		return
	}
	modules := make([]any, 0, len(rawModules))
	for i, el := range rawModules {
		modDoc, err := h.publishedByID(ctx, tid, extractRefID(el))
		if err != nil {
			continue
		}
		modEnv := BuildEnvelope(ctx, tid, modDoc, h.registry, locale, h.defaultLocale, h.assets)
		node := map[string]any{"id": modEnv.ID, "slug": modEnv.Slug, "title": modEnv.Data["title"], "order": i}

		rawLessons, _ := modEnv.Data["lessons"].([]any)
		lessons := make([]any, 0, len(rawLessons))
		for j, lel := range rawLessons {
			lesDoc, err := h.publishedByID(ctx, tid, extractRefID(lel))
			if err != nil {
				continue
			}
			lesEnv := BuildEnvelope(ctx, tid, lesDoc, h.registry, locale, h.defaultLocale, h.assets)
			lesson := map[string]any{"id": lesEnv.ID, "slug": lesEnv.Slug, "title": lesEnv.Data["title"], "order": j}
			if dur, ok := lesEnv.Data["estimatedMinutes"]; ok {
				lesson["durationEstimate"] = dur
			}
			lessons = append(lessons, lesson)
		}
		node["lessons"] = lessons
		modules = append(modules, node)
	}
	env.Data["modules"] = modules
}

// publishedByID fetches a published document by id (tenant-scoped). Non-published,
// binned or malformed ids yield ErrNotFound so the tree simply omits them.
//
// It holds the same conditions as GetPublished, because store.Get does not: Get
// also returns a document in the bin, and binning keeps status "published".
func (h *Handlers) publishedByID(ctx context.Context, tid uuid.UUID, idStr string) (content.Document, error) {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return content.Document{}, content.ErrNotFound
	}
	d, err := h.store.Get(ctx, tid, id)
	if err != nil {
		return content.Document{}, err
	}
	if d.Status != content.StatusPublished || d.IsDeleted() || d.Dataset != content.DefaultDataset {
		return content.Document{}, content.ErrNotFound
	}
	return d, nil
}

// extractRefID pulls the referenced doc id out of a resolved array element such
// as {"module": {"ref": {"type","id"}}} (the wrapper key varies: module/lesson).
func extractRefID(el any) string {
	m, ok := el.(map[string]any)
	if !ok {
		return ""
	}
	for _, v := range m {
		ref, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if r, ok := ref["ref"].(map[string]any); ok {
			if id, ok := r["id"].(string); ok {
				return id
			}
		}
	}
	return ""
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
	} else if v, ok := env.Data["excerpt"]; ok {
		// Blog posts carry their teaser in `excerpt` — surface it as the catalog
		// summary so a blog index needs no per-post fetch (post is a real consumer).
		s["summary"] = v
	}
	// Common metadata fields a listing surface (blog index) needs — only added
	// when the doc-type has them (courses/pages won't, so they stay absent).
	for _, k := range []string{"author", "category"} {
		if v, ok := env.Data[k]; ok {
			s[k] = v
		}
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

// previewDraft serves a DRAFT document, authorized by a preview token (which
// carries the tenant + doc scope). Unlike the published paths, the tenant comes
// from the verified token, not X-Scope-OrgID.
func (h *Handlers) previewDraft(w http.ResponseWriter, r *http.Request) {
	if len(h.previewSecret) == 0 {
		writeJSON(w, http.StatusNotFound, errBody("preview not enabled"))
		return
	}
	claims, err := preview.Verify(h.previewSecret, r.URL.Query().Get("token"), time.Now())
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, errBody("invalid or expired preview token"))
		return
	}
	typ, slug := r.PathValue("type"), r.PathValue("slug")
	if claims.Type != typ || claims.Slug != slug {
		writeJSON(w, http.StatusForbidden, errBody("token scope mismatch"))
		return
	}
	tid, err := uuid.Parse(claims.Tenant)
	if err != nil {
		writeJSON(w, http.StatusForbidden, errBody("invalid tenant in token"))
		return
	}
	d, err := h.store.GetBySlug(r.Context(), tid, typ, slug)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	env := BuildEnvelope(r.Context(), tid, d, h.registry, h.locale(r), h.defaultLocale, h.assets)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, env)
}

// locale returns the requested locale, defaulting when absent.
func (h *Handlers) locale(r *http.Request) string {
	if l := r.URL.Query().Get("locale"); l != "" {
		return l
	}
	return h.defaultLocale
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

// Package app wires the Resonans CMS: config -> otel -> jwt -> db/migrate ->
// content store -> REST router -> graceful shutdown. It mirrors the boot
// structure of the permissions/notifications Go services.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/KlangHaus/resonans/internal/auth"
	"github.com/KlangHaus/resonans/internal/backfill"
	"github.com/KlangHaus/resonans/internal/bridge"
	"github.com/KlangHaus/resonans/internal/config"
	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/delivery"
	"github.com/KlangHaus/resonans/internal/httpapi"
	"github.com/KlangHaus/resonans/internal/media"
	"github.com/KlangHaus/resonans/internal/observability"
	"github.com/KlangHaus/resonans/internal/persistence"
	"github.com/KlangHaus/resonans/internal/schema"
	"github.com/KlangHaus/resonans/internal/seed"
	"github.com/KlangHaus/resonans/internal/settings"
)

// version and revision are stamped via -ldflags at build time. revision is the
// full git sha the image was built from. /healthz exposes both so the RUNNING
// process can be asked what it is, without trusting compose, the deploy agent
// or a mutable tag — "does prod run the reviewed code?" must be answerable from
// outside the deploy system (Beslutning AJ).
var (
	version  = "0.0.0"
	revision = "unknown"
)

const shutdownTimeout = 10 * time.Second

// Run boots the service and blocks until ctx is canceled (SIGINT/SIGTERM) or
// the HTTP server fails fatally.
func Run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	stdout := newStdoutLogger(cfg.LogLevel, cfg.OTelServiceName, cfg.NodeEnv)

	providers, err := observability.Setup(ctx, observability.Options{
		ServiceName:      cfg.OTelServiceName,
		ServiceNamespace: cfg.OTelServiceNamespace,
		Version:          version,
		Environment:      cfg.NodeEnv,
		Forced:           cfg.OTelEnabled,
	}, stdout)
	if err != nil {
		return fmt.Errorf("observability setup: %w", err)
	}
	logger := providers.Logger
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := providers.Shutdown(shutCtx); err != nil {
			logger.Warn("observability_shutdown_error", "err", err)
		}
	}()
	logger.Info("observability_initialized", "otel", cfg.OTelEnabled || observability.Enabled())

	// --- JWT verifier (Beslutning D + K) ---
	// Dev auth is a positive opt-in (RESONANS_DEV_AUTH=1), never the absence of
	// NODE_ENV=production: the check runs before anything is served.
	editorDevAuth, err := cfg.EditorDevAuth()
	if err != nil {
		return err
	}
	deliveryDevAuth, err := cfg.DeliveryDevAuth()
	if err != nil {
		return err
	}
	var verifier auth.Verifier
	if editorDevAuth {
		// Named event, so an environment can be MEASURED for dev auth in its logs.
		logger.Warn("dev_auth_active", "surface", "editor", "header", "X-Dev-Tenant",
			"detail", "RESONANS_DEV_AUTH=1 and no JWT config — JWT auth DISABLED (dev/test only)")
	} else {
		if cfg.DevAuth {
			logger.Warn("RESONANS_DEV_AUTH=1 ignored for the editor API: JWT is configured, X-Dev-Tenant is not read")
		}
		verifier, err = auth.NewVerifier(ctx, auth.VerifierConfig{
			Issuer:  cfg.JWTIssuer,
			JWKSURL: cfg.JWKSURL,
		})
		if err != nil {
			return fmt.Errorf("jwt verifier: %w", err)
		}
		logger.Info("jwt_verifier_active", "issuer", cfg.JWTIssuer)
	}

	// --- Content store (Beslutning K — own content-DB, multi-tenant + RLS) ---
	var (
		store content.Store
		pool  *pgxpool.Pool
	)
	switch {
	case cfg.DatabaseURL != "":
		pool, err = persistence.NewPool(ctx, cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("connect db: %w", err)
		}
		defer pool.Close()

		// In production the server connects as the NOBYPASSRLS app-role, which
		// has no DDL privileges — migrations run via the `resonans migrate`
		// one-shot as the BYPASSRLS migrator-role (CTO deploy-wiring, Option A).
		// In dev the single role owns the schema, so auto-migrate for convenience.
		if cfg.IsProduction() {
			logger.Info("boot migrations skipped in production — run `resonans migrate` (migrator role) before deploy")
		} else {
			applied, err := persistence.Migrate(ctx, pool)
			if err != nil {
				return fmt.Errorf("migrate: %w", err)
			}
			if len(applied) > 0 {
				logger.Info("migrations_applied", "migrations", applied)
			}
		}
		store = content.NewPostgresStore(pool)
	case cfg.IsProduction():
		// The in-memory store loses all content on restart AND does not enforce
		// RLS — refuse to boot rather than degrade tenant isolation silently.
		return errors.New(
			"DATABASE_URL not set in production — refusing to start with a non-persistent in-memory content store",
		)
	default:
		logger.Warn("DATABASE_URL not set — using in-memory content store (dev/test only)")
		store = content.NewMemoryStore()
	}

	// --- Delivery service-auth (Beslutning: trusted service-to-service) ---
	// Decided above with the editor gate (DeliveryDevAuth), before any I/O.
	if deliveryDevAuth {
		logger.Warn("dev_auth_active", "surface", "delivery", "header", "X-Scope-OrgID",
			"detail", "RESONANS_DEV_AUTH=1 and no RESONANS_DELIVERY_TOKEN — /delivery trusts the header (dev/test only)")
	}

	// --- Schema-registry (kode-defineret content-model, Fase 1) ---
	registry, err := schema.DefaultRegistry()
	if err != nil {
		return fmt.Errorf("load schema registry: %w", err)
	}
	logger.Info("schema_registry_loaded", "docTypes", len(registry.DocTypes()), "blocks", len(registry.Blocks()))

	// --- Media (Fase 3): asset store + storage backend + optional video plugin ---
	var mediaStore media.Store
	if pool != nil {
		mediaStore = media.NewPostgresStore(pool)
	} else {
		mediaStore = media.NewMemoryStore()
	}
	backendName, err := cfg.StorageBackendName()
	if err != nil {
		return err
	}
	var backend media.StorageBackend
	switch backendName {
	case "bunny":
		backend = media.NewBunnyBackend(cfg.StorageEndpoint, cfg.StorageZone, cfg.StorageKey)
	case "disk":
		disk, err := media.NewDiskBackend(cfg.StorageDir)
		if err != nil {
			return err
		}
		backend = disk
	default:
		logger.Warn("no asset storage configured — using the in-memory backend (dev/test); uploads are lost on restart")
		backend = media.NewMemoryBackend()
	}
	logger.Info("storage_backend", "backend", backend.Name(), "cdnBase", cfg.CDNBase)
	mediaSvc := media.NewService(mediaStore, backend, media.NewURLBuilder(cfg.CDNBase))

	var videoProvider media.VideoProvider
	if cfg.VideoProvider != "" {
		vp, ok := media.ProviderByName(cfg.VideoProvider)
		if !ok {
			return fmt.Errorf("unknown video provider %q", cfg.VideoProvider)
		}
		videoProvider = vp
		logger.Info("video_plugin_enabled", "provider", vp.Name())
	}

	// --- Per-tenant settings (content profile) ---
	var settingsStore settings.Store
	if pool != nil {
		settingsStore = settings.NewPostgresStore(pool)
	} else {
		settingsStore = settings.NewMemoryStore()
	}

	// --- Editor REST router (tenant-scoped, JWT-gated) ---
	apiRouter := httpapi.NewRouter(httpapi.Deps{
		Store:         store,
		Registry:      registry,
		Media:         mediaSvc,
		Video:         videoProvider,
		Verifier:      verifier,
		Settings:      settingsStore,
		PreviewSecret: cfg.PreviewSecret,
		Assets:        &mediaAssetResolver{media: mediaSvc},
		Logger:        logger,
	})

	// --- Delivery read router (published-only, service-auth'd) ---
	deliveryRouter := delivery.NewRouter(delivery.Deps{
		Store:         store,
		Registry:      registry,
		Assets:        &mediaAssetResolver{media: mediaSvc},
		ServiceToken:  cfg.DeliveryToken,
		PreviewSecret: cfg.PreviewSecret,
		Logger:        logger,
	})

	mux := http.NewServeMux()
	mux.Handle("/api/", apiRouter)
	mux.Handle("/delivery/", deliveryRouter)

	// Serve asset bytes ourselves when no CDN holds them. Bunny is a ByteServer
	// on purpose NOT: with a pull-zone configured the CDN answers these URLs and
	// mounting an origin route would only invite traffic past it.
	if server, ok := backend.(media.ByteServer); ok {
		mux.Handle("GET "+media.AssetPath+"{key...}", media.AssetHandler(server))
		logger.Info("asset_route_mounted", "path", media.AssetPath)
	}
	// Visual-editing bridge script (public, no tenant data).
	mux.Handle(bridge.Path, bridge.Handler())

	// HTTP probes used by the Docker HEALTHCHECK + Caddy (unauthenticated).
	// Build identity is marshaled once at boot: it never changes, and a probe
	// must not be able to fail on serialization.
	healthBody, err := healthPayload(cfg.SourceURL)
	if err != nil {
		return fmt.Errorf("marshal health body: %w", err)
	}
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(healthBody)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})

	root := otelhttp.NewHandler(mux, "resonans.http")
	srv := &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           root,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("resonans listening", "host", cfg.Host, "port", cfg.Port, "publicURL", cfg.PublicURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http server: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown_signal_received")
	case err := <-errCh:
		logger.Error("fatal_error", "err", err)
		return err
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Error("http_shutdown_error", "err", err)
	}
	logger.Info("stopped")
	return nil
}

// Seed inserts the explainers demo dataset for the tenant in RESONANS_SEED_TENANT.
// Run as a one-shot (`resonans seed`) against staging so consumers (Etude M5) can
// render real content. Connects as the app role; the stores set the tenant GUC.
func Seed(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required for seed")
	}
	tenantID, err := uuid.Parse(strings.TrimSpace(os.Getenv("RESONANS_SEED_TENANT")))
	if err != nil {
		return fmt.Errorf("RESONANS_SEED_TENANT must be a tenant uuid: %w", err)
	}
	pool, err := persistence.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()
	reg, err := schema.DefaultRegistry()
	if err != nil {
		return fmt.Errorf("load schema: %w", err)
	}
	if err := seed.Run(ctx, content.NewPostgresStore(pool), media.NewPostgresStore(pool), reg, tenantID); err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	newStdoutLogger(cfg.LogLevel, cfg.OTelServiceName, cfg.NodeEnv).Info("seed_complete", "tenant", tenantID)
	return nil
}

// Migrate runs pending migrations and returns. Used by the `resonans migrate`
// one-shot deploy service, which connects as the BYPASSRLS migrator-role (via
// DATABASE_URL) so the long-running server (NOBYPASSRLS app-role) never needs
// DDL privileges (CTO deploy-wiring, Option A).
func Migrate(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required for migrate")
	}
	pool, err := persistence.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	applied, err := persistence.Migrate(ctx, pool)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	newStdoutLogger(cfg.LogLevel, cfg.OTelServiceName, cfg.NodeEnv).
		Info("migrations_complete", "applied", applied)
	return nil
}

// SanitizeBackfill runs the rich-text backfill over documents and
// document_versions (internal/backfill). Dry-run unless apply; writes a report
// with counts and ids/paths only. Must run as the migrator role (BYPASSRLS).
func SanitizeBackfill(ctx context.Context, apply bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required for sanitize-backfill")
	}
	registry, err := schema.DefaultRegistry()
	if err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	pool, err := persistence.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	logger := newStdoutLogger(cfg.LogLevel, cfg.OTelServiceName, cfg.NodeEnv)
	mode := "dry-run"
	if apply {
		mode = "apply"
	}
	reports, err := backfill.Run(ctx, backfill.PostgresStore{Pool: pool}, registry, apply)
	for _, r := range reports {
		logger.Info("sanitize_backfill", "mode", mode, "table", r.Table, "scanned", r.Scanned,
			"unchanged", r.Unchanged, "changed", r.Changed, "blanked", r.Blanked,
			"skipped_concurrent", r.SkippedConcurrent,
			"unknown_type", r.UnknownType, "invalid", r.Invalid, "findings", r.Findings)
	}
	return err
}

// SlugAudit reports stored slugs that create now rejects (read-only). Runs as
// the migrator role, like sanitize-backfill.
func SlugAudit(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required for slug-audit")
	}
	pool, err := persistence.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()
	rep, err := backfill.AuditSlugs(ctx, backfill.PostgresStore{Pool: pool}, content.ValidSlug)
	newStdoutLogger(cfg.LogLevel, cfg.OTelServiceName, cfg.NodeEnv).
		Info("slug_audit", "scanned", rep.Scanned, "invalid", rep.Invalid, "findings", rep.Findings)
	return err
}

// mediaAssetResolver adapts the media service to delivery.AssetResolver: it
// expands a stored asset id to `{url, width?, height?}` (Bunny CDN URL) so the
// delivery envelope carries ready-to-render assets.
type mediaAssetResolver struct{ media *media.Service }

func (m *mediaAssetResolver) ResolveAsset(ctx context.Context, tenantID uuid.UUID, assetID string) (map[string]any, bool) {
	id, err := uuid.Parse(assetID)
	if err != nil {
		return nil, false
	}
	a, err := m.media.Get(ctx, tenantID, id)
	if err != nil {
		return nil, false
	}
	out := map[string]any{"url": m.media.URL(a, media.Transform{})}
	if a.Width != nil {
		out["width"] = *a.Width
	}
	if a.Height != nil {
		out["height"] = *a.Height
	}
	if a.SizeBytes > 0 {
		out["sizeBytes"] = a.SizeBytes
	}
	return out, true
}

// healthPayload is the /healthz body. Split out so a test can assert the probe
// reports the STAMPED identity: the constant `{"status":"ok"}` it replaces would
// have satisfied any test that only checked the status field.
func healthPayload(sourceURL string) ([]byte, error) {
	return json.Marshal(map[string]string{
		"status":  "ok",
		"service": "resonans",
		"version": version,
		// AGPL-3.0 §13: a user interacting with this instance over a network is
		// entitled to its Corresponding Source. An unauthenticated probe is the
		// one surface every such user can reach, so the offer lives here rather
		// than only in a README nobody deploying a container will read.
		"source":   sourceURL,
		"revision": revision,
	})
}

// Version returns the build-stamped version (exposed for the healthcheck cmd).
func Version() string { return version }

// Revision returns the build-stamped git sha, or "unknown" when the binary was
// not built through the release pipeline.
func Revision() string { return revision }

// Package app wires the Resonans CMS: config -> otel -> jwt -> db/migrate ->
// content store -> REST router -> graceful shutdown. It mirrors the boot
// structure of the permissions/notifications Go services.
package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/KlangHaus/resonans/internal/auth"
	"github.com/KlangHaus/resonans/internal/config"
	"github.com/KlangHaus/resonans/internal/content"
	"github.com/KlangHaus/resonans/internal/delivery"
	"github.com/KlangHaus/resonans/internal/httpapi"
	"github.com/KlangHaus/resonans/internal/media"
	"github.com/KlangHaus/resonans/internal/observability"
	"github.com/KlangHaus/resonans/internal/persistence"
	"github.com/KlangHaus/resonans/internal/schema"
)

// version is stamped via -ldflags at build time.
var version = "0.0.0"

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
	var verifier auth.Verifier
	switch {
	case cfg.AuthEnabled():
		verifier, err = auth.NewVerifier(ctx, auth.VerifierConfig{
			Issuer:  cfg.JWTIssuer,
			JWKSURL: cfg.JWKSURL,
		})
		if err != nil {
			return fmt.Errorf("jwt verifier: %w", err)
		}
		logger.Info("jwt_verifier_active", "issuer", cfg.JWTIssuer)
	case cfg.IsProduction():
		// Serving content without verifying the tenant claim would break tenant
		// isolation (Beslutning K) — refuse to boot with open endpoints.
		return errors.New(
			"RESONANS_JWT_ISSUER + RESONANS_JWKS_URL not set in production — refusing to start with unauthenticated endpoints",
		)
	default:
		logger.Warn("RESONANS_JWT_ISSUER + RESONANS_JWKS_URL not set — JWT auth DISABLED; tenant via X-Dev-Tenant header (dev/test only)")
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

		applied, err := persistence.Migrate(ctx, pool)
		if err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
		if len(applied) > 0 {
			logger.Info("migrations_applied", "migrations", applied)
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
	if cfg.DeliveryToken == "" && cfg.IsProduction() {
		// Without a service token the /delivery endpoints would trust an
		// unauthenticated X-Scope-OrgID header — a cross-tenant read hole.
		return errors.New(
			"RESONANS_DELIVERY_TOKEN not set in production — refusing to start with an unauthenticated content-delivery surface",
		)
	}
	if cfg.DeliveryToken == "" {
		logger.Warn("RESONANS_DELIVERY_TOKEN not set — /delivery trusts X-Scope-OrgID directly (dev/test only)")
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
	var backend media.StorageBackend
	switch {
	case cfg.StorageConfigured():
		backend = media.NewBunnyBackend(cfg.StorageEndpoint, cfg.StorageZone, cfg.StorageKey)
		logger.Info("storage_backend", "backend", "bunny")
	case cfg.IsProduction():
		logger.Warn("BUNNY_STORAGE_* not set in production — asset bytes use non-persistent in-memory backend")
		backend = media.NewMemoryBackend()
	default:
		logger.Warn("BUNNY_STORAGE_* not set — using in-memory storage backend (dev/test)")
		backend = media.NewMemoryBackend()
	}
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

	// --- Editor REST router (tenant-scoped, JWT-gated) ---
	apiRouter := httpapi.NewRouter(httpapi.Deps{
		Store:    store,
		Registry: registry,
		Media:    mediaSvc,
		Video:    videoProvider,
		Verifier: verifier,
		Logger:   logger,
	})

	// --- Delivery read router (published-only, service-auth'd) ---
	deliveryRouter := delivery.NewRouter(delivery.Deps{
		Store:        store,
		ServiceToken: cfg.DeliveryToken,
		Logger:       logger,
	})

	mux := http.NewServeMux()
	mux.Handle("/api/", apiRouter)
	mux.Handle("/delivery/", deliveryRouter)

	// HTTP probes used by the Docker HEALTHCHECK + Caddy (unauthenticated).
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
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

// Version returns the build-stamped version (exposed for the healthcheck cmd).
func Version() string { return version }

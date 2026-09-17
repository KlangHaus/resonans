// Command resonans is the KlangHaus headless CMS (Resonans) server.
//
// Subcommands:
//
//	resonans              run the server (default)
//	resonans healthcheck  probe /healthz on PORT and exit 0/1 (distroless
//	                      HEALTHCHECK — no curl/wget in the image)
//	resonans version      print the build version
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/KlangHaus/resonans/internal/app"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			os.Exit(healthcheck())
		case "migrate":
			os.Exit(runMigrate())
		case "seed":
			os.Exit(runSeed())
		case "sanitize-backfill":
			os.Exit(runSanitizeBackfill(os.Args[2:]))
		case "slug-audit":
			os.Exit(runSlugAudit())
		case "help", "-h", "--help":
			usage(os.Stdout)
			return
		case "version", "--version":
			_, _ = fmt.Fprintln(os.Stdout, app.Version())
			return
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx); err != nil {
		slog.Error("resonans exited with error", "error", err)
		os.Exit(1)
	}
}

// healthcheck probes the local /healthz endpoint and exits accordingly.
// Distroless ships no shell/curl, so the container HEALTHCHECK invokes the
// binary itself in this mode (same pattern as permissions/notifications).
func healthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	if _, err := strconv.Atoi(port); err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: invalid PORT=%q\n", port)
		return 1
	}
	host := os.Getenv("HEALTHCHECK_HOST")
	if host == "" {
		host = "localhost"
	}

	timeout := 3 * time.Second
	if v := os.Getenv("HEALTHCHECK_TIMEOUT_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			timeout = time.Duration(ms) * time.Millisecond
		}
	}

	url := fmt.Sprintf("http://%s:%s/healthz", host, port)
	c := &http.Client{Timeout: timeout}
	// #nosec G704 -- localhost /healthz probe; host/port stammer fra betroet
	// container-env (HEALTHCHECK_HOST/PORT), ikke bruger-input. Standard
	// distroless-healthcheck-mønster (samme som permissions/notifications).
	resp, err := c.Get(url) //nolint:noctx // kortlivet probe med client-timeout
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck: status %d\n", resp.StatusCode)
		return 1
	}
	return 0
}

// runMigrate runs pending DB migrations and exits. The deploy runs this as a
// one-shot as the migrator-role before starting the server (CTO Option A).
func runMigrate() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Migrate(ctx); err != nil {
		slog.Error("migrate failed", "error", err)
		return 1
	}
	return 0
}

// runSeed inserts the explainers demo dataset (RESONANS_SEED_TENANT) and exits.
func runSeed() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Seed(ctx); err != nil {
		slog.Error("seed failed", "error", err)
		return 1
	}
	return 0
}

// runSanitizeBackfill cleans stored rich text. Dry-run unless --apply.
func runSanitizeBackfill(args []string) int {
	apply := false
	for _, a := range args {
		switch a {
		case "--apply":
			apply = true
		default:
			fmt.Fprintf(os.Stderr, "sanitize-backfill: unknown argument %q (only --apply)\n", a)
			return 2
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.SanitizeBackfill(ctx, apply); err != nil {
		slog.Error("sanitize-backfill failed", "error", err)
		return 1
	}
	return 0
}

// runSlugAudit lists stored slugs that create now rejects. Read-only.
func runSlugAudit() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.SlugAudit(ctx); err != nil {
		slog.Error("slug-audit failed", "error", err)
		return 1
	}
	return 0
}

func usage(w *os.File) {
	_, _ = fmt.Fprintln(w, `resonans — KlangHaus headless CMS server

Usage:
  resonans              run the server
  resonans migrate      run pending DB migrations and exit (as migrator role)
  resonans seed         insert the explainers demo dataset (RESONANS_SEED_TENANT) and exit
  resonans sanitize-backfill [--apply]
                        clean stored rich text in documents + document_versions
                        (dry-run unless --apply; as migrator role)
  resonans slug-audit   list stored slugs that create now rejects (read-only; as migrator role)
  resonans healthcheck  probe /healthz on $PORT and exit 0 (ok) / 1 (fail)
  resonans version      print the build version
  resonans help         show this message`)
}

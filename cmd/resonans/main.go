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

func usage(w *os.File) {
	_, _ = fmt.Fprintln(w, `resonans — KlangHaus headless CMS server

Usage:
  resonans              run the server
  resonans healthcheck  probe /healthz on $PORT and exit 0 (ok) / 1 (fail)
  resonans version      print the build version
  resonans help         show this message`)
}

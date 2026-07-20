// Package observability wires OpenTelemetry (traces + metrics + logs) for the
// Resonans CMS, mirroring the permissions/notifications services. Export is
// OTLP/HTTP via the standard OTEL_* environment variables. When no endpoint is
// configured the setup is a no-op: global providers stay no-op and logging is
// plain stdout JSON, keeping local dev free of a Collector.
//
// Product brief: OTel fra dag ét. site/tenant.id is set per-request on spans,
// never on the resource (cardinality too high for cross-site metrics).
package observability

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Providers holds the shutdown hooks and the (possibly OTel-bridged) logger.
type Providers struct {
	Logger    *slog.Logger
	shutdowns []func(context.Context) error
}

// Options control identity attributes on the OTel resource (ADR 0002 §E).
type Options struct {
	ServiceName      string
	ServiceNamespace string
	Version          string
	Environment      string
	// Forced configures exporters even if the standard OTEL_* endpoint env vars
	// are unset (used so OTEL_ENABLED=true works as a single toggle).
	Forced bool
}

// Enabled reports whether OTLP export is configured via the standard env vars.
func Enabled() bool {
	if os.Getenv("OTEL_SDK_DISABLED") == "true" {
		return false
	}
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT") != ""
}

func buildResource(ctx context.Context, opts Options) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		attribute.String("service.name", opts.ServiceName),
		attribute.String("service.version", opts.Version),
	}
	if opts.ServiceNamespace != "" {
		attrs = append(attrs, attribute.String("service.namespace", opts.ServiceNamespace))
	}
	if opts.Environment != "" {
		attrs = append(attrs, attribute.String("deployment.environment.name", opts.Environment))
	}
	return resource.New(
		ctx,
		resource.WithTelemetrySDK(),
		resource.WithAttributes(attrs...),
		resource.WithFromEnv(),
	)
}

// Setup initializes the trace, metric and log providers. stdout is the base
// logger; when OTel is active, logs are tee'd to the OTLP log exporter too.
func Setup(ctx context.Context, opts Options, stdout *slog.Logger) (*Providers, error) {
	p := &Providers{Logger: stdout}
	if !opts.Forced && !Enabled() {
		return p, nil
	}

	res, err := buildResource(ctx, opts)
	if err != nil {
		return nil, err
	}

	traceExp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	p.shutdowns = append(p.shutdowns, tp.Shutdown)

	metricExp, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)
	p.shutdowns = append(p.shutdowns, mp.Shutdown)

	logExp, err := otlploghttp.New(ctx)
	if err != nil {
		return nil, err
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
		sdklog.WithResource(res),
	)
	global.SetLoggerProvider(lp)
	p.shutdowns = append(p.shutdowns, lp.Shutdown)

	otelHandler := otelslog.NewHandler(opts.ServiceName, otelslog.WithLoggerProvider(lp))
	p.Logger = slog.New(fanout{handlers: []slog.Handler{stdout.Handler(), otelHandler}})

	return p, nil
}

// Shutdown flushes and stops all providers in reverse order. Safe when disabled.
func (p *Providers) Shutdown(ctx context.Context) error {
	var firstErr error
	for i := len(p.shutdowns) - 1; i >= 0; i-- {
		if err := p.shutdowns[i](ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

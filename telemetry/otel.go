package telemetry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// ShutdownFunc shuts down telemetry providers.
type ShutdownFunc func(ctx context.Context) error

// Init wires telemetry according to Config. Call once on startup.
func Init(ctx context.Context, cfg Config) (ShutdownFunc, error) {
	if cfg.ServiceName == "" {
		return nil, errors.New("telemetry: ServiceName is required")
	}
	if cfg.StartupTimeout <= 0 {
		cfg.StartupTimeout = 5 * time.Second
	}
	if cfg.Mode == "" {
		cfg.Mode = ModeDetect
	}

	autoDetected := detectGoAuto()

	switch cfg.Mode {
	case ModeAuto:
		return initAutoMode(autoDetected)

	case ModeManual:
		return initManualMode(ctx, cfg)

	case ModeDetect:
		if autoDetected {
			return initAutoMode(true)
		}
		return initManualMode(ctx, cfg)

	default:
		return nil, fmt.Errorf("telemetry: unknown Mode %q", cfg.Mode)
	}
}

// detectGoAuto checks for official Go auto-instrumentation signals.
// OTEL_GO_AUTO_TARGET_EXE is required by the Operator’s Go auto-instrumentation.
func detectGoAuto() bool {
	if os.Getenv("OTEL_GO_AUTO_TARGET_EXE") != "" {
		return true
	}
	// Optional org-specific escape hatch.
	switch strings.ToLower(os.Getenv("OTEL_GO_AUTO_ENABLED")) {
	case "true", "1", "yes":
		return true
	}
	return false
}

// In Auto mode we do NOT set our own TracerProvider, so we don't fight Auto SDK / sidecar.
func initAutoMode(autoDetected bool) (ShutdownFunc, error) {
	// Ensure we at least have sane propagators if nothing else set.
	if isNoopPropagator(otel.GetTextMapPropagator()) {
		otel.SetTextMapPropagator(
			propagation.NewCompositeTextMapPropagator(
				propagation.TraceContext{},
				propagation.Baggage{},
			),
		)
	}

	if autoDetected {
		// Sidecar / Auto SDK will own TP+exporters.
		return func(context.Context) error { return nil }, nil
	}

	// Config says "auto" but nothing detected — treat as noop and complain to stderr.
	fmt.Fprintln(os.Stderr, "[telemetry] ModeAuto/Detect set but no Go auto-instrumentation detected; using global no-op")
	return func(context.Context) error { return nil }, nil
}

// Manual mode: standard OTel SDK + OTLP exporter
func initManualMode(parent context.Context, cfg Config) (ShutdownFunc, error) {
	ctx, cancel := context.WithTimeout(parent, cfg.StartupTimeout)
	defer cancel()

	res, err := buildResource(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("telemetry: build resource: %w", err)
	}

	var exp sdktrace.SpanExporter
	if os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL") == "grpc" {
		exp, err = buildGRPCTraceExporter(ctx, cfg)
	} else {
		exp, err = buildHTTPTraceExporter(ctx, cfg)
	}
	if err != nil {
		return nil, fmt.Errorf("telemetry: build trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(buildSampler(cfg.SamplerRatio)),
	)

	otel.SetTracerProvider(tp)

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	// MeterProvider setup can be added here later, similar pattern.

	return func(ctx context.Context) error {
		if err := tp.Shutdown(ctx); err != nil {
			return fmt.Errorf("telemetry: tracer provider shutdown: %w", err)
		}
		return nil
	}, nil
}

func buildResource(ctx context.Context, cfg Config) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceNameKey.String(cfg.ServiceName),
	}

	if cfg.ServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersionKey.String(cfg.ServiceVersion))
	}
	if cfg.Environment != "" {
		attrs = append(attrs, attribute.String("deployment.environment", cfg.Environment))
	}
	for k, v := range cfg.ResourceAttrs {
		attrs = append(attrs, attribute.String(k, v))
	}

	return resource.New(
		ctx,
		resource.WithFromEnv(),      // OTEL_RESOURCE_ATTRIBUTES, etc.
		resource.WithTelemetrySDK(), // telemetry.sdk.*
		resource.WithHost(),
		resource.WithOS(),
		resource.WithAttributes(attrs...),
	)
}

func buildGRPCTraceExporter(ctx context.Context, cfg Config) (sdktrace.SpanExporter, error) {
	var opts []otlptracegrpc.Option

	if cfg.OTLPEndpoint != "" {
		endpoint := cfg.OTLPEndpoint

		// Convenience: if no scheme, derive from Insecure.
		if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
			if cfg.Insecure {
				endpoint = "http://" + endpoint
			} else {
				endpoint = "https://" + endpoint
			}
		}

		opts = append(opts, otlptracegrpc.WithEndpoint(endpoint))
	}

	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	// If neither OTLPEndpoint nor Insecure provided, exporter relies on OTEL_EXPORTER_OTLP_* env vars.

	return otlptracegrpc.New(ctx, opts...)
}

func buildHTTPTraceExporter(ctx context.Context, cfg Config) (sdktrace.SpanExporter, error) {
	opts := []otlptracehttp.Option{}

	if cfg.OTLPEndpoint != "" {
		ep := cfg.OTLPEndpoint

		switch {
		// Full URL: e.g. "http://coroot:8080/v1/traces"
		case strings.HasPrefix(ep, "http://") || strings.HasPrefix(ep, "https://"):
			opts = append(opts, otlptracehttp.WithEndpointURL(ep))

		// Host:port: e.g. "coroot:8080" or "otel-collector:4318"
		default:
			scheme := "https"
			if cfg.Insecure {
				scheme = "http"
			}
			// Here we treat it as base OTLP endpoint; /v1/traces will be appended.
			base := fmt.Sprintf("%s://%s", scheme, ep)
			opts = append(opts, otlptracehttp.WithEndpoint(base))
		}
	}

	if cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	// If cfg.OTLPEndpoint is empty, exporter will rely on:
	//   OTEL_EXPORTER_OTLP_TRACES_ENDPOINT or OTEL_EXPORTER_OTLP_ENDPOINT
	return otlptracehttp.New(ctx, opts...)
}

func buildSampler(ratio float64) sdktrace.Sampler {
	switch {
	case ratio <= 0:
		return sdktrace.NeverSample()
	case ratio >= 1:
		return sdktrace.AlwaysSample()
	default:
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
	}
}

func isNoopPropagator(p propagation.TextMapPropagator) bool {
	return p == nil || fmt.Sprint(p) == "{}"
}

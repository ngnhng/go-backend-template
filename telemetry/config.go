package telemetry

import "time"

type Mode string

const (
	ModeDetect Mode = "detect"
	ModeManual Mode = "manual"
	ModeAuto   Mode = "auto"
)

type Config struct {
	ServiceName    string `env:"SERVICE_NAME"`
	ServiceVersion string
	Environment    string

	// Optional; if empty, OTEL_EXPORTER_OTLP_(TRACES_)ENDPOINT is used.
	// Can be "http://otel-collector:4317" or just "otel-collector:4317"
	// (we'll normalize).
	OTLPEndpoint string

	// If true, disable TLS for OTLP (or set OTEL_EXPORTER_OTLP_TRACES_INSECURE).
	Insecure bool

	// 0..1: sampling ratio (0=never,1=all,else parentbased+ratio).
	SamplerRatio float64

	StartupTimeout time.Duration

	// How to interact with Go auto-instrumentation / Auto SDK.
	Mode Mode

	DisableMetrics bool

	// Extra resource attributes.
	ResourceAttrs map[string]string
}

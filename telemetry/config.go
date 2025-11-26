package telemetry

import "time"

type Mode string

const (
	ModeDetect Mode = "detect"
	ModeManual Mode = "manual"
	ModeAuto   Mode = "auto"
)

type Config struct {
	ServiceName    string `env:"OTEL_SERVICE_NAME" envDefault:"profile-api"`
	ServiceVersion string `env:"SERVICE_VERSION" envDefault:"dev"`
	Environment    string `env:"ENVIRONMENT" envDefault:"local"`

	// Optional; if empty, OTEL_EXPORTER_OTLP_(TRACES_)ENDPOINT is used.
	// Can be "http://otel-collector:4317" or just "otel-collector:4317"
	// (we'll normalize).
	OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT" envDefault:"otel-collector:4317"`

	// If true, disable TLS for OTLP (or set OTEL_EXPORTER_OTLP_TRACES_INSECURE).
	Insecure bool `env:"OTEL_EXPORTER_OTLP_TRACES_INSECURE"`

	// 0..1: sampling ratio (0=never,1=all,else parentbased+ratio).
	SamplerRatio float64 `envDefault:"1"`

	StartupTimeout time.Duration `envDefault:"5s"`

	// How to interact with Go auto-instrumentation / Auto SDK.
	Mode Mode `envDefault:"detect"`

	DisableMetrics bool `envDefault:"false"`

	// Extra resource attributes.
	ResourceAttrs map[string]string `env:"OTEL_RESOURCE_ATTRIBUTES" envDefault:"deployment.environment=local,service.version=dev" envSeparator:"," envKeySeparator:"="`
}

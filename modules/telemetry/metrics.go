// Copyright 2025 Nhat-Nguyen Nguyen
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// HTTPMetrics holds counters and histograms for HTTP endpoint instrumentation
type HTTPMetrics struct {
	requestCounter    metric.Int64Counter
	durationHisto     metric.Float64Histogram
	responseSizeHisto metric.Int64Histogram
}

// NewHTTPMetrics creates a new HTTPMetrics instance for a given service name
func NewHTTPMetrics(serviceName string) (*HTTPMetrics, error) {
	meter := otel.Meter(serviceName)

	requestCounter, err := meter.Int64Counter(
		"http_server_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	durationHisto, err := meter.Float64Histogram(
		"http_server_duration",
		metric.WithDescription("HTTP request duration"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	responseSizeHisto, err := meter.Int64Histogram(
		"http_server_response_size",
		metric.WithDescription("HTTP response size in bytes"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, err
	}

	return &HTTPMetrics{
		requestCounter:    requestCounter,
		durationHisto:     durationHisto,
		responseSizeHisto: responseSizeHisto,
	}, nil
}

// RecordRequest records a single HTTP request with its attributes
func (m *HTTPMetrics) RecordRequest(ctx context.Context, method, endpoint, statusCode string, durationMs float64, responseSize int64) {
	attrs := []attribute.KeyValue{
		attribute.String("http_method", method),
		attribute.String("http_endpoint", endpoint),
		attribute.String("http_status_code", statusCode),
	}

	m.requestCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.durationHisto.Record(ctx, durationMs, metric.WithAttributes(attrs...))
	if responseSize > 0 {
		m.responseSizeHisto.Record(ctx, responseSize, metric.WithAttributes(attrs...))
	}
}

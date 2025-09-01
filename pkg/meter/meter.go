package meter

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/embedded"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
)

// Config holds the metrics configuration.
type Config struct {
	Address     string
	Insecure    bool
	Username    string
	Password    string
	ServiceName string
}

// OTLPProvider is a metrics provider which exports metrics to OTLP.
type OTLPProvider struct {
	embedded.MeterProvider

	provider *sdkmetric.MeterProvider
	reader   sdkmetric.Reader
	exporter sdkmetric.Exporter
}

// NewOTLPProvider creates a new OTLPProvider.
func NewOTLPProvider(ctx context.Context, cfg Config) (*OTLPProvider, error) {
	auth := base64.StdEncoding.EncodeToString([]byte(cfg.Username + ":" + cfg.Password))

	options := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(cfg.Address),
		otlpmetrichttp.WithHeaders(map[string]string{"Authorization": "Basic " + auth}),
	}

	if cfg.Insecure {
		options = append(options, otlpmetrichttp.WithInsecure())
	}

	exporter, err := otlpmetrichttp.New(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	reader := sdkmetric.NewManualReader()

	return &OTLPProvider{
		provider: sdkmetric.NewMeterProvider(
			sdkmetric.WithResource(resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceNameKey.String(cfg.ServiceName),
				attribute.String("exporter", "prometheus"),
				attribute.String("namespace", currentNamespace()),
			)),
			sdkmetric.WithReader(
				sdkmetric.NewPeriodicReader(exporter),
			),
		),
		exporter: exporter,
		reader:   reader,
	}, nil
}

// Meter returns a Meter with the given name and configured with options.
func (p *OTLPProvider) Meter(name string, options ...metric.MeterOption) metric.Meter {
	return p.provider.Meter(name, options...)
}

// Stop stops the provider once all metrics have been uploaded.
func (p *OTLPProvider) Stop(ctx context.Context) error {
	if err := p.provider.ForceFlush(ctx); err != nil {
		return fmt.Errorf("flushing provider: %w", err)
	}

	if err := p.provider.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutting down provider: %w", err)
	}

	return nil
}

func currentNamespace() string {
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}

	if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		if ns := strings.TrimSpace(string(data)); ns != "" {
			return ns
		}
	}

	return "default"
}

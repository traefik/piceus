package tracer

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"
)

// Config holds the tracing configuration.
type Config struct {
	Address     string
	Insecure    bool
	Username    string
	Password    string
	Probability float64
	ServiceName string
}

// NewTracer creates a new trace.Tracer.
func NewTracer(ctx context.Context, cfg Config) (trace.Tracer, io.Closer, error) {
	auth := base64.StdEncoding.EncodeToString([]byte(cfg.Username + ":" + cfg.Password))

	options := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(cfg.Address),
		otlptracehttp.WithHeaders(map[string]string{"Authorization": "Basic " + auth}),
	}

	if cfg.Insecure {
		options = append(options, otlptracehttp.WithInsecure())
	}

	exporter, err := otlptracehttp.New(ctx, options...)
	if err != nil {
		return nil, nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			attribute.String("exporter", "jaeger"),
			attribute.String("namespace", currentNamespace()),
		),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithOSType(),
		resource.WithProcessCommandArgs(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("building resource: %w", err)
	}

	// Register the trace exporter with a TracerProvider, using a batch
	// span processor to aggregate spans before export.
	bsp := sdktrace.NewBatchSpanProcessor(exporter)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.Probability))),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	log.Debug().Msg("OpenTelemetry tracer configured")

	return tracerProvider.Tracer("github.com/traefik/plugin-service"), &tpCloser{
		provider: tracerProvider,
		exporter: exporter,
	}, nil
}

// tpCloser converts a TraceProvider into an io.Closer.
type tpCloser struct {
	provider *sdktrace.TracerProvider
	exporter *otlptrace.Exporter
}

func (t *tpCloser) Close() error {
	if t == nil {
		return nil
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer cancel()

	if err := t.exporter.Shutdown(ctx); err != nil {
		return err
	}

	return t.provider.Shutdown(ctx)
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

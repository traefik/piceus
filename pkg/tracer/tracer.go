package tracer

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/embedded"
)

type traceIDSet struct{}

// Config holds the tracing configuration.
type Config struct {
	Address     string
	Insecure    bool
	Username    string
	Password    string
	Probability float64
	ServiceName string
}

// OTLP is an extension of otel tracer to add custom logging.
type OTLP struct {
	embedded.Tracer

	tracer trace.Tracer
}

// Start returns a new span with the corresponding trace id field filled.
func (t *OTLP) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	ctx, span := t.tracer.Start(ctx, spanName, opts...)

	if !span.SpanContext().IsSampled() || ctx.Value(traceIDSet{}) != nil {
		return ctx, span
	}

	logger := log.Ctx(ctx).With().Str("trace_id", span.SpanContext().TraceID().String()).Logger()
	ctx = context.WithValue(ctx, traceIDSet{}, struct{}{})

	return logger.WithContext(ctx), span
}

// OTLPProvider is a trace provider which exports traces to OTLP.
type OTLPProvider struct {
	embedded.TracerProvider

	provider *sdktrace.TracerProvider
	exporter *otlptrace.Exporter
}

// NewOTLPProvider creates a new OTLPProvider.
func NewOTLPProvider(ctx context.Context, cfg Config) (*OTLPProvider, error) {
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
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	bsp := sdktrace.NewBatchSpanProcessor(exporter)
	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.Probability))

	tc := propagation.TraceContext{}
	otel.SetTextMapPropagator(tc)

	return &OTLPProvider{
		provider: sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sampler),
			sdktrace.WithResource(resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceNameKey.String(cfg.ServiceName),
				attribute.String("exporter", "jaeger"),
				attribute.String("namespace", currentNamespace()),
			)),
			sdktrace.WithSpanProcessor(bsp),
		),
		exporter: exporter,
	}, nil
}

// Tracer creates a new tracer with the given name and options.
func (p *OTLPProvider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return &OTLP{
		tracer: p.provider.Tracer(name, opts...),
	}
}

// Stop stops the provider once all traces have been uploaded.
func (p *OTLPProvider) Stop(ctx context.Context) error {
	if err := p.exporter.Shutdown(ctx); err != nil {
		return err
	}

	return p.provider.Shutdown(ctx)
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

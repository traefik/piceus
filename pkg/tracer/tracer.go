package tracer

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

const serviceName = "piceus"

// Setup setups the tracer.
func Setup(exporter sdktrace.SpanExporter, probability float64) sdktrace.SpanProcessor {
	bsp := sdktrace.NewBatchSpanProcessor(exporter)

	sampler := sdktrace.TraceIDRatioBased(probability)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sampler)),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			attribute.String("exporter", "jaeger"),
		)),
		sdktrace.WithSpanProcessor(bsp),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}))

	return bsp
}

// NewJaegerExporter creates a new Jaeger exporter.
func NewJaegerExporter(endpoint, username, password string) (*jaeger.Exporter, error) {
	return jaeger.New(
		jaeger.WithCollectorEndpoint(
			jaeger.WithEndpoint(endpoint+"/api/traces"),
			jaeger.WithUsername(username),
			jaeger.WithPassword(password),
		))
}

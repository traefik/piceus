package tracer

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/trace/jaeger"
	"go.opentelemetry.io/otel/label"
	"go.opentelemetry.io/otel/propagation"
	export "go.opentelemetry.io/otel/sdk/export/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const serviceName = "piceus"

// Setup setups the tracer.
func Setup(exporter export.SpanExporter, probability float64) *sdktrace.BatchSpanProcessor {
	bsp := sdktrace.NewBatchSpanProcessor(exporter)

	sampler := sdktrace.TraceIDRatioBased(probability)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithConfig(sdktrace.Config{DefaultSampler: sdktrace.ParentBased(sampler)}),
		sdktrace.WithSpanProcessor(bsp),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}))

	return bsp
}

// NewJaegerExporter creates a new Jaeger exporter.
func NewJaegerExporter(endpoint, username, password string) (*jaeger.Exporter, error) {
	return jaeger.NewRawExporter(
		jaeger.WithCollectorEndpoint(endpoint+"/api/traces",
			jaeger.WithUsername(username),
			jaeger.WithPassword(password),
		),
		jaeger.WithProcess(jaeger.Process{
			ServiceName: serviceName,
			Tags: []label.KeyValue{
				label.String("exporter", "jaeger"),
			},
		}))
}

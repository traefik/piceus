package client

import (
	"context"
	"fmt"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type MetricsClient struct {
	enabled bool
}

func (mc MetricsClient) Apply(ctx context.Context, c *Client) error {
	if !mc.enabled {
		c.client.Transport = otelhttp.NewTransport(c.client.Transport)
		return nil
	}

	m := otel.Meter("piceus")
	requestCounter, err := m.Int64Counter(
		"http.requests.total",
		metric.WithDescription("Number of API calls."),
		metric.WithUnit("requests"),
	)
	if err != nil {
		return fmt.Errorf("creating counter: %w", err)
	}

	c.client.Transport = &githubMetricsTripper{
		requestCounter: requestCounter,
		next:           otelhttp.NewTransport(c.client.Transport),
	}

	return nil
}

type githubMetricsTripper struct {
	requestCounter metric.Int64Counter
	next           http.RoundTripper
}

func (rt *githubMetricsTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.requestCounter.Add(req.Context(), 1, metric.WithAttributes(
		attribute.String("method", req.Method),
		attribute.String("host", req.Host),
		attribute.String("path", req.URL.Path),
	))
	return rt.next.RoundTrip(req)
}

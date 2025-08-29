package run

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/google/go-github/v57/github"
	"github.com/ldez/grignotin/goproxy"
	"github.com/rs/zerolog/log"
	"github.com/traefik/piceus/internal/plugin"
	"github.com/traefik/piceus/pkg/core"
	"github.com/traefik/piceus/pkg/meter"
	"github.com/traefik/piceus/pkg/sources"
	"github.com/traefik/piceus/pkg/tracer"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"golang.org/x/oauth2"
)

func run(ctx context.Context, cfg Config) error {
	stopTracer, err := setupTracing(ctx, cfg.Tracing)
	if err != nil {
		return fmt.Errorf("setting up tracing provider: %w", err)
	}
	defer stopTracer()

	if cfg.EnableMetrics {
		stopMeter, err := setupMetrics(ctx, cfg.Metrics)
		if err != nil {
			return fmt.Errorf("setting up metrics provider: %w", err)
		}
		defer stopMeter()
	}

	ghClient, err := newGitHubClient(ctx, cfg.GithubToken, cfg.EnableMetrics)
	if err != nil {
		return fmt.Errorf("creating github client: %w", err)
	}
	gpClient := goproxy.NewClient("")

	pgClient := plugin.New(cfg.PluginURL)

	var srcs core.Sources
	if _, ok := os.LookupEnv(core.PrivateModeEnv); ok {
		srcs = &sources.GitHub{Client: ghClient}
	} else {
		srcs = &sources.GoProxy{Client: gpClient}
	}

	scrapper := core.NewScrapper(ghClient, gpClient, pgClient, cfg.DryRun, srcs, cfg.GithubSearchQueries, cfg.GithubSearchQueriesIssues)

	return scrapper.Run(ctx)
}

func newGitHubClient(ctx context.Context, token string, enableMetrics bool) (*github.Client, error) {
	if token == "" {
		return github.NewClient(nil), nil
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)

	client := oauth2.NewClient(ctx, ts)

	if enableMetrics {
		m := otel.Meter("piceus")
		requestCounter, err := m.Int64Counter(
			"http.requests.total",
			metric.WithDescription("Number of API calls."),
			metric.WithUnit("requests"),
		)
		if err != nil {
			return nil, fmt.Errorf("creating counter: %w", err)
		}

		client.Transport = &githubMetricsTripper{
			requestCounter: requestCounter,
			roundTripper:   otelhttp.NewTransport(client.Transport),
		}
	} else {
		client.Transport = otelhttp.NewTransport(client.Transport)
	}

	return github.NewClient(client), nil
}

func setupMetrics(ctx context.Context, cfg meter.Config) (func(), error) {
	metricProvider, err := meter.NewOTLPProvider(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("setup metrics provider: %w", err)
	}

	otel.SetMeterProvider(metricProvider)

	return func() {
		if err := metricProvider.Stop(ctx); err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("Stopping metrics provider")
		}
	}, nil
}

func setupTracing(ctx context.Context, cfg tracer.Config) (func(), error) {
	tracePropagator := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{})
	traceProvider, err := tracer.NewOTLPProvider(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("setup tracing provider: %w", err)
	}

	otel.SetTracerProvider(traceProvider)
	otel.SetTextMapPropagator(tracePropagator)

	return func() {
		if err := traceProvider.Stop(ctx); err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("Stopping trace provider")
		}
	}, nil
}

type githubMetricsTripper struct {
	requestCounter metric.Int64Counter
	roundTripper   http.RoundTripper
}

func (rt *githubMetricsTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.requestCounter.Add(req.Context(), 1, metric.WithAttributes(
		attribute.String("method", req.Method),
		attribute.String("host", req.Host),
		attribute.String("path", req.URL.Path),
	))
	return rt.roundTripper.RoundTrip(req)
}

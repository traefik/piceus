package run

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ldez/grignotin/goproxy"
	"github.com/rs/zerolog/log"
	"github.com/traefik/piceus/internal/plugin"
	"github.com/traefik/piceus/pkg/client"
	"github.com/traefik/piceus/pkg/core"
	"github.com/traefik/piceus/pkg/meter"
	"github.com/traefik/piceus/pkg/sources"
	"github.com/traefik/piceus/pkg/tracer"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func run(ctx context.Context, cfg Config) error {
	stopTracer, err := setupTracing(ctx, cfg.Tracing)
	if err != nil {
		return fmt.Errorf("setting up tracing provider: %w", err)
	}
	defer stopTracer()

	if cfg.EnableMetrics {
		stopMeter, mErr := setupMetrics(ctx, cfg.Metrics)
		if mErr != nil {
			return fmt.Errorf("setting up metrics provider: %w", mErr)
		}
		defer stopMeter()
	}

	ghClient, err := client.New(ctx,
		client.WithToken(cfg.GithubToken),
		client.WithMetrics(cfg.EnableMetrics),
		client.WithRateLimiter(30, 25, time.Now().Add(time.Minute)),
		client.WithRetry(4, 30*time.Second),
	)
	if err != nil {
		return fmt.Errorf("creating github client: %w", err)
	}
	gpClient := goproxy.NewClient("")

	pgClient := plugin.New(cfg.PluginURL)

	var srcs core.Sources
	if _, ok := os.LookupEnv(core.PrivateModeEnv); ok {
		srcs = &sources.GitHub{Client: ghClient.GithubClient()}
	} else {
		srcs = &sources.GoProxy{Client: gpClient}
	}

	scrapper := core.NewScrapper(ghClient.GithubClient(), gpClient, pgClient, cfg.DryRun, srcs, cfg.GithubSearchQueries, cfg.GithubSearchQueriesIssues)

	return scrapper.Run(ctx)
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

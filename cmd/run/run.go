package run

import (
	"context"
	"fmt"
	"os"

	"github.com/google/go-github/v57/github"
	"github.com/ldez/grignotin/goproxy"
	"github.com/traefik/piceus/internal/plugin"
	"github.com/traefik/piceus/pkg/core"
	"github.com/traefik/piceus/pkg/sources"
	"github.com/traefik/piceus/pkg/tracer"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"golang.org/x/oauth2"
)

func run(ctx context.Context, cfg Config) error {
	stopTracer, err := setupTracing(ctx, cfg.Tracing)
	if err != nil {
		return fmt.Errorf("setting up tracing provider: %w", err)
	}
	defer stopTracer()

	ghClient := newGitHubClient(ctx, cfg.GithubToken)
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

func newGitHubClient(ctx context.Context, token string) *github.Client {
	if token == "" {
		return github.NewClient(nil)
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)

	client := oauth2.NewClient(ctx, ts)
	client.Transport = otelhttp.NewTransport(client.Transport)

	return github.NewClient(client)
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
		_ = traceProvider.Stop(ctx)
	}, nil
}

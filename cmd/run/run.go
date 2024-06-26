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
	"golang.org/x/oauth2"
)

func run(ctx context.Context, cfg Config) error {
	tTracer, closer, err := tracer.NewTracer(ctx, cfg.Tracing)
	if err != nil {
		return fmt.Errorf("setup tracing provider: %w", err)
	}
	defer func() { _ = closer.Close() }()

	ghClient := newGitHubClient(ctx, cfg.GithubToken)
	gpClient := goproxy.NewClient("")

	pgClient := plugin.New(cfg.PluginURL)

	var srcs core.Sources
	if _, ok := os.LookupEnv(core.PrivateModeEnv); ok {
		srcs = &sources.GitHub{Client: ghClient}
	} else {
		srcs = &sources.GoProxy{Client: gpClient}
	}

	scrapper := core.NewScrapper(ghClient, gpClient, pgClient, srcs, tTracer, cfg.GithubSearchQueries, cfg.GithubSearchQueriesIssues)

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

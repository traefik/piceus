package run

import (
	"context"
	"os"

	"github.com/google/go-github/v45/github"
	"github.com/ldez/grignotin/goproxy"
	"github.com/rs/zerolog/log"
	"github.com/traefik/piceus/internal/plugin"
	"github.com/traefik/piceus/pkg/core"
	"github.com/traefik/piceus/pkg/sources"
	"github.com/traefik/piceus/pkg/tracer"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/oauth2"
)

func run(ctx context.Context, cfg Config) error {
	exporter, err := tracer.NewJaegerExporter(cfg.Tracing.Endpoint, cfg.Tracing.Username, cfg.Tracing.Password)
	if err != nil {
		log.Error().Err(err).Msg("Unable to configure new exporter.")
		return err
	}
	defer exporter.Flush()

	bsp := tracer.Setup(exporter, cfg.Tracing.Probability)
	defer func() { _ = bsp.Shutdown(ctx) }()

	ghClient := newGitHubClient(ctx, cfg.GithubToken)
	gpClient := goproxy.NewClient("")

	pgClient := plugin.New(cfg.PluginURL, cfg.ServicesAccessToken)

	var srcs core.Sources
	if _, ok := os.LookupEnv(core.PrivateModeEnv); ok {
		srcs = &sources.GitHub{Client: ghClient}
	} else {
		srcs = &sources.GoProxy{Client: gpClient}
	}

	scrapper := core.NewScrapper(ghClient, gpClient, pgClient, srcs)

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

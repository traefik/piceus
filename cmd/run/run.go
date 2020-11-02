package run

import (
	"context"
	"os"

	"github.com/google/go-github/v32/github"
	"github.com/ldez/grignotin/goproxy"
	"github.com/rs/zerolog/log"
	"github.com/traefik/piceus/internal/plugin"
	"github.com/traefik/piceus/pkg/core"
	"github.com/traefik/piceus/pkg/sources"
	"github.com/traefik/piceus/pkg/tracer"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/oauth2"
)

// Run executes Piceus scrapper.
func Run(c *cli.Context) error {
	ctx := context.Background()

	exporter, err := tracer.NewJaegerExporter(c.String("tracing-endpoint"), c.String("tracing-username"), c.String("tracing-password"))
	if err != nil {
		log.Error().Err(err).Msg("Unable to configure new exporter.")
		return err
	}
	defer exporter.Flush()

	bsp := tracer.Setup(exporter, c.Float64("tracing-probability"))
	defer bsp.Shutdown()

	ghClient := newGitHubClient(ctx, c.String("github-token"))
	gpClient := goproxy.NewClient("")

	pgClient := plugin.New(c.String("plugin-url"), c.String("services-access-token"))

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
	if len(token) == 0 {
		return github.NewClient(nil)
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)

	client := oauth2.NewClient(ctx, ts)
	client.Transport = otelhttp.NewTransport(client.Transport)

	return github.NewClient(client)
}

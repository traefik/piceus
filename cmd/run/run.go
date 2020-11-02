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
	"github.com/urfave/cli/v2"
	"golang.org/x/oauth2"
)

// Run executes Piceus scrapper.
func Run(ctx *cli.Context) error {
	c := context.Background()

	ghClient := newGitHubClient(c, ctx.String("github-token"))
	gpClient := goproxy.NewClient("")

	pgClient := plugin.New(ctx.String("plugin-url"), ctx.String("services-access-token"))

	var srcs core.Sources
	if _, ok := os.LookupEnv(core.PrivateModeEnv); ok {
		srcs = &sources.GitHub{Client: ghClient}
	} else {
		srcs = &sources.GoProxy{Client: gpClient}
	}

	scrapper := core.NewScrapper(ghClient, gpClient, pgClient, srcs)

	err := scrapper.Run(c)
	if err != nil {
		log.Fatal().Err(err).Msg("unable to run scrapper")
	}

	return nil
}

func newGitHubClient(ctx context.Context, token string) *github.Client {
	if len(token) == 0 {
		return github.NewClient(nil)
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	return github.NewClient(oauth2.NewClient(ctx, ts))
}

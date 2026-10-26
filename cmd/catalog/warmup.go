package catalog

import (
	"github.com/rs/zerolog/log"
	"github.com/traefik/piceus/pkg/audit"
	"github.com/urfave/cli/v2"
)

const flagTop = "top"

func warmupCommand() *cli.Command {
	flags := append(commonFlags(),
		&cli.IntFlag{
			Name:  flagTop,
			Usage: "Number of plugins to warm up, by stars",
			Value: 200,
		},
		stringFlag(flagGoProxyURL, "Go module proxy, to find the plugins with dependencies", "https://proxy.golang.org", false),
	)

	return &cli.Command{
		Name:        "warmup",
		Usage:       "Download the latest version of the top plugins so the CDN caches them",
		Description: "Plugins with dependencies go first: they are served from GitHub zipballs, the slowest path.",
		Flags:       flags,
		Action: func(cliCtx *cli.Context) error {
			store, client, closeStore, err := setup(cliCtx)
			if err != nil {
				return err
			}

			defer closeStore()

			ctx := cliCtx.Context

			entries, err := store.Entries(ctx)
			if err != nil {
				return err
			}

			checker := audit.NewTagChecker(cliCtx.String(flagGoProxyURL), nil)
			selected := audit.SelectWarmup(ctx, checker, entries, cliCtx.Int(flagTop), cliCtx.Int(flagConcurrency))

			log.Info().Msgf("Warming up %d plugins on %s", len(selected), client.BaseURL)

			results := audit.Warmup(ctx, client, selected, cliCtx.Int(flagConcurrency))

			if err = writeJSON(cliCtx.String(flagOut), "warmup.json", results); err != nil {
				return err
			}

			log.Info().Interface("verdicts", summary(results)).Msg("Warm-up done")

			return nil
		},
	}
}

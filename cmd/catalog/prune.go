package catalog

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/rs/zerolog/log"
	"github.com/traefik/piceus/pkg/audit"
	"github.com/urfave/cli/v2"
)

func pruneCommand() *cli.Command {
	flags := append(commonFlags(),
		dryRunFlag(),
		stringFlag(flagInput, "audit.json listing the versions that are not downloadable", "audit.json", false),
		stringFlag(flagGitHubToken, "GitHub Token", "", true),
		stringFlag(flagGoProxyURL, "Go module proxy", "https://proxy.golang.org", false),
	)

	return &cli.Command{
		Name:  "prune",
		Usage: "Remove the versions of an audit that are not downloadable",
		Description: "A version is only removed when it is still not served and is gone upstream: " +
			"no go.mod on the Go module proxy, or no tag on GitHub when it is served from there. " +
			"A plugin left without versions is hidden.",
		Flags: flags,
		Action: func(cliCtx *cli.Context) error {
			store, client, closeStore, err := setup(cliCtx)
			if err != nil {
				return err
			}

			defer closeStore()

			results, err := readResults(cliCtx.String(flagInput))
			if err != nil {
				return err
			}

			ctx := cliCtx.Context

			checker := newTagChecker(cliCtx.String(flagGoProxyURL), cliCtx.String(flagGitHubToken))

			gone, kept, failed := confirmGone(ctx, client, checker, results)

			dryRun := cliCtx.Bool(flagDryRun)

			var removed, hidden int

			for _, module := range slices.Sorted(maps.Keys(gone)) {
				logger := log.With().Str("module", module).Strs("versions", slices.Sorted(maps.Keys(gone[module]))).Logger()

				removal, errR := store.RemoveVersions(ctx, module, gone[module], dryRun)
				if errR != nil {
					logger.Error().Err(errR).Msg("Prune failed")
					failed += len(gone[module])

					continue
				}

				removed += len(gone[module])

				if len(removal.Versions) == 0 {
					hidden++
				}

				logger.Info().Str("latestVersion", removal.LatestVersion).Bool("hidden", removal.Hidden).Msg("Pruned")
			}

			log.Info().Bool("dryRun", dryRun).Msgf("%d versions pruned, %d plugins hidden, %d kept, %d failed", removed, hidden, kept, failed)

			if failed > 0 {
				return fmt.Errorf("%d entries failed", failed)
			}

			return nil
		},
	}
}

// confirmGone returns, as module → version → recorded hash, the not downloadable versions of results
// that are still not served and are gone upstream.
func confirmGone(ctx context.Context, client *audit.Client, checker *audit.TagChecker, results []audit.Result) (map[string]map[string]string, int, int) {
	gone := map[string]map[string]string{}

	var kept, failed int

	for _, r := range results {
		if r.Verdict != audit.VerdictDownloadError {
			continue
		}

		logger := log.With().Str("entry", r.Key()).Logger()

		dl, err := client.Download(ctx, r.Module, r.Version)
		if err == nil {
			logger.Info().Msg("Kept: downloadable again")
			kept++

			continue
		}

		if dl.Status == 0 {
			logger.Error().Err(err).Msg("Download failed")
			failed++

			continue
		}

		isGone, err := checker.Gone(ctx, r.Entry)
		if err != nil {
			logger.Error().Err(err).Msg("Upstream check failed")
			failed++

			continue
		}

		if !isGone {
			logger.Info().Int("status", dl.Status).Msg("Kept: still available upstream")
			kept++

			continue
		}

		if gone[r.Module] == nil {
			gone[r.Module] = map[string]string{}
		}

		gone[r.Module][r.Version] = r.Hash
	}

	return gone, kept, failed
}

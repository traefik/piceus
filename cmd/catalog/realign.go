package catalog

import (
	"fmt"
	"slices"

	"github.com/ettle/strcase"
	"github.com/rs/zerolog/log"
	"github.com/traefik/piceus/pkg/audit"
	"github.com/urfave/cli/v2"
)

const (
	flagInput       = "input"
	flagOnly        = "only"
	flagIncludeWasm = "include-wasm"
)

func realignCommand() *cli.Command {
	flags := append(commonFlags(),
		dryRunFlag(),
		stringFlag(flagInput, "audit.json listing the mismatches to realign", "audit.json", false),
		&cli.StringSliceFlag{
			Name:  flagOnly,
			Usage: "Only realign these <module>@<version>. Required for a version whose tag moved or could not be checked.",
		},
		&cli.BoolFlag{
			Name:    flagIncludeWasm,
			Usage:   "Also realign wasm plugins (their hash is a release asset digest, review it first)",
			EnvVars: []string{strcase.ToSNAKE(flagIncludeWasm)},
		},
	)

	return &cli.Command{
		Name:  "realign",
		Usage: "Record the hash of the archive served today for the mismatches of an audit",
		Description: "Each archive is downloaded again and must match the audit, " +
			"and the recorded hash is only replaced if it still holds the value the audit saw.",
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
			dryRun := cliCtx.Bool(flagDryRun)
			only := cliCtx.StringSlice(flagOnly)

			var realigned, skipped, failed int

			for _, r := range results {
				key := r.Key()
				logger := log.With().Str("entry", key).Logger()

				if len(only) > 0 && !slices.Contains(only, key) {
					continue
				}

				if reason := skipReason(r, only, cliCtx.Bool(flagIncludeWasm)); reason != "" {
					logger.Info().Msgf("Skipped: %s", reason)
					skipped++

					continue
				}

				dl, errD := client.Download(ctx, r.Module, r.Version)
				if errD != nil {
					logger.Error().Err(errD).Msg("Download failed")
					failed++

					continue
				}

				if dl.Hash != r.ServedHash {
					logger.Error().Str("audit", r.ServedHash).Str("now", dl.Hash).Msg("The served archive changed since the audit, re-run it")
					failed++

					continue
				}

				logger = logger.With().Str("old", r.Hash).Str("new", dl.Hash).Logger()

				if dryRun {
					logger.Info().Msg("Would realign")
					realigned++

					continue
				}

				if err = store.SetHash(ctx, r.Module, r.Version, r.Hash, dl.Hash); err != nil {
					logger.Error().Err(err).Msg("Realign failed")
					failed++

					continue
				}

				logger.Info().Msg("Realigned")
				realigned++
			}

			log.Info().Bool("dryRun", dryRun).Msgf("%d realigned, %d skipped, %d failed", realigned, skipped, failed)

			if failed > 0 {
				return fmt.Errorf("%d entries failed", failed)
			}

			return nil
		},
	}
}

// skipReason explains why an audited entry must not be realigned, empty if it can be.
func skipReason(r audit.Result, only []string, includeWasm bool) string {
	switch {
	case r.Verdict != audit.VerdictMismatch:
		return "verdict " + r.Verdict
	case r.IsWasm() && !includeWasm:
		return "wasm plugin, see --" + flagIncludeWasm
	case slices.Contains(only, r.Key()):
		return ""
	// Realigning would accept code that changed under a published version.
	case r.Tag != nil && r.Tag.Moved:
		return "tag moved after publication, review the diff then pass --" + flagOnly
	case r.Tag != nil && r.Tag.Error != "":
		return "tag check failed (" + r.Tag.Error + "), audit again or pass --" + flagOnly
	case r.Tag == nil && !r.IsWasm() && audit.IsGitHubModule(r.Module):
		return "tag not checked, audit with --" + flagCheckTags + " or pass --" + flagOnly
	default:
		return ""
	}
}

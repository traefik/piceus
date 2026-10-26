package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ettle/strcase"
	"github.com/rs/zerolog/log"
	"github.com/traefik/piceus/pkg/audit"
	"github.com/urfave/cli/v2"
)

const (
	flagModule    = "module"
	flagLimit     = "limit"
	flagCheckTags = "check-tags"
	flagCompare   = "compare"
)

func auditCommand() *cli.Command {
	flags := append(commonFlags(),
		stringFlag(flagModule, "Only audit this module", "", false),
		&cli.IntFlag{
			Name:  flagLimit,
			Usage: "Only audit the first N entries (0 = all)",
		},
		&cli.BoolFlag{
			Name:    flagCheckTags,
			Usage:   "Report versions whose tag moved after publication (one GitHub call per plugin version with dependencies)",
			EnvVars: []string{strcase.ToSNAKE(flagCheckTags)},
		},
		stringFlag(flagGitHubToken, "GitHub Token, for --check-tags", "", false),
		stringFlag(flagGoProxyURL, "Go module proxy pinning the published commits", "https://proxy.golang.org", false),
		stringFlag(flagCompare, "Previous audit.json to diff against", "", false),
	)

	return &cli.Command{
		Name:        "audit",
		Usage:       "Audit the recorded plugin hashes",
		Description: "For every recorded hash: download the archive, sha256 it, then call validate with that hash. Read-only.",
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

			entries = filterModule(entries, cliCtx.String(flagModule), cliCtx.Int(flagLimit))

			// Read before writing audit.json, which may be the same file.
			var previous []audit.Result

			if path := cliCtx.String(flagCompare); path != "" {
				if previous, err = readResults(path); err != nil {
					return err
				}
			}

			log.Info().Msgf("Auditing %d entries against %s", len(entries), client.BaseURL)

			results := audit.Run(ctx, client, entries, cliCtx.Int(flagConcurrency))

			if cliCtx.Bool(flagCheckTags) {
				checker := newTagChecker(cliCtx.String(flagGoProxyURL), cliCtx.String(flagGitHubToken))
				audit.CheckTags(ctx, checker, results, cliCtx.Int(flagConcurrency))
			}

			if err = ctx.Err(); err != nil {
				return err
			}

			out := cliCtx.String(flagOut)

			if err = writeAudit(out, client.BaseURL, "audit", results); err != nil {
				return err
			}

			log.Info().Interface("verdicts", summary(results)).Msgf("Wrote %s", filepath.Join(out, "audit.md"))

			if path := cliCtx.String(flagCompare); path != "" {
				return writeCompare(out, path, previous, results)
			}

			return nil
		},
	}
}

func writeCompare(out, previousPath string, previous, results []audit.Result) error {
	d := audit.Compare(previous, results)

	if err := writeJSON(out, "diff.json", d); err != nil {
		return err
	}

	var md strings.Builder

	audit.WriteDiff(&md, d)

	if err := os.WriteFile(filepath.Join(out, "diff.md"), []byte(md.String()), 0o600); err != nil {
		return err
	}

	log.Info().Msgf("Wrote %s: %d regressions, %d with changed bytes, %d fixed",
		filepath.Join(out, "diff.md"), len(d.Regressions), len(d.BytesChanged), len(d.Fixed))

	if len(d.Regressions) > 0 || len(d.BytesChanged) > 0 {
		return fmt.Errorf("%d regressions and %d changed archives since %s", len(d.Regressions), len(d.BytesChanged), previousPath)
	}

	return nil
}

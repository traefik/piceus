package catalog

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/traefik/piceus/pkg/audit"
	"github.com/traefik/piceus/pkg/catalog"
	"github.com/urfave/cli/v2"
)

const (
	flagSourceMongoURI      = "source-mongo-uri"
	flagSourceMongoDatabase = "source-mongo-database"
	flagSince               = "since"
)

func deltaSyncCommand() *cli.Command {
	flags := append(commonFlags(),
		dryRunFlag(),
		stringFlag(flagSourceMongoURI, "MongoDB URI of the catalog to copy from", "", true),
		stringFlag(flagSourceMongoDatabase, "MongoDB database of the catalog to copy from", "plugin", false),
		&cli.TimestampFlag{
			Name:     flagSince,
			Usage:    "Time of the dump the target was restored from (RFC 3339)",
			Layout:   time.RFC3339,
			Required: true,
		},
	)

	return &cli.Command{
		Name:  "delta-sync",
		Usage: "Copy into the catalog what was added to the source catalog after a dump",
		Description: "Copies the plugins created after --since, new versions and missing hashes, then audits the copied hashes. " +
			"Nothing is removed from the target and no recorded hash is changed.",
		Flags: flags,
		Action: func(cliCtx *cli.Context) error {
			store, client, closeStore, err := setup(cliCtx)
			if err != nil {
				return err
			}

			defer closeStore()

			source, err := catalog.Open(cliCtx.String(flagSourceMongoURI), cliCtx.String(flagSourceMongoDatabase))
			if err != nil {
				return err
			}

			defer func() { _ = source.Close(context.Background()) }()

			ctx := cliCtx.Context
			dryRun := cliCtx.Bool(flagDryRun)
			out := cliCtx.String(flagOut)

			result, err := catalog.Sync(ctx, source, store, *cliCtx.Timestamp(flagSince), dryRun)
			if err != nil {
				return err
			}

			if err = writeJSON(out, "delta-sync.json", result); err != nil {
				return err
			}

			log.Info().Bool("dryRun", dryRun).
				Int("inserted", len(result.Inserted)).
				Int("updated", len(result.Updated)).
				Int("hashes", len(result.Hashes)).
				Int("conflicts", len(result.Conflicts)).
				Int("skipped", len(result.Skipped)).
				Int("changed", len(result.Changed)).
				Int("missingInSource", len(result.MissingInSource)).
				Msg("Delta sync done")

			// Downloading a version without a recorded hash records one, so only audit what is really copied.
			if dryRun || len(result.Hashes) == 0 {
				return nil
			}

			results := audit.Run(ctx, client, result.Hashes, cliCtx.Int(flagConcurrency))

			if err = ctx.Err(); err != nil {
				return err
			}

			if err = writeAudit(out, client.BaseURL, "delta-sync-audit", results); err != nil {
				return err
			}

			log.Info().Interface("verdicts", summary(results)).Msg("Audited the copied hashes")

			return nil
		},
	}
}

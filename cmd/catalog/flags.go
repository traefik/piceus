// Package catalog provides the commands that audit and repair the plugin catalog.
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ettle/strcase"
	"github.com/google/go-github/v57/github"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog/log"
	"github.com/traefik/piceus/pkg/audit"
	"github.com/traefik/piceus/pkg/catalog"
	"github.com/traefik/piceus/pkg/logger"
	"github.com/urfave/cli/v2"
)

const (
	flagLogLevel          = "log-level"
	flagMongoURI          = "mongo-uri"
	flagMongoDatabase     = "mongo-database"
	flagPluginBaseURL     = "plugin-base-url"
	flagBasicAuthUser     = "basic-auth-user"
	flagBasicAuthPassword = "basic-auth-password" //nolint:gosec // Flag name, not a credential.
	flagConcurrency       = "concurrency"
	flagOut               = "out"
	flagGoProxyURL        = "go-proxy-url"
	flagGitHubToken       = "github-token"
	flagDryRun            = "dry-run"
)

// Commands returns the catalog commands.
func Commands() []*cli.Command {
	return []*cli.Command{auditCommand(), realignCommand(), pruneCommand(), deltaSyncCommand(), warmupCommand()}
}

func stringFlag(name, usage, value string, required bool) *cli.StringFlag {
	return &cli.StringFlag{
		Name:     name,
		Usage:    usage,
		EnvVars:  []string{strcase.ToSNAKE(name)},
		Value:    value,
		Required: required,
	}
}

func commonFlags() []cli.Flag {
	return []cli.Flag{
		stringFlag(flagLogLevel, "Log level", "info", false),
		stringFlag(flagMongoURI, "MongoDB URI of the plugin catalog", "", true),
		stringFlag(flagMongoDatabase, "MongoDB database of the plugin catalog", "plugin", false),
		stringFlag(flagPluginBaseURL, "Public URL of the plugin service serving this catalog", "https://plugins.traefik.io", false),
		stringFlag(flagBasicAuthUser, "Basic auth user for the plugin service", "", false),
		stringFlag(flagBasicAuthPassword, "Basic auth password for the plugin service", "", false),
		stringFlag(flagOut, "Output directory", ".", false),
		&cli.IntFlag{
			Name:    flagConcurrency,
			Usage:   "Parallel downloads",
			EnvVars: []string{strcase.ToSNAKE(flagConcurrency)},
			Value:   6,
		},
	}
}

func dryRunFlag() cli.Flag {
	return &cli.BoolFlag{
		Name:    flagDryRun,
		Usage:   "Dry run mode.",
		EnvVars: []string{strcase.ToSNAKE(flagDryRun)},
		Value:   true,
	}
}

func setup(cliCtx *cli.Context) (*catalog.Store, *audit.Client, func(), error) {
	logger.Setup(cliCtx.String(flagLogLevel))

	// Fail before hours of work rather than when writing the results.
	if err := os.MkdirAll(cliCtx.String(flagOut), 0o750); err != nil {
		return nil, nil, nil, fmt.Errorf("output directory: %w", err)
	}

	store, err := catalog.Open(cliCtx.String(flagMongoURI), cliCtx.String(flagMongoDatabase))
	if err != nil {
		return nil, nil, nil, err
	}

	closeStore := func() {
		if err := store.Close(context.Background()); err != nil {
			log.Error().Err(err).Msg("Closing MongoDB connection")
		}
	}

	c := audit.NewClient(cliCtx.String(flagPluginBaseURL), cliCtx.String(flagBasicAuthUser), cliCtx.String(flagBasicAuthPassword))

	return store, c, closeStore, nil
}

func newTagChecker(goProxyURL, token string) *audit.TagChecker {
	// Unlike pkg/client, follows redirects: a renamed repository answers 301.
	retry := retryablehttp.NewClient()
	retry.RetryMax = 4
	retry.RetryWaitMin = 30 * time.Second
	retry.Logger = nil

	gh := github.NewClient(retry.StandardClient())
	if token != "" {
		gh = gh.WithAuthToken(token)
	}

	return audit.NewTagChecker(goProxyURL, gh)
}

func filterModule(entries []catalog.Entry, module string, limit int) []catalog.Entry {
	if module != "" {
		var kept []catalog.Entry

		for _, e := range entries {
			if e.Module == module {
				kept = append(kept, e)
			}
		}

		entries = kept
	}

	if limit > 0 && limit < len(entries) {
		entries = entries[:limit]
	}

	return entries
}

func writeJSON(dir, name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, name), data, 0o600)
}

func readResults(path string) ([]audit.Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var results []audit.Result
	if err = json.Unmarshal(data, &results); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}

	return results, nil
}

func writeAudit(dir, baseURL, prefix string, results []audit.Result) error {
	if err := writeJSON(dir, prefix+".json", results); err != nil {
		return err
	}

	var md strings.Builder

	audit.WriteReport(&md, baseURL, results)

	return os.WriteFile(filepath.Join(dir, prefix+".md"), []byte(md.String()), 0o600)
}

func summary(results []audit.Result) map[string]int {
	counts := map[string]int{}
	for _, r := range results {
		counts[r.Verdict]++
	}

	return counts
}

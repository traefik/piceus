package run

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/traefik/piceus/pkg/meter"
	"github.com/traefik/piceus/pkg/tracer"
	"github.com/urfave/cli/v2"
)

// Config represents the configuration for the run command.
type Config struct {
	GithubToken string
	PluginURL   string

	DryRun bool

	GithubSearchQueries       []string
	GithubSearchQueriesIssues []string

	EvaluationTimeout time.Duration

	EnableMetrics bool
	Metrics       meter.Config
	Tracing       tracer.Config
}

func buildConfig(cliCtx *cli.Context) (Config, error) {
	token, err := readGitHubToken(cliCtx)
	if err != nil {
		return Config{}, err
	}

	return Config{
		GithubToken:               token,
		PluginURL:                 cliCtx.String(flagPluginURL),
		DryRun:                    cliCtx.Bool(flagDryRun),
		GithubSearchQueries:       cliCtx.StringSlice(flagGithubSearchQueries),
		GithubSearchQueriesIssues: cliCtx.StringSlice(flagGithubSearchQueriesIssues),
		EvaluationTimeout:         cliCtx.Duration(flagEvaluationTimeout),
		EnableMetrics:             cliCtx.Bool(flagEnableMetrics),
		Metrics: meter.Config{
			Address:     cliCtx.String(flagMetricsAddress),
			Insecure:    cliCtx.Bool(flagMetricsInsecure),
			Username:    cliCtx.String(flagMetricsUsername),
			Password:    cliCtx.String(flagMetricsPassword),
			ServiceName: "piceus",
		},
		Tracing: tracer.Config{
			Address:     cliCtx.String(flagTracingAddress),
			Insecure:    cliCtx.Bool(flagTracingInsecure),
			Username:    cliCtx.String(flagTracingUsername),
			Password:    cliCtx.String(flagTracingPassword),
			Probability: cliCtx.Float64(flagTracingProbability),
			ServiceName: "piceus",
		},
	}, nil
}

// readGitHubToken reads the GitHub token from the token file, or from the deprecated token flag.
func readGitHubToken(cliCtx *cli.Context) (string, error) {
	if path := cliCtx.String(flagGitHubTokenFile); path != "" {
		content, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("reading GitHub token file: %w", err)
		}

		token := strings.TrimSpace(string(content))
		if token == "" {
			return "", fmt.Errorf("empty GitHub token file: %s", path)
		}

		return token, nil
	}

	if token := cliCtx.String(flagGitHubToken); token != "" {
		log.Warn().Msgf("--%s is deprecated: use --%s", flagGitHubToken, flagGitHubTokenFile)

		return token, nil
	}

	return "", fmt.Errorf("missing GitHub token: set --%s", flagGitHubTokenFile)
}

package run

import (
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

	EnableMetrics bool
	Metrics       meter.Config
	Tracing       tracer.Config
}

func buildConfig(cliCtx *cli.Context) Config {
	return Config{
		GithubToken:               cliCtx.String(flagGitHubToken),
		PluginURL:                 cliCtx.String(flagPluginURL),
		DryRun:                    cliCtx.Bool(flagDryRun),
		GithubSearchQueries:       cliCtx.StringSlice(flagGithubSearchQueries),
		GithubSearchQueriesIssues: cliCtx.StringSlice(flagGithubSearchQueriesIssues),
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
	}
}

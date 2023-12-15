package run

import (
	"github.com/traefik/piceus/pkg/tracer"
	"github.com/urfave/cli/v2"
)

// Config represents the configuration for the run command.
type Config struct {
	GithubToken string
	PluginURL   string
	Tracing     tracer.Config
}

func buildConfig(cliCtx *cli.Context) Config {
	return Config{
		GithubToken: cliCtx.String(flagGitHubToken),
		PluginURL:   cliCtx.String(flagPluginURL),
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

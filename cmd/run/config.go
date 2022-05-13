package run

import "github.com/urfave/cli/v2"

// Tracing holds the tracing configuration.
type Tracing struct {
	Endpoint    string
	Username    string
	Password    string
	Probability float64
}

// Config represents the configuration for the run command.
type Config struct {
	GithubToken         string
	ServicesAccessToken string
	PluginURL           string
	Tracing             Tracing
}

func buildConfig(cliCtx *cli.Context) Config {
	return Config{
		GithubToken:         cliCtx.String(flagGitHubToken),
		ServicesAccessToken: cliCtx.String(flagServicesAccessToken),
		PluginURL:           cliCtx.String(flagPluginURL),
		Tracing: Tracing{
			Endpoint:    cliCtx.String(flagTracingEndpoint),
			Username:    cliCtx.String(flagTracingUsername),
			Password:    cliCtx.String(flagTracingPassword),
			Probability: cliCtx.Float64(flagTracingProbability),
		},
	}
}

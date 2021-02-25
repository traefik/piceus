package run

import "github.com/urfave/cli/v2"

// Pilot holds the pilot configuration.
type Pilot struct {
	GithubToken         string
	ServicesAccessToken string
	PluginURL           string
}

// Tracing holds the tracing configuration.
type Tracing struct {
	Endpoint    string
	Username    string
	Password    string
	Probability float64
}

// Config represents the configuration for the run command.
type Config struct {
	Pilot   Pilot
	Tracing Tracing
}

func buildConfig(cliCtx *cli.Context) Config {
	return Config{
		Pilot: Pilot{
			GithubToken:         cliCtx.String("github-token"),
			ServicesAccessToken: cliCtx.String("services-access-token"),
			PluginURL:           cliCtx.String("plugin-url"),
		},
		Tracing: Tracing{
			Endpoint:    cliCtx.String("tracing-endpoint"),
			Username:    cliCtx.String("tracing-username"),
			Password:    cliCtx.String("tracing-password"),
			Probability: cliCtx.Float64("tracing-probability"),
		},
	}
}

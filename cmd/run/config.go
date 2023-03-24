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
	GithubToken string
	PluginURL   string
	S3Bucket    string
	S3Key       string
	Tracing     Tracing
}

func buildConfig(cliCtx *cli.Context) Config {
	return Config{
		GithubToken: cliCtx.String(flagGitHubToken),
		PluginURL:   cliCtx.String(flagPluginURL),
		S3Bucket:    cliCtx.String(flagS3Bucket),
		S3Key:       cliCtx.String(flagS3Key),
		Tracing: Tracing{
			Endpoint:    cliCtx.String(flagTracingEndpoint),
			Username:    cliCtx.String(flagTracingUsername),
			Password:    cliCtx.String(flagTracingPassword),
			Probability: cliCtx.Float64(flagTracingProbability),
		},
	}
}

package run

import (
	"github.com/ettle/strcase"
	"github.com/traefik/piceus/pkg/logger"
	"github.com/urfave/cli/v2"
)

const (
	flagLogLevel    = "log-level"
	flagGitHubToken = "github-token"
	flagPluginURL   = "plugin-url"
	flagS3Bucket    = "s3-bucket"
	flagS3Key       = "s3-key"
)

const (
	flagTracingEndpoint    = "tracing-endpoint"
	flagTracingUsername    = "tracing-username"
	flagTracingPassword    = "tracing-password"
	flagTracingProbability = "tracing-probability"
)

// Command creates the run command.
func Command() *cli.Command {
	cmd := &cli.Command{
		Name:        "run",
		Usage:       "Run Piceus",
		Description: "Launch application piceus",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    flagLogLevel,
				Usage:   "Log level",
				EnvVars: []string{strcase.ToSNAKE(flagLogLevel)},
				Value:   "info",
			},
			&cli.StringFlag{
				Name:     flagGitHubToken,
				Usage:    "GitHub Token.",
				EnvVars:  []string{strcase.ToSNAKE(flagGitHubToken)},
				Required: true,
			},
			&cli.StringFlag{
				Name:     flagPluginURL,
				Usage:    "Plugin Service URL",
				EnvVars:  []string{strcase.ToSNAKE(flagPluginURL)},
				Required: true,
			},
		},
		Action: func(cliCtx *cli.Context) error {
			logger.Setup(cliCtx.String(flagLogLevel))

			cfg := buildConfig(cliCtx)

			return run(cliCtx.Context, cfg)
		},
	}

	cmd.Flags = append(cmd.Flags, s3Flags()...)
	cmd.Flags = append(cmd.Flags, tracingFlags()...)

	return cmd
}

func s3Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     flagS3Bucket,
			Usage:    "Bucket to use for storing data",
			EnvVars:  []string{strcase.ToSNAKE(flagS3Bucket)},
			Required: true,
		},
		&cli.StringFlag{
			Name:     flagS3Key,
			Usage:    "Key of file within the S3 Bucket",
			EnvVars:  []string{strcase.ToSNAKE(flagS3Key)},
			Value:    "plugins.json",
			Required: true,
		},
	}
}

func tracingFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     flagTracingEndpoint,
			Usage:    "Endpoint to send traces",
			EnvVars:  []string{strcase.ToSNAKE(flagTracingEndpoint)},
			Value:    "https://collector.infra.traefiklabs.tech",
			Required: false,
		},
		&cli.StringFlag{
			Name:     flagTracingUsername,
			Usage:    "Username to connect to Jaeger",
			EnvVars:  []string{strcase.ToSNAKE(flagTracingUsername)},
			Value:    "jaeger",
			Required: false,
		},
		&cli.StringFlag{
			Name:     flagTracingPassword,
			Usage:    "Password to connect to Jaeger",
			EnvVars:  []string{strcase.ToSNAKE(flagTracingPassword)},
			Value:    "jaeger",
			Required: false,
		},
		&cli.Float64Flag{
			Name:     flagTracingProbability,
			Usage:    "Probability to send traces.",
			EnvVars:  []string{strcase.ToSNAKE(flagTracingProbability)},
			Value:    0,
			Required: false,
		},
	}
}

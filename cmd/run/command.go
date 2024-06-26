package run

import (
	"github.com/ettle/strcase"
	"github.com/traefik/piceus/pkg/logger"
	"github.com/urfave/cli/v2"
)

const (
	flagLogLevel                  = "log-level"
	flagGitHubToken               = "github-token"
	flagPluginURL                 = "plugin-url"
	flagGithubSearchQueries       = "github-search-queries"
	flagGithubSearchQueriesIssues = "github-search-queries-issues"
)

const (
	flagTracingAddress     = "tracing-address"
	flagTracingInsecure    = "tracing-insecure"
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
			// flagGithubSearchQueries queries used to search plugins on GitHub.
			// https://help.github.com/en/github/searching-for-information-on-github/searching-for-repositories
			&cli.StringSliceFlag{
				Name:    flagGithubSearchQueries,
				Usage:   "Github search queries",
				EnvVars: []string{strcase.ToSNAKE(flagGithubSearchQueries)},
				Value:   cli.NewStringSlice("topic:traefik-plugin language:Go archived:false is:public"),
			},
			// flagGithubSearchQueryIssues queries used to search issues opened by the bot account.
			// https://help.github.com/en/github/searching-for-information-on-github/searching-for-repositories
			&cli.StringSliceFlag{
				Name:    flagGithubSearchQueriesIssues,
				Usage:   "Github queries used to search issues opened by the bot account",
				EnvVars: []string{strcase.ToSNAKE(flagGithubSearchQueriesIssues)},
				Value:   cli.NewStringSlice("is:open is:issue is:public author:traefiker"),
			},
		},
		Action: func(cliCtx *cli.Context) error {
			logger.Setup(cliCtx.String(flagLogLevel))

			cfg := buildConfig(cliCtx)

			return run(cliCtx.Context, cfg)
		},
	}

	cmd.Flags = append(cmd.Flags, tracingFlags()...)

	return cmd
}

func tracingFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    flagTracingAddress,
			Usage:   "Address to send traces",
			EnvVars: []string{strcase.ToSNAKE(flagTracingAddress)},
			Value:   "jaeger.jaeger.svc.cluster.local:4318",
		},
		&cli.BoolFlag{
			Name:    flagTracingInsecure,
			Usage:   "use HTTP instead of HTTPS",
			EnvVars: []string{strcase.ToSNAKE(flagTracingInsecure)},
			Value:   true,
		},
		&cli.StringFlag{
			Name:    flagTracingUsername,
			Usage:   "Username to connect to Jaeger",
			EnvVars: []string{strcase.ToSNAKE(flagTracingUsername)},
			Value:   "jaeger",
		},
		&cli.StringFlag{
			Name:    flagTracingPassword,
			Usage:   "Password to connect to Jaeger",
			EnvVars: []string{strcase.ToSNAKE(flagTracingPassword)},
			Value:   "jaeger",
		},
		&cli.Float64Flag{
			Name:    flagTracingProbability,
			Usage:   "Probability to send traces",
			EnvVars: []string{strcase.ToSNAKE(flagTracingProbability)},
			Value:   0,
		},
	}
}

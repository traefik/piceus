package main

import (
	"os"

	"github.com/rs/zerolog/log"
	"github.com/traefik/piceus/cmd/run"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "Piceus CLI",
		Usage: "Run piceus",
		Commands: []*cli.Command{
			runCommand(),
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal().Err(err).Msg("Error while executing command")
	}
}

func runCommand() *cli.Command {
	cmd := &cli.Command{
		Name:        "run",
		Usage:       "Run Piceus",
		Description: "Launch application piceus",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "github-token",
				Usage:    "GitHub Token.",
				EnvVars:  []string{"GITHUB_TOKEN"},
				Required: true,
			},
			&cli.StringFlag{
				Name:     "services-access-token",
				Usage:    "Pilot Services Access Token",
				EnvVars:  []string{"PILOT_SERVICES_ACCESS_TOKEN"},
				Required: true,
			},
			&cli.StringFlag{
				Name:     "plugin-url",
				Usage:    "Plugin Service URL",
				EnvVars:  []string{"PILOT_PLUGIN_URL"},
				Required: true,
			},
			&cli.StringFlag{
				Name:     "tracing-endpoint",
				Usage:    "Endpoint to send traces",
				EnvVars:  []string{"TRACING_ENDPOINT"},
				Value:    "https://collector.infra.traefiklabs.tech",
				Required: false,
			},
			&cli.StringFlag{
				Name:     "tracing-username",
				Usage:    "Username to connect to Jaeger",
				EnvVars:  []string{"TRACING_USERNAME"},
				Value:    "jaeger",
				Required: false,
			},
			&cli.StringFlag{
				Name:     "tracing-password",
				Usage:    "Password to connect to Jaeger",
				EnvVars:  []string{"TRACING_PASSWORD"},
				Value:    "jaeger",
				Required: false,
			},
			&cli.Float64Flag{
				Name:     "tracing-probability",
				Usage:    "Probability to send traces.",
				EnvVars:  []string{"TRACING_PROBABILITY"},
				Value:    0,
				Required: false,
			},
		},
		Action: run.Run,
	}

	return cmd
}

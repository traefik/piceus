package main

import (
	"os"

	"github.com/rs/zerolog/log"
	"github.com/traefik/piceus/cmd/catalog"
	"github.com/traefik/piceus/cmd/evaluate"
	"github.com/traefik/piceus/cmd/run"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:     "Piceus CLI",
		Usage:    "Run piceus",
		Commands: append([]*cli.Command{run.Command(), evaluate.Command()}, catalog.Commands()...),
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal().Err(err).Msg("Error while executing command")
	}
}

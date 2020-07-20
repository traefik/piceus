package main

import (
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// SetupLogger is configuring the logger.
func setupLogger() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.With().Caller().Logger()

	logLevel, err := zerolog.ParseLevel(strings.ToLower(os.Getenv("LOG_LEVEL")))
	if err != nil {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		log.Debug().Msg("Unspecified or invalid log level, setting the level to default (INFO)...")
	} else {
		zerolog.SetGlobalLevel(logLevel)
		log.Debug().Msgf("Log level set to %v.", logLevel)
	}
}

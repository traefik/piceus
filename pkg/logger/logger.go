package logger

import (
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Setup is configuring the logger.
func Setup() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	log.Logger = zerolog.New(os.Stderr).With().Caller().Logger()

	rawLevel := strings.ToLower(os.Getenv("LOG_LEVEL"))

	logLevel, err := zerolog.ParseLevel(rawLevel)
	if err != nil {
		logLevel = zerolog.InfoLevel

		log.Debug().Err(err).Str("LOG_LEVEL", rawLevel).Msg("Unspecified or invalid log level, setting the level to default (INFO)...")
	}

	zerolog.SetGlobalLevel(logLevel)

	log.Debug().Msgf("Log level set to %s.", logLevel)
}

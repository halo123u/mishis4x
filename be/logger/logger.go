// Package logger configures the process-wide zerolog logger. Call Init once
// at startup (see cmd/root.go); every other package just imports
// github.com/rs/zerolog/log and uses its global logger.
package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Init sets the global zerolog logger. env == "local" gets human-readable
// colored console output; anything else gets structured JSON on stdout,
// which is what you want for a real deploy (grep/ingest-friendly).
func Init(env string) {
	zerolog.TimeFieldFormat = time.RFC3339

	if env == "local" {
		log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen}).
			With().Timestamp().Caller().Logger()
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		return
	}

	log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}

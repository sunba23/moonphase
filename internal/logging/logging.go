package logging

import (
	"os"

	"github.com/rs/zerolog"
)

// New builds a structured JSON logger writing to stdout.
func New() *zerolog.Logger {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	return &logger
}

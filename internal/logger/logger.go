package logger

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

func New(env string) zerolog.Logger {
	level := zerolog.InfoLevel
	if strings.EqualFold(env, "development") {
		level = zerolog.DebugLevel
	}

	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}

	return zerolog.New(output).
		Level(level).
		With().
		Timestamp().
		Logger()
}

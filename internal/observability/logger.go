package observability

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func InitLogger(app string) zerolog.Logger {
	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}
	logger := zerolog.New(output).With().Timestamp().Str("app", app).Logger()
	log.Logger = logger
	return logger
}

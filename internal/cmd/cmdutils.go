package cmd

import (
	"errors"
	"os"

	"github.com/go-logr/logr"
)

func FatalError(l logr.Logger, err error, msg string, fields ...interface{}) {
	l.Error(err, msg, fields...)
	os.Exit(1)
}

func GetEnvOrFatalError(name string, logger logr.Logger) string {
	val := os.Getenv(name)
	if val == "" {
		FatalError(logger, errors.New("empty value"), "Could not get the environment variable", "name", name)
	}

	return val
}

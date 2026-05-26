package cmd

import (
	"os"

	"github.com/mblarsen/env-lease/internal/presentation"
)

func colorsEnabled(output *os.File) bool {
	return presentation.ColorsEnabled(output)
}

func colorsEnabledWithEnv(output *os.File, lookupEnv func(string) (string, bool)) bool {
	return presentation.ColorsEnabledWithEnv(output, lookupEnv)
}

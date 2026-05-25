package cmd

import (
	"os"

	"github.com/charmbracelet/x/term"
)

func colorsEnabled(output *os.File) bool {
	return colorsEnabledWithEnv(output, os.LookupEnv)
}

func colorsEnabledWithEnv(output *os.File, lookupEnv func(string) (string, bool)) bool {
	if _, disabled := lookupEnv("NO_COLOR"); disabled {
		return false
	}
	if output == nil {
		return false
	}

	return term.IsTerminal(output.Fd())
}

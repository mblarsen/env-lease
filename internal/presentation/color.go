package presentation

import (
	"os"

	"github.com/charmbracelet/x/term"
)

// ColorsEnabled reports whether ANSI color should be emitted for output.
func ColorsEnabled(output *os.File) bool {
	return ColorsEnabledWithEnv(output, os.LookupEnv)
}

// ColorsEnabledWithEnv is ColorsEnabled with injectable environment lookup for tests.
func ColorsEnabledWithEnv(output *os.File, lookupEnv func(string) (string, bool)) bool {
	if _, disabled := lookupEnv("NO_COLOR"); disabled {
		return false
	}
	if output == nil {
		return false
	}

	return term.IsTerminal(output.Fd())
}

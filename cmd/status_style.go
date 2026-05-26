package cmd

import "github.com/mblarsen/env-lease/internal/presentation"

func formatStatusOutput(output string) string {
	return presentation.FormatStatusOutput(output)
}

func styleStatusOutput(output string) string {
	return presentation.StyleStatusOutput(output)
}

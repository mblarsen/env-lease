package cmd

import "github.com/mblarsen/env-lease/internal/presentation"

func formatConfirmPrompt(prompt string) string {
	return presentation.FormatConfirmPrompt(prompt)
}

func plainConfirmPrompt(prompt string) string {
	return presentation.PlainConfirmPrompt(prompt)
}

func styleConfirmPrompt(prompt string) string {
	return presentation.StyleConfirmPrompt(prompt)
}

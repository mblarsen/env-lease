package cmd

import (
	"regexp"

	"github.com/charmbracelet/lipgloss/v2"
)

var (
	confirmPromptTextStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#94a3b8"))
	confirmPromptQuoteStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#f8fafc"))
	confirmPromptVariableStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#38bdf8"))
)

var grantPromptPattern = regexp.MustCompile(`^(Grant) '([^']+)'([?])$`)

func styleConfirmPrompt(prompt string) string {
	return styleConfirmPromptText(prompt) + " " + styleConfirmOptions() + ": "
}

func styleConfirmPromptText(prompt string) string {
	matches := grantPromptPattern.FindStringSubmatch(prompt)
	if len(matches) == 4 {
		return confirmPromptTextStyle.Render(matches[1]) +
			" " +
			confirmPromptQuoteStyle.Render("'") +
			confirmPromptVariableStyle.Render(matches[2]) +
			confirmPromptQuoteStyle.Render("'") +
			confirmPromptTextStyle.Render(matches[3])
	}

	return confirmPromptTextStyle.Render(prompt)
}

func styleConfirmOptions() string {
	return confirmPromptTextStyle.Render("[y/n/a/d/?]")
}

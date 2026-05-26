package presentation

import (
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss/v2"
)

var (
	statusHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#7dd3fc")).
				Bold(true).
				Underline(true)
	statusTreeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#34d399")).
			Bold(true)
	statusVariableStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#94a3b8"))
	statusPlaceholderVariableStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#38bdf8")).
					Bold(true)
	statusSourceSchemeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#38bdf8")).
				Bold(true)
	statusSourceSeparatorStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#64748b"))
	statusSourcePathStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#64748b"))
	statusSourceParentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#94a3b8"))
	statusSourceNameStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#f8fafc"))
	statusPathDirStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#64748b"))
	statusPathFileStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#e2e8f0")).
				Bold(true)
	statusDurationNumberStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#94a3b8"))
	statusDurationUnitStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#64748b"))
)

var (
	statusAbsolutePathPattern = regexp.MustCompile(`/(?:[^/\s]+/)*[^/\s]+`)
	statusDurationPattern     = regexp.MustCompile(`\b\d+(?:h|m|s)(?:\d+(?:h|m|s))*\b`)
	statusDurationPartPattern = regexp.MustCompile(`(\d+)([hms])`)
)

func FormatStatusOutput(output string) string {
	if !ColorsEnabled(os.Stdout) {
		return output
	}

	return StyleStatusOutput(output)
}

func StyleStatusOutput(output string) string {
	if output == "" {
		return output
	}

	lines := strings.SplitAfter(output, "\n")
	for i, line := range lines {
		lineEnding := ""
		text := line
		if strings.HasSuffix(line, "\n") {
			lineEnding = "\n"
			text = strings.TrimSuffix(line, "\n")
		}

		if strings.HasPrefix(text, "VARIABLE") {
			text = styleStatusHeader(text)
		} else {
			text = styleStatusLine(text)
		}

		lines[i] = text + lineEnding
	}

	return strings.Join(lines, "")
}

func styleStatusHeader(line string) string {
	for _, header := range []string{"VARIABLE", "SOURCE", "DESTINATION", "EXPIRES IN"} {
		line = strings.Replace(line, header, statusHeaderStyle.Render(header), 1)
	}
	return line
}

func styleStatusLine(line string) string {
	line = styleStatusDurations(line)

	destinationStart := firstStatusDestinationPathStart(line)
	if destinationStart >= 0 {
		line = styleStatusSource(line[:destinationStart]) + styleStatusPaths(line[destinationStart:])
	} else {
		line = styleStatusSource(line)
	}

	line = styleStatusVariableName(line)
	line = styleStatusTreeSymbols(line)
	return line
}

func styleStatusTreeSymbols(line string) string {
	for _, symbol := range []string{"├─", "└─", "│"} {
		line = strings.ReplaceAll(line, symbol, statusTreeStyle.Render(symbol))
	}
	return line
}

func styleStatusVariableName(line string) string {
	if start := strings.Index(line, "├─ "); start >= 0 {
		return styleStatusVariableNameAt(line, start+len("├─ "))
	}
	if start := strings.Index(line, "└─ "); start >= 0 {
		return styleStatusVariableNameAt(line, start+len("└─ "))
	}

	start := 0
	for start < len(line) && line[start] == ' ' {
		start++
	}
	return styleStatusVariableNameAt(line, start)
}

func styleStatusVariableNameAt(line string, start int) string {
	if start >= len(line) {
		return line
	}

	end := start
	for end < len(line) && line[end] != ' ' && line[end] != '\t' {
		end++
	}
	if end == start {
		return line
	}

	variable := line[start:end]
	if variable == "<exploded>" {
		return line[:start] + statusPlaceholderVariableStyle.Render(variable) + line[end:]
	}

	return line[:start] + statusVariableStyle.Render(variable) + line[end:]
}

func styleStatusSource(line string) string {
	schemeEnd := strings.Index(line, "://")
	if schemeEnd < 0 {
		return line
	}

	schemeStart := schemeEnd
	for schemeStart > 0 && isStatusSchemeCharacter(line[schemeStart-1]) {
		schemeStart--
	}

	sourceEnd := len(line)
	for sourceEnd > schemeEnd+3 && line[sourceEnd-1] == ' ' {
		sourceEnd--
	}

	source := line[schemeStart:sourceEnd]
	padding := line[sourceEnd:]
	pathStart := schemeEnd - schemeStart + len("://")

	var styled strings.Builder
	styled.WriteString(line[:schemeStart])
	styled.WriteString(statusSourceSchemeStyle.Render(source[:schemeEnd-schemeStart]))
	styled.WriteString(statusSourceSeparatorStyle.Render(source[schemeEnd-schemeStart : pathStart]))
	styled.WriteString(styleStatusSourcePath(source[pathStart:]))
	styled.WriteString(padding)
	return styled.String()
}

func styleStatusSourcePath(path string) string {
	if path == "" {
		return path
	}

	parts := strings.Split(path, "/")
	var styled strings.Builder
	for i, part := range parts {
		if part != "" {
			styled.WriteString(styleStatusSourcePathPart(part, i, len(parts)))
		}
		if i < len(parts)-1 {
			styled.WriteString(statusSourceSeparatorStyle.Render("/"))
		}
	}
	return styled.String()
}

func styleStatusSourcePathPart(part string, index, count int) string {
	switch index {
	case count - 1:
		return statusSourceNameStyle.Render(part)
	case count - 2:
		return statusSourceParentStyle.Render(part)
	default:
		return statusSourcePathStyle.Render(part)
	}
}

func isStatusSchemeCharacter(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '+' || b == '-' || b == '.'
}

func styleStatusPaths(line string) string {
	matches := statusAbsolutePathPattern.FindAllStringIndex(line, -1)
	if len(matches) == 0 {
		return line
	}

	var styled strings.Builder
	last := 0
	for _, match := range matches {
		start, end := match[0], match[1]
		path := line[start:end]

		styled.WriteString(line[last:start])
		if isStatusDestinationPathStart(line, start) {
			styled.WriteString(styleStatusPath(path))
		} else {
			styled.WriteString(path)
		}
		last = end
	}
	styled.WriteString(line[last:])
	return styled.String()
}

func firstStatusDestinationPathStart(line string) int {
	matches := statusAbsolutePathPattern.FindAllStringIndex(line, -1)
	for _, match := range matches {
		if isStatusDestinationPathStart(line, match[0]) {
			return match[0]
		}
	}
	return -1
}

func isStatusDestinationPathStart(line string, start int) bool {
	if start == 0 {
		return true
	}

	previous := line[start-1]
	return previous == ' ' || previous == '\t'
}

func styleStatusPath(path string) string {
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash < 0 || lastSlash == len(path)-1 {
		return path
	}

	dir := path[:lastSlash+1]
	file := path[lastSlash+1:]
	return statusPathDirStyle.Render(dir) + statusPathFileStyle.Render(file)
}

func styleStatusDurations(line string) string {
	return statusDurationPattern.ReplaceAllStringFunc(line, func(duration string) string {
		return statusDurationPartPattern.ReplaceAllStringFunc(duration, func(part string) string {
			matches := statusDurationPartPattern.FindStringSubmatch(part)
			if len(matches) != 3 {
				return part
			}

			return statusDurationNumberStyle.Render(matches[1]) +
				statusDurationUnitStyle.Render(matches[2])
		})
	})
}

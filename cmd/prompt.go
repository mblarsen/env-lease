package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// confirm prompts the user for a yes/no answer.
// It returns true if the user answers yes, and false otherwise.
// confirm is a variable so we can swap it out in tests
var confirm = func(prompt string) bool {
	var writer io.Writer = os.Stdout
	if shellMode {
		writer = os.Stderr
	}
	fmt.Fprintf(writer, "%s [y/N] ", prompt)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	response := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return response == "y" || response == "yes"
}

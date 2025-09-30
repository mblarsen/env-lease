package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// confirm prompts the user for a yes/no answer.
// It returns true if the user answers yes, and false otherwise.
func confirm(prompt string) bool {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)
		input, err := reader.ReadString('\n')
		if err != nil {
			return false
		}
		input = strings.TrimSpace(strings.ToLower(input))
		if input == "y" || input == "yes" {
			return true
		}
		return false
	}
}

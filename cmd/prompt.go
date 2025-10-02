package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// interactiveState determines the behavior of the confirmation prompt.
type interactiveState int

const (
	// stateAsk prompts the user for each confirmation.
	stateAsk interactiveState = iota
	// stateAlways assumes "yes" for all subsequent confirmations.
	stateAlways
	// stateDeny assumes "no" for all subsequent confirmations.
	stateDeny
)

var (
	// confirmState tracks the user's choice to apply to all subsequent prompts.
	confirmState = stateAsk
	// confirm is the function used to prompt the user. Can be swapped for tests.
	confirm = interactiveConfirm
)

// resetConfirmState resets the interactive confirmation state. It should be
// called at the beginning of any command that uses interactive prompts.
func resetConfirmState() {
	confirmState = stateAsk
}

// interactiveConfirm prompts the user for a decision and handles advanced
// options like 'all' and 'deny'.
func interactiveConfirm(prompt string) bool {
	return doConfirm(prompt, os.Stdin)
}

// doConfirm is the underlying implementation of the confirmation prompt.
// It reads from the provided reader, allowing for testing.
func doConfirm(prompt string, in io.Reader) bool {
	// If a decision for all subsequent prompts has been made, act on it.
	if confirmState == stateAlways {
		return true
	}
	if confirmState == stateDeny {
		return false
	}

	var writer io.Writer = os.Stdout
	if shellMode {
		writer = os.Stderr
	}

	for {
		fmt.Fprintf(writer, "%s [y/n/a/d/?] ", prompt)
		var response string
		fmt.Fscanln(in, &response)
		response = strings.TrimSpace(strings.ToLower(response))

		switch response {
		case "y", "yes":
			return true
		case "n", "no", "":
			return false
		case "a", "all":
			confirmState = stateAlways
			return true // 'a' means yes to this and all subsequent prompts
		case "d", "deny":
			confirmState = stateDeny
			return false // 'd' means no to this and all subsequent prompts
		case "?", "help":
			fmt.Fprintln(writer, "y - yes")
			fmt.Fprintln(writer, "n - no (default)")
			fmt.Fprintln(writer, "a - yes to current and all remaining")
			fmt.Fprintln(writer, "d - no to current and all remaining")
			fmt.Fprintln(writer, "? - show this help")
			continue
		default:
			fmt.Fprintf(writer, "Invalid response: %q\n", response)
			continue
		}
	}
}

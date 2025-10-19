package cmd

import (
	"bufio"
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
	confirm = func(prompt string) bool {
		return doConfirm(prompt, os.Stdin)
	}
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

// doConfirm handles the logic of prompting the user for a yes/no decision.
// It supports single answers, as well as "all" and "deny" to apply to
// subsequent prompts. It reads from the provided io.Reader.
func doConfirm(prompt string, in io.Reader) bool {
	if confirmState == stateAlways {
		return true
	}
	if confirmState == stateDeny {
		return false
	}

	reader := bufio.NewReader(in)

	for {
		fmt.Printf("%s [y/n/a/d/?]: ", prompt)
		input, err := reader.ReadString('\n')
		if err != nil {
			// On EOF, default to "no"
			if err == io.EOF {
				fmt.Println()
				return false
			}
			// In tests, this can be triggered by a closed pipe, so we'll
			// treat it as a "no". In real use, this is unlikely.
			return false
		}
		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "y", "yes":
			return true
		case "n", "no", "":
			return false
		case "a", "all":
			confirmState = stateAlways
			return true
		case "d", "deny":
			confirmState = stateDeny
			return false
		case "?", "help":
			fmt.Println("y: yes")
			fmt.Println("n: no (default)")
			fmt.Println("a: yes to all subsequent prompts")
			fmt.Println("d: no to all subsequent prompts")
			fmt.Println("?: show this help message")
		default:
			fmt.Printf("Invalid input: %q. Please try again.\n", input)
		}
	}
}

package presentation

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"
)

// StatusLease is the presentation-ready subset of daemon Lease facts needed to
// render active Lease status. Command code owns selection and authorization;
// presentation owns labels, ordering, and table layout.
type StatusLease struct {
	ID          string
	ParentID    string
	Variable    string
	Source      string
	Destination string
	LeaseType   string
	ExpiresAt   time.Time
}

// Message identifies common user-facing terminal messages.
type Message int

const (
	MessageNoActiveLeases Message = iota
	MessageNoActiveLeasesForProject
	MessageOtherActiveLeases
	MessageGrantShellHint
	MessageRevokeShellHint
	MessageGrantSent
	MessageRevokeSent
	MessageDaemonOffline
	MessageDaemonConnectionFailed
	MessageDirenvModified
	MessageDirenvTTYFailed
	MessageDirenvAllowFailed
)

// Renderer is the command package's seam into user-facing terminal output.
type Renderer interface {
	ConfirmPrompt(prompt string) string
	ConfirmHelp(out io.Writer)
	InvalidPromptInput(out io.Writer, input string)
	RenderStatus(leases []StatusLease, out io.Writer)
	Print(out io.Writer, message Message, args ...any)
	PrintLine(out io.Writer, text string)
	PrintLines(out io.Writer, lines []string)
}

// Terminal renders env-lease's conventional terminal presentation.
type Terminal struct{}

// NewTerminal creates the default Renderer implementation.
func NewTerminal() Terminal { return Terminal{} }

func (Terminal) ConfirmPrompt(prompt string) string { return FormatConfirmPrompt(prompt) }

func (Terminal) ConfirmHelp(out io.Writer) {
	fmt.Fprintln(out, "y: yes")
	fmt.Fprintln(out, "n: no (default)")
	fmt.Fprintln(out, "a: yes to all subsequent prompts")
	fmt.Fprintln(out, "d: no to all subsequent prompts")
	fmt.Fprintln(out, "?: show this help message")
}

func (Terminal) InvalidPromptInput(out io.Writer, input string) {
	fmt.Fprintf(out, "Invalid input: %q. Please try again.\n", input)
}

func (Terminal) RenderStatus(leases []StatusLease, out io.Writer) {
	grouped, topLevel := groupStatusLeases(leases)
	sort.Slice(topLevel, func(i, j int) bool {
		return topLevel[i].Destination < topLevel[j].Destination
	})

	var output bytes.Buffer
	w := tabwriter.NewWriter(&output, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "VARIABLE\tSOURCE\tDESTINATION\tEXPIRES IN")

	for _, displayed := range topLevel {
		expiresIn := time.Until(displayed.ExpiresAt).Round(time.Second)
		variable := statusVariableLabel(displayed)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", variable, displayed.Source, displayed.Destination, expiresIn)

		if childLeases, ok := grouped[displayed.ID]; ok {
			sort.Slice(childLeases, func(i, j int) bool {
				return childLeases[i].Variable < childLeases[j].Variable
			})
			for i, child := range childLeases {
				expiresInChild := time.Until(child.ExpiresAt).Round(time.Second)
				connector := "├─"
				if i == len(childLeases)-1 {
					connector = "└─"
				}
				fmt.Fprintf(w, " %s %s\t\t%s\t%s\n", connector, child.Variable, child.Destination, expiresInChild)
			}
		}
	}
	_ = w.Flush()
	fmt.Fprint(out, FormatStatusOutput(output.String()))
}

func (t Terminal) Print(out io.Writer, message Message, args ...any) {
	switch message {
	case MessageNoActiveLeases:
		t.PrintLine(out, "No active leases.")
	case MessageNoActiveLeasesForProject:
		t.PrintLine(out, "No active leases for this project.")
	case MessageOtherActiveLeases:
		t.PrintLine(out, "-------------------------------------------------------")
		fmt.Fprintf(out, "%d more active leases. Use --all to show all leases.\n", args[0])
	case MessageGrantShellHint:
		t.PrintLine(out, "# When using shell lease types run this command like `eval $(env-lease grant)`")
	case MessageRevokeShellHint:
		t.PrintLine(out, "# When using shell lease types run this command like `eval $(env-lease revoke)`")
	case MessageGrantSent:
		t.PrintLine(out, "Grant request sent successfully.")
	case MessageRevokeSent:
		t.PrintLine(out, "Revoke request sent.")
	case MessageDaemonOffline:
		t.PrintLine(out, "Error: env-lease daemon is not running. Please start it with 'env-lease daemon install'.")
	case MessageDaemonConnectionFailed:
		t.PrintLine(out, "Error: could not connect to the env-lease daemon. Is it running?")
	case MessageDirenvModified:
		t.PrintLine(out, ".envrc modified. Run 'direnv allow' to apply changes.")
	case MessageDirenvTTYFailed:
		fmt.Fprintf(out, "Failed to open tty: %v\n", args[0])
	case MessageDirenvAllowFailed:
		fmt.Fprintf(out, "direnv allow failed: %v\n", args[0])
	}
}

func (Terminal) PrintLine(out io.Writer, text string) { fmt.Fprintln(out, text) }

func (t Terminal) PrintLines(out io.Writer, lines []string) {
	for _, line := range lines {
		t.PrintLine(out, line)
	}
}

func groupStatusLeases(leases []StatusLease) (map[string][]StatusLease, []StatusLease) {
	grouped := make(map[string][]StatusLease)
	var topLevel []StatusLease
	for _, l := range leases {
		if l.ParentID != "" {
			grouped[l.ParentID] = append(grouped[l.ParentID], l)
		} else {
			topLevel = append(topLevel, l)
		}
	}
	return grouped, topLevel
}

func statusVariableLabel(l StatusLease) string {
	if l.Variable != "" {
		return l.Variable
	}
	if l.LeaseType == "file" {
		return "<file>"
	}
	return "<exploded>"
}

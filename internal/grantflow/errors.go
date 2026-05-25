package grantflow

import (
	"fmt"
	"strings"
)

// Error associates a Grant workflow failure with the Lease source that caused it.
type Error struct {
	Source string
	Err    error
}

// Errors is the aggregated Grant workflow error shown by the CLI.
type Errors struct {
	errs []Error
}

// List returns the underlying per-Lease errors.
func (e Errors) List() []Error {
	return append([]Error(nil), e.errs...)
}

func (e Errors) Error() string {
	var sb strings.Builder
	if len(e.errs) > 1 {
		sb.WriteString(fmt.Sprintf("Failed to grant %d leases:\n\n", len(e.errs)))
	} else {
		sb.WriteString("Failed to grant lease:\n\n")
	}
	for _, ge := range e.errs {
		sb.WriteString(fmt.Sprintf("Lease: %s\n", ge.Source))
		sb.WriteString(fmt.Sprintf("└─ Error: %s\n\n", ge.Err))
	}

	if len(e.errs) > 1 {
		sb.WriteString("Note: Other leases may have been granted successfully.\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

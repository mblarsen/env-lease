// Package cmd provides the command-line interface for env-lease. The grant
// command is the core of the application, responsible for fetching secrets and
// managing their lifecycle. It supports both a standard, non-interactive mode
// and a detailed interactive mode for granular control over lease approvals.
//
// # Interactive Grant Workflow (`grant --interactive`)
//
// The interactive grant process is designed to be secure, efficient, and
// user-friendly. It follows a strict, multi-phase workflow to ensure a
// predictable user experience.
//
// ## Key Design Principles
//
//  1. **Just-in-Time Secret Fetching**: Secrets are only retrieved from the
//     provider *after* the user approves the corresponding lease. This minimizes
//     unnecessary access to sensitive data.
//  2. **Intelligent Batching**: To minimize latency, approved `op://` leases
//     that share the same `op_account` are fetched together in a single,
//     batched `op` CLI call.
//  3. **Efficient Caching**: `op+file://` sources are fetched only once per
//     run. If multiple leases use the same `op+file://` URI, the content is
//     fetched for the first approved lease and then reused from an in-memory
//     cache for all subsequent leases in the same run.
//  4. **Strictly Ordered Workflow**: The flow is separated into distinct,
//     predictable phases: a complete pass for approving sources (Round 1), a
//     parallelized fetching phase, and a final pass for approving individual
//     secrets from `explode` leases (Round 2).
//  5. **Descriptive & Multi-Stage Prompting**: For leases with an `explode`
//     transform, the user is guided through a two-stage approval process. To
//     avoid ambiguity, prompts for such leases include details from the
//     transformation pipeline (e.g., `select 'production'`), ensuring the user
//     knows exactly which configuration they are approving.
//
// ## The User Workflow
//
// ### Phase 1: Round 1 - Approve Sources
//
// The command first makes a complete pass through the `env-lease.toml`
// configuration, prompting the user to approve or deny each top-level
// `[[lease]]` block. No secrets are fetched during this phase.
//
// ### Phase 2: Fetch Secrets
//
// Once Round 1 is complete, the system executes all necessary secret lookups
// for the approved sources. To maximize speed, these lookups are performed
// **in parallel**:
//   - One batched `op` call is made for each group of approved `op://` leases
//     that share an `op_account`.
//   - One individual `op` call is made for each unique `op+file://` URI that
//     was approved.
//
// ### Phase 3: Round 2 - Approve Individual Secrets (Optional)
//
// This is an optional phase that only runs for `explode` leases that were
// approved in Round 1. All simple (non-`explode`) leases approved in the first
// round are now considered final and are ready to be granted without any
// further prompts.
//
// ### Phase 4: Grant Leases
//
// Once all approvals are gathered, the final, verified list of leases is sent
// to the `env-lease` daemon to be activated.
//
// THESE COMMENTS ARE INTENDED TO BE VERBOSE AND THEY ARE AT NO POINT ALLOWED TO
// BE REMOVED OR TRIMMED.
package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/fileutil"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/mblarsen/env-lease/internal/lease"
	"github.com/mblarsen/env-lease/internal/secretlookup"
	"github.com/mblarsen/env-lease/internal/transform"
	"github.com/spf13/cobra"
)

var shellMode bool

type grantError struct {
	Source string
	Err    error
}

type GrantErrors struct {
	errs []grantError
}

func (e *GrantErrors) Error() string {
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

func processSingleLease(cmd *cobra.Command, l lease.Lease, secretVal string, projectRoot string, absConfigFile string, interactive bool, errs *[]grantError, continueOnError bool) ([]ipc.Lease, []string, error) {
	// Duration validation
	duration, err := time.ParseDuration(l.Duration)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid duration '%s': %w", l.Duration, err)
	}
	if duration > 12*time.Hour {
		slog.Warn("Leases longer than 12 hours are discouraged for security reasons.")
	}

	// Handle result: could be a single string or exploded data
	var approvedLeases []ipc.Lease
	var approvedShellCommands []string

	// Pre-determine the prompt string
	isExplode := l.IsExplode()

	var prompt string
	if isExplode {
		prompt = fmt.Sprintf("Grant leases from '%s'?", l.Source)
	} else if l.Variable == "" {
		prompt = fmt.Sprintf("Grant lease for '%s'?", l.Source)
	} else {
		prompt = fmt.Sprintf("Grant lease for '%s'?", l.Variable)
	}

	if !interactive || (secretVal != "") || confirm(prompt) {
		// Fetch secret if not already fetched
		if secretVal == "" {
			slog.Info("Fetching secret", "source", l.Source)
			secrets, lookupErrs, err := secretlookup.Fetch([]lease.Lease{l}, secretlookup.Options{})
			if len(lookupErrs) > 0 {
				return nil, nil, fmt.Errorf("failed to fetch secret: %w", lookupErrs[0].Err)
			}
			if err != nil {
				return nil, nil, fmt.Errorf("failed to fetch secret: %w", err)
			}
			var ok bool
			secretVal, ok = secrets.Get(l)
			if !ok {
				return nil, nil, fmt.Errorf("failed to fetch secret: no secret returned for %s", l.Source)
			}
			slog.Info("Fetched secret", "source", l.Source)
		}

		// Run transform pipeline
		var transformResult interface{} = secretVal
		if len(l.Transform) > 0 {

			pipeline, err := transform.NewPipeline(l.Transform)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create transform pipeline: %w", err)
			}
			transformResult, err = pipeline.Run(secretVal)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to transform secret: %w", err)
			}
		}

		switch result := transformResult.(type) {
		case string:
			// SINGLE LEASE CASE
			finalLeases, sc, err := processLease(cmd, l, result, projectRoot, absConfigFile)
			if err != nil {
				return nil, nil, err
			}
			approvedLeases = append(approvedLeases, finalLeases...)
			approvedShellCommands = append(approvedShellCommands, sc...)

		case transform.ExplodedData:
			// EXPLODED LEASE CASE
			if l.LeaseType == "file" {
				return nil, nil, fmt.Errorf("'explode' transform cannot be used with lease_type 'file'")
			}

			// Add a parent/container lease for the status command to find
			parentLeaseConfig := l
			parentLeaseConfig.Variable = "" // No single variable for the parent
			parentLeases, _, err := processLease(cmd, parentLeaseConfig, "", projectRoot, absConfigFile)
			if err != nil {
				return nil, nil, err
			}
			approvedLeases = append(approvedLeases, parentLeases...)
			uniqueParentID := parentLeaseConfig.ParentIdentity()

			// Process all the child leases
			fmt.Fprintf(os.Stderr, "Granting sub-leases from '%s'%s:\n", l.Source, getTransformSummary(l.Transform))
			for key, value := range result {
				if !interactive || confirm(fmt.Sprintf("Grant lease for '%s'?", key)) {
					explodedLeaseConfig := l
					explodedLeaseConfig.Variable = key
					explodedLeaseConfig.ParentSource = uniqueParentID

					finalLeases, sc, err := processLease(cmd, explodedLeaseConfig, value, projectRoot, absConfigFile)
					if err != nil {
						*errs = append(*errs, grantError{Source: key, Err: err})
						if !continueOnError {
							return nil, nil, err
						}
						continue
					}
					approvedLeases = append(approvedLeases, finalLeases...)
					approvedShellCommands = append(approvedShellCommands, sc...)
				}
			}
		}
	}
	return approvedLeases, approvedShellCommands, nil
}

func lookupGrantErrors(lookupErrs []secretlookup.Error) []grantError {
	errs := make([]grantError, 0, len(lookupErrs))
	for _, lookupErr := range lookupErrs {
		errs = append(errs, grantError{Source: lookupErr.Lease.Source, Err: lookupErr.Err})
	}
	return errs
}

var grantCmd = &cobra.Command{
	Use:   "grant",
	Short: "Grant all leases defined in env-lease.toml.",
	Long: `Grant all leases defined in env-lease.toml.

For security, file-based leases are restricted to writing files only within the
project root directory (the directory containing the env-lease.toml file).
This can be overridden with the --destination-outside-root flag.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		resetConfirmState()
		var (
			cfg           *config.Config
			err           error
			absConfigFile string
		)

		configFileFlag, _ := cmd.Flags().GetString("config")
		localConfigFileFlag, _ := cmd.Flags().GetString("local-config")
		configFile, err := config.ResolveConfigFile(configFileFlag)
		if err != nil {
			return err
		}
		cfg, err = config.Load(configFile, localConfigFileFlag)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		absConfigFile = filepath.Join(cfg.Root, filepath.Base(configFile))
		leaseSet, err := lease.Normalize(cfg, absConfigFile)
		if err != nil {
			return fmt.Errorf("failed to normalize leases: %w", err)
		}

		interactive, _ := cmd.Flags().GetBool("interactive")
		appendMode, _ := cmd.Flags().GetBool("append")
		if appendMode && !interactive {
			return fmt.Errorf("--append requires --interactive")
		}

		// Check if stdout is a terminal
		stat, _ := os.Stdout.Stat()
		isPiped := (stat.Mode() & os.ModeCharDevice) == 0

		if isPiped && interactive && os.Getenv("ENV_LEASE_TEST") != "1" {
			return fmt.Errorf("interactive mode is not supported when piping output (e.g., inside 'eval $(...)')\n" +
				"Please run 'eval $(env-lease grant)' without the interactive flag.")
		}

		for _, l := range leaseSet.Leases {
			if l.LeaseType == "shell" {
				shellMode = true
				break
			}
		}

		client := ensureDaemonClient()

		if interactive {
			return interactiveGrant(cmd, leaseSet, client)
		}

		continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
		var errs []grantError
		var shellCommands []string
		leases := make([]ipc.Lease, 0, len(leaseSet.Leases))

		fetched, fetchErrs, fetchErr := secretlookup.Fetch(leaseSet.Leases, secretlookup.Options{ContinueOnError: continueOnError, Mode: "non-interactive"})
		errs = append(errs, lookupGrantErrors(fetchErrs)...)
		if fetchErr != nil {
			return &GrantErrors{errs: errs}
		}

		for _, l := range leaseSet.Leases {
			secretVal, ok := fetched.Get(l)
			if !ok {
				// Missing secret indicates a prior fetch failure.
				if !continueOnError {
					return &GrantErrors{errs: errs}
				}
				continue
			}
			finalLeases, sc, err := processSingleLease(cmd, l, secretVal, leaseSet.Root, leaseSet.ConfigFile, false, &errs, continueOnError)
			if err != nil {
				errs = append(errs, grantError{Source: l.Source, Err: err})
				if !continueOnError {
					return &GrantErrors{errs: errs}
				}
				continue
			}
			leases = append(leases, finalLeases...)
			shellCommands = append(shellCommands, sc...)
		}

		if len(errs) > 0 {
			return &GrantErrors{errs: errs}
		}

		override, _ := cmd.Flags().GetBool("override")
		req := ipc.GrantRequest{
			Command:    "grant",
			Leases:     leases,
			Override:   override,
			Append:     false,
			ConfigFile: leaseSet.ConfigFile,
		}
		// If in test mode, don't try to send to the daemon.
		if os.Getenv("ENV_LEASE_TEST") == "1" {
			fmt.Fprintln(os.Stderr, "Grant request (test mode) processed successfully.")
			if len(errs) > 0 {
				return &GrantErrors{errs: errs}
			}
			return nil
		}

		if client != nil {
			var resp ipc.GrantResponse
			if err := client.Send(req, &resp); err != nil {
				handleClientError(err)
			}
			for _, msg := range resp.Messages {
				fmt.Fprintln(os.Stderr, msg)
			}
		} else {
			fmt.Fprintln(os.Stderr, "Grant request processed in test mode.")
		}

		noDirenv, _ := cmd.Flags().GetBool("no-direnv")
		for _, l := range leases {
			if filepath.Base(l.Destination) == ".envrc" {
				HandleDirenv(noDirenv, os.Stderr)
				break
			}
		}

		if shellMode {
			fmt.Fprintln(os.Stderr, "# When using shell lease types run this command like `eval $(env-lease grant)`")
			for _, cmd := range shellCommands {
				fmt.Println(cmd)
			}
		}
		fmt.Fprintln(os.Stderr, "Grant request sent successfully.")
		return nil
	},
}

// processLease handles the logic for processing a single lease, including validating
// file paths, writing lease files, and preparing the lease for communication with
// the daemon. It returns a slice of IPC leases, a slice of shell commands, or an
// error.
//
// Parameters:
//   - cmd: The cobra.Command object, used to access command-line flags.
//   - l: The config.Lease object containing the lease details.
//   - secretVal: The secret value fetched from the provider.
//   - projectRoot: The absolute path to the project root directory, which is the
//     directory containing the configuration file. This is used to resolve
//     relative paths for file-based leases and ensure they are written within the
//     project directory for security.
//   - configFile: The absolute path to the configuration file. This is stored in the
//     lease object to allow the daemon to associate the lease with a specific
//     project, which is crucial for commands like `env-lease status` and
//     `env-lease revoke` to correctly identify leases for the current project.
func processLease(cmd *cobra.Command, l lease.Lease, secretVal, projectRoot, configFile string) ([]ipc.Lease, []string, error) {
	var err error
	l, err = l.WithDefaultFormat()
	if err != nil {
		return nil, nil, err
	}

	var shellCommands []string
	var leases []ipc.Lease

	// For file leases, ensure the destination is within the project root.
	if l.LeaseType == "file" {
		destinationOutsideRoot, _ := cmd.Flags().GetBool("destination-outside-root")
		if !destinationOutsideRoot {
			expandedDest, err := fileutil.ExpandPath(l.Destination)
			if err != nil {
				return nil, nil, fmt.Errorf("could not expand destination path: %w", err)
			}
			isInside, err := fileutil.IsPathInsideRoot(projectRoot, expandedDest)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to validate destination path: %w", err)
			}
			if !isInside {
				return nil, nil, fmt.Errorf("destination path '%s' is outside the project root. Use --destination-outside-root to override", l.Destination)
			}
		}
	}

	if l.LeaseType == "shell" {
		if l.Variable != "" {
			shellCommands = append(shellCommands, fmt.Sprintf("export %s=%q", l.Variable, secretVal))
		}
	} else {
		// For file/env leases, only write if there's a variable,
		// or if it's a file lease. This prevents writing the
		// parent/container lease of an explode.
		if l.LeaseType == "file" || (l.LeaseType == "env" && l.Variable != "") {
			override, _ := cmd.Flags().GetBool("override")
			created, err := writeLease(l, secretVal, projectRoot, override)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to write lease: %w", err)
			}
			if created {
				fmt.Fprintf(os.Stderr, "Created file: %s\n", l.Destination)
			}
		}
	}

	ipcLease := l.ToIPC()
	ipcLease.ConfigFile = configFile
	leases = append(leases, ipcLease)
	return leases, shellCommands, nil
}

func init() {
	grantCmd.Flags().Bool("override", false, "Override existing values in destination files.")
	grantCmd.Flags().Bool("continue-on-error", false, "Continue granting leases even if one fails.")
	grantCmd.Flags().Bool("no-direnv", false, "Do not automatically run 'direnv allow'.")
	grantCmd.Flags().StringP("config", "c", "env-lease.toml", "Path to config file.")
	grantCmd.Flags().String("local-config", "", "Path to local override config file.")
	grantCmd.Flags().BoolP("interactive", "i", false, "Prompt for confirmation before granting each lease.")
	grantCmd.Flags().Bool("append", false, "In interactive mode, keep existing granted leases and only add newly approved leases. Skipped prompts are left unchanged.")
	grantCmd.Flags().Bool("destination-outside-root", false, "Allow file-based leases to write outside of the project root.")
	rootCmd.AddCommand(grantCmd)
}

// getTransformSummary creates a short, human-readable summary of the transform pipeline.
func getTransformSummary(transforms []string) string {
	if len(transforms) == 0 {
		return ""
	}
	return fmt.Sprintf(" (%s)", strings.Join(transforms, ", "))
}

// interactiveGrant orchestrates the user-facing interactive lease approval process.
// It is designed around a multi-phase workflow to provide a clear, consistent,
// and secure user experience, deferring all secret lookups until after the user
// has explicitly approved them.
//
// The function executes the following distinct phases:
//
// ### Phase 1: Round 1 - Approve Sources
// The function first makes a complete pass through all `[[lease]]` blocks from
// the configuration. It generates a descriptive prompt for each lease, including
// transformation details for `explode` leases to avoid ambiguity. It collects
// all of the user's 'yes' or 'no' responses for these top-level sources
// without fetching any secrets.
//
// ### Phase 2: Fetch Secrets
// After Round 1 is complete, the function identifies all unique secret sources
// that need to be fetched based on the user's approvals. It then fetches these
// secrets in parallel to maximize efficiency:
//   - `op://` sources are grouped by `op_account` and fetched in batches.
//   - `op+file://` sources are fetched individually, with the content of each
//     unique URI being fetched only once and then cached for the remainder of the
//     run.
//
// ### Phase 3: Round 2 - Approve Individual Secrets (Optional)
// If any of the leases approved in Round 1 were `explode` leases, this phase
// begins. The function iterates through the now-fetched and parsed secrets and
// prompts the user to approve each individual key-value pair that resulted from
// the `explode` transformation. Simple, non-exploding leases that were approved
// in Round 1 are considered final and are not part of this phase.
//
// ### Phase 4: Grant Leases
// Finally, the function gathers all the approved leases (both simple leases from
// Round 1 and sub-leases from Round 2) into a single list and sends it to the
// `env-lease` daemon to be activated. It also handles the output of any shell
// commands for `shell` type leases.
func interactiveGrant(cmd *cobra.Command, leaseSet *lease.Set, client *ipc.Client) error {
	slog.Debug("interactive grant: phase 1 start", "lease_count", len(leaseSet.Leases))
	// ------- Phase 1: ROUND 1 – APPROVE SOURCES -------
	selectedLeases := make([]lease.Lease, 0, len(leaseSet.Leases))
	for _, l := range leaseSet.Leases {
		isExplode := l.IsExplode()

		var key string
		if isExplode {
			// Descriptive label with transformation breadcrumbs
			key = fmt.Sprintf("leases from '%s'%s", l.Source, getTransformSummary(l.Transform))
		} else if l.Variable != "" {
			key = fmt.Sprintf("'%s'", l.Variable)
		} else {
			key = fmt.Sprintf("'%s'", l.Source)
		}

		if confirm(fmt.Sprintf("Grant %s?", key)) {
			selectedLeases = append(selectedLeases, l)
		}
	}

	if len(selectedLeases) == 0 {
		fmt.Fprintln(os.Stderr, "No leases selected.")
		return nil
	}

	slog.Debug("interactive grant: phase 1 approvals",
		"selected_count", len(selectedLeases),
		"skipped_count", len(leaseSet.Leases)-len(selectedLeases))

	continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
	override, _ := cmd.Flags().GetBool("override")
	appendMode, _ := cmd.Flags().GetBool("append")
	noDirenv, _ := cmd.Flags().GetBool("no-direnv")

	if appendMode {
		fmt.Fprintln(os.Stderr, "Append mode enabled: skipped leases will remain unchanged.")
	}

	var errs []grantError

	fetched, fetchErrs, fetchErr := secretlookup.Fetch(selectedLeases, secretlookup.Options{ContinueOnError: continueOnError, Mode: "interactive"})
	errs = append(errs, lookupGrantErrors(fetchErrs)...)
	if fetchErr != nil {
		return &GrantErrors{errs: errs}
	}

	// Pre-compute transform results without writing or prompting.
	type child struct {
		lease lease.Lease
		value string
	}
	type simple struct {
		lease lease.Lease
		value string
	}
	explodedChildren := make([]child, 0)
	simpleApproved := make([]simple, 0)
	parentApproved := make([]ipc.Lease, 0)

	slog.Debug("interactive grant: preprocessing transforms",
		"selected_count", len(selectedLeases))

	for _, l := range selectedLeases {
		isExplode := l.IsExplode()
		raw, ok := fetched.Get(l)
		if !ok {
			errs = append(errs, grantError{Source: l.Source, Err: fmt.Errorf("no secret returned")})
			if !continueOnError {
				return &GrantErrors{errs: errs}
			}
			continue
		}

		formatted := l
		slog.Debug("interactive grant: preparing lease",
			"source", formatted.Source,
			"explode", isExplode,
			"transform_steps", len(formatted.Transform))
		if len(formatted.Transform) == 0 {
			// simple lease; Phase 1 already approved — no more prompts later
			simpleApproved = append(simpleApproved, simple{lease: formatted, value: raw})
			continue
		}

		pipe, err := transform.NewPipeline(formatted.Transform)
		if err != nil {
			errs = append(errs, grantError{Source: formatted.Source, Err: err})
			if !continueOnError {
				return &GrantErrors{errs: errs}
			}
			continue
		}
		res, err := pipe.Run(raw)
		if err != nil {
			errs = append(errs, grantError{Source: formatted.Source, Err: err})
			if !continueOnError {
				return &GrantErrors{errs: errs}
			}
			continue
		}
		if data, ok := res.(transform.ExplodedData); ok {
			slog.Debug("interactive grant: explode result",
				"source", formatted.Source,
				"child_count", len(data))

			parentLeaseConfig := formatted
			parentLeaseConfig.Variable = ""

			parentLeases, _, err := processLease(cmd, parentLeaseConfig, "", leaseSet.Root, leaseSet.ConfigFile)
			if err != nil {
				errs = append(errs, grantError{Source: formatted.Source, Err: err})
				if !continueOnError {
					return &GrantErrors{errs: errs}
				}
				continue
			}
			if len(parentLeases) == 0 {
				errs = append(errs, grantError{Source: formatted.Source, Err: fmt.Errorf("explode parent produced no leases")})
				if !continueOnError {
					return &GrantErrors{errs: errs}
				}
				continue
			}

			uniqueParentID := parentLeaseConfig.ParentIdentity()
			for i := range parentLeases {
				parentLeases[i].ParentSource = ""
			}
			parentApproved = append(parentApproved, parentLeases...)

			for k, v := range data {
				childLease := formatted
				childLease.Variable = k
				childLease.ParentSource = uniqueParentID
				explodedChildren = append(explodedChildren, child{
					lease: childLease,
					value: v,
				})
			}
		} else if s, ok := res.(string); ok {
			// pipeline resulted in single value — treat as simple
			slog.Debug("interactive grant: pipeline produced single value", "source", formatted.Source)
			formatted.Variable = strings.TrimSpace(formatted.Variable)
			simpleApproved = append(simpleApproved, simple{lease: formatted, value: s})
		} else {
			errs = append(errs, grantError{Source: formatted.Source, Err: fmt.Errorf("transform pipeline must produce a string or exploded data")})
			if !continueOnError {
				return &GrantErrors{errs: errs}
			}
		}
	}

	// ------- Phase 3: ROUND 2 – APPROVE INDIVIDUAL SECRETS FOR EXPLODE -------
	slog.Debug("interactive grant: phase 3 start",
		"simple_count", len(simpleApproved),
		"explode_children", len(explodedChildren))
	finalLeases := make([]ipc.Lease, 0)
	approvedShellCommands := make([]string, 0)

	finalLeases = append(finalLeases, parentApproved...)

	// First, materialize all simple leases (no new prompts)
	for _, approved := range simpleApproved {
		l := approved.lease
		leas, sc, err := processLease(cmd, l, approved.value, leaseSet.Root, leaseSet.ConfigFile)
		if err != nil {
			errs = append(errs, grantError{Source: l.Source, Err: err})
			if !continueOnError {
				return &GrantErrors{errs: errs}
			}
			continue
		}
		finalLeases = append(finalLeases, leas...)
		approvedShellCommands = append(approvedShellCommands, sc...)
	}

	// Then, prompt for exploded keys
	// Group by parent source only for nice prompts
	for _, ch := range explodedChildren {
		prompt := fmt.Sprintf("Grant lease for '%s'?", ch.lease.Variable)
		if !confirm(prompt) {
			continue
		}
		leas, sc, err := processLease(cmd, ch.lease, ch.value, leaseSet.Root, leaseSet.ConfigFile)
		if err != nil {
			errs = append(errs, grantError{Source: ch.lease.Source, Err: err})
			if !continueOnError {
				return &GrantErrors{errs: errs}
			}
			continue
		}
		finalLeases = append(finalLeases, leas...)
		approvedShellCommands = append(approvedShellCommands, sc...)
	}

	if len(errs) > 0 && !continueOnError {
		return &GrantErrors{errs: errs}
	}

	// ------- Phase 4: GRANT (single request) -------
	slog.Debug("interactive grant: phase 4 start", "final_lease_count", len(finalLeases))
	req := ipc.GrantRequest{Command: "grant", Leases: finalLeases, Override: override, Append: appendMode, ConfigFile: leaseSet.ConfigFile}
	if client != nil {
		var resp ipc.GrantResponse
		if err := client.Send(req, &resp); err != nil {
			handleClientError(err)
		}
		for _, msg := range resp.Messages {
			fmt.Fprintln(os.Stderr, msg)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Grant request processed in test mode.")
	}

	for _, l := range finalLeases {
		if filepath.Base(l.Destination) == ".envrc" {
			HandleDirenv(noDirenv, os.Stderr)
			break
		}
	}

	if shellMode {
		fmt.Fprintln(os.Stderr, "# When using shell lease types run this command like `eval $(env-lease grant)`")
		for _, c := range approvedShellCommands {
			fmt.Println(c)
		}
	}

	fmt.Fprintln(os.Stderr, "Grant request sent successfully.")
	return nil
}

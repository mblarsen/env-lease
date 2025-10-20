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
	"github.com/mblarsen/env-lease/internal/provider"
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

func processSingleLease(cmd *cobra.Command, l config.Lease, secretVal string, projectRoot string, absConfigFile string, interactive bool, errs *[]grantError, continueOnError bool) ([]ipc.Lease, []string, error) {
	// Duration validation
	duration, err := time.ParseDuration(l.Duration)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid duration '%s': %w", l.Duration, err)
	}
	if duration > 12*time.Hour {
		slog.Warn("Leases longer than 12 hours are discouraged for security reasons.")
	}

	// Set default format
	if l.Format == "" {
		switch filepath.Base(l.Destination) {
		case ".envrc":
			l.Format = "export %s=%q"
		case ".env":
			l.Format = "%s=%q"
		default:
			if l.LeaseType == "env" {
				return nil, nil, fmt.Errorf("lease for '%s' has no format specified", l.Destination)
			}
		}
	}

	// Handle result: could be a single string or exploded data
	var approvedLeases []ipc.Lease
	var approvedShellCommands []string

	// Pre-determine the prompt string
	isExplode := false
	for _, t := range l.Transform {
		if strings.HasPrefix(strings.TrimSpace(t), "explode") {
			isExplode = true
			break
		}
	}

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
			var p provider.SecretProvider
			if os.Getenv("ENV_LEASE_TEST") == "1" {
				p = &provider.MockProvider{}
			} else {
				p = &provider.OnePasswordCLI{Account: l.OpAccount}
			}
			secretVal, err = p.Fetch(l.Source)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to fetch secret: %w", err)
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
			uniqueParentID := parentLeases[0].Source + "->" + parentLeases[0].Destination

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

		interactive, _ := cmd.Flags().GetBool("interactive")

		// Check if stdout is a terminal
		stat, _ := os.Stdout.Stat()
		isPiped := (stat.Mode() & os.ModeCharDevice) == 0

		if isPiped && interactive && os.Getenv("ENV_LEASE_TEST") != "1" {
			return fmt.Errorf("interactive mode is not supported when piping output (e.g., inside 'eval $(...)')\n" +
				"Please run 'eval $(env-lease grant)' without the interactive flag.")
		}

		for _, l := range cfg.Lease {
			if l.LeaseType == "shell" {
				shellMode = true
				break
			}
		}

		if interactive {
			return interactiveGrant(cmd, cfg, absConfigFile)
		}

		continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
		var errs []grantError
		var shellCommands []string
		leases := make([]ipc.Lease, 0, len(cfg.Lease))

		// Group leases by provider
		leasesByProvider := make(map[string][]config.Lease)
		for _, l := range cfg.Lease {
			leasesByProvider[l.Provider] = append(leasesByProvider[l.Provider], l)
		}

		for providerName, providerLeases := range leasesByProvider {
			var p provider.SecretProvider
			if os.Getenv("ENV_LEASE_TEST") == "1" {
				p = &provider.MockProvider{}
			} else {
				// This assumes all leases for a provider share the same account config.
				p = &provider.OnePasswordCLI{Account: providerLeases[0].OpAccount}
			}

			slog.Info("Fetching secrets", "provider", providerName, "count", len(providerLeases))
			secrets, providerErrors := p.FetchLeases(providerLeases)
			if len(providerErrors) > 0 {
				for _, pe := range providerErrors {
					errs = append(errs, grantError{Source: pe.Lease.Source, Err: pe.Err})
				}
				if !continueOnError {
					return &GrantErrors{errs: errs}
				}
			}
			slog.Info("Fetched secrets", "provider", providerName, "count", len(secrets))

			for variable, secretVal := range secrets {
				var l config.Lease
				for _, lease := range providerLeases {
					if lease.Variable == variable {
						l = lease
						break
					}
				}
				finalLeases, sc, err := processSingleLease(cmd, l, secretVal, cfg.Root, absConfigFile, interactive, &errs, continueOnError)
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
		}

		if len(errs) > 0 {
			return &GrantErrors{errs: errs}
		}

		override, _ := cmd.Flags().GetBool("override")
		req := ipc.GrantRequest{
			Command:    "grant",
			Leases:     leases,
			Override:   override,
			ConfigFile: absConfigFile,
		}
		// If in test mode, don't try to send to the daemon.
		if os.Getenv("ENV_LEASE_TEST") == "1" {
			fmt.Fprintln(os.Stderr, "Grant request (test mode) processed successfully.")
			if len(errs) > 0 {
				return &GrantErrors{errs: errs}
			}
			return nil
		}

		client := newClient()
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
func processLease(cmd *cobra.Command, l config.Lease, secretVal, projectRoot, configFile string) ([]ipc.Lease, []string, error) {
	var shellCommands []string
	var leases []ipc.Lease
	var absDest string
	var err error

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
		absDest = filepath.Join(projectRoot, "<shell>")
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
		absDest, err = fileutil.ExpandPath(l.Destination)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to expand path for %s: %w", l.Destination, err)
		}
		if !filepath.IsAbs(absDest) {
			absDest = filepath.Join(projectRoot, absDest)
		}
	}

	leases = append(leases, ipc.Lease{
		Source:       l.Source,
		Destination:  absDest,
		Duration:     l.Duration,
		LeaseType:    l.LeaseType,
		Variable:     l.Variable,
		Format:       l.Format,
		Transform:    l.Transform,
		FileMode:     l.FileMode,
		ParentSource: l.ParentSource,
		ConfigFile:   configFile,
	})
	return leases, shellCommands, nil
}

func init() {
	grantCmd.Flags().Bool("override", false, "Override existing values in destination files.")
	grantCmd.Flags().Bool("continue-on-error", false, "Continue granting leases even if one fails.")
	grantCmd.Flags().Bool("no-direnv", false, "Do not automatically run 'direnv allow'.")
	grantCmd.Flags().StringP("config", "c", "env-lease.toml", "Path to config file.")
	grantCmd.Flags().String("local-config", "", "Path to local override config file.")
	grantCmd.Flags().BoolP("interactive", "i", false, "Prompt for confirmation before granting each lease.")
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
func interactiveGrant(cmd *cobra.Command, cfg *config.Config, absConfigFile string) error {
	// TODO: The current implementation of interactiveGrant is incorrect. It uses an
	// interleaved, single-loop approach instead of the correct, multi-phase
	// workflow designed and documented in docs/feature_description_interactive_grant.md
	// and the package-level comments.
	//
	// The function body needs to be refactored to follow these distinct phases:
	//
	// 1. PHASE 1: ROUND 1 - APPROVE SOURCES
	//    - Iterate through ALL leases in cfg.Lease first.
	//    - Generate a descriptive prompt for each, including transformation details
	//      for `explode` leases.
	//    - Collect all top-level user approvals ('y'/'n'/'a'/'d') without fetching
	//      any secrets.
	//
	// 2. PHASE 2: FETCH SECRETS
	//    - After Round 1 is complete, gather all the approved sources.
	//    - Group approved `op://` leases by account and `op+file://` leases by
	//      their unique URI.
	//    - Execute all secret lookups in parallel:
	//      - One batched `op` call per `op_account`.
	//      - One individual `op` call per unique `op+file://` URI.
	//    - Cache the content of fetched `op+file://` sources to avoid re-fetching
	//      the same file.
	//
	// 3. PHASE 3: ROUND 2 - APPROVE INDIVIDUAL SECRETS (OPTIONAL)
	//    - After the secrets have been fetched, iterate through the leases that
	//      were approved in Round 1 AND have an `explode` transform.
	//    - For each, prompt the user to approve the individual key-value pairs
	//      that resulted from the transformation.
	//
	// 4. PHASE 4: GRANT LEASES
	//    - Collect all fully-approved leases (simple leases from Round 1 and
	//      sub-leases from Round 2) into a final list.
	//    - Send the single, complete list to the daemon for activation.

	var options []string
	leaseMap := make(map[string]config.Lease)

	for _, l := range cfg.Lease {
		isExplode := false
		for _, t := range l.Transform {
			if strings.HasPrefix(strings.TrimSpace(t), "explode") {
				isExplode = true
				break
			}
		}

		slog.Debug("Categorizing lease for initial prompt", "source", l.Source, "is_explode", isExplode)
		var key string
		if isExplode {
			key = fmt.Sprintf("leases from '%s'%s", l.Source, getTransformSummary(l.Transform))
		} else if l.Variable != "" {
			key = fmt.Sprintf("'%s'", l.Variable)
		} else {
			key = fmt.Sprintf("'%s'", l.Source)
		}
		options = append(options, key)
		leaseMap[key] = l
	}

	var selectedLeaseKeys []string
	for _, opt := range options {
		if confirm(fmt.Sprintf("Grant %s?", opt)) {
			selectedLeaseKeys = append(selectedLeaseKeys, opt)
		}
	}

	if len(selectedLeaseKeys) == 0 {
		fmt.Fprintln(os.Stderr, "No leases selected.")
		return nil
	}

	var selectedLeases []config.Lease
	for _, key := range selectedLeaseKeys {
		selectedLeases = append(selectedLeases, leaseMap[key])
	}
	slog.Debug("Selected leases", "leases", selectedLeases)

	continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
	var errs []grantError
	finalLeases := make([]ipc.Lease, 0)
	approvedShellCommands := make([]string, 0)

	for _, l := range selectedLeases {
		slog.Debug("Processing selected lease", "source", l.Source)
		processed, sc, err := processSingleLease(cmd, l, "", cfg.Root, absConfigFile, true, &errs, continueOnError)
		if err != nil {
			if !continueOnError {
				return err
			}
			continue
		}
		finalLeases = append(finalLeases, processed...)
		approvedShellCommands = append(approvedShellCommands, sc...)
	}

	if len(errs) > 0 {
		return &GrantErrors{errs: errs}
	}

	// 5. Send grant request to daemon
	override, _ := cmd.Flags().GetBool("override")
	req := ipc.GrantRequest{
		Command:    "grant",
		Leases:     finalLeases,
		Override:   override,
		ConfigFile: absConfigFile,
	}
	client := newClient()
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
	for _, l := range finalLeases {
		if filepath.Base(l.Destination) == ".envrc" {
			HandleDirenv(noDirenv, os.Stderr)
			break
		}
	}

	if shellMode {
		fmt.Fprintln(os.Stderr, "# When using shell lease types run this command like `eval $(env-lease grant)`")
		for _, cmd := range approvedShellCommands {
			fmt.Println(cmd)
		}
	}

	fmt.Fprintln(os.Stderr, "Grant request sent successfully.")
	return nil
}

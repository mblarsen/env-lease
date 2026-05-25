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
	"os"
	"path/filepath"

	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/fileutil"
	"github.com/mblarsen/env-lease/internal/grantflow"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/mblarsen/env-lease/internal/lease"
	"github.com/spf13/cobra"
)

var shellMode bool

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

		continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
		override, _ := cmd.Flags().GetBool("override")
		noDirenv, _ := cmd.Flags().GetBool("no-direnv")

		flow := grantflow.Flow{
			Confirm: confirm,
			Notice: func(message string) {
				fmt.Fprintln(os.Stderr, message)
			},
			Materialize: func(l lease.Lease, secret string) (grantflow.Materialized, error) {
				leases, shellCommands, err := processLease(cmd, l, secret, leaseSet.Root, leaseSet.ConfigFile)
				return grantflow.Materialized{Leases: leases, ShellCommands: shellCommands}, err
			},
		}
		result, err := flow.Run(leaseSet, grantflow.Options{
			Interactive:     interactive,
			ContinueOnError: continueOnError,
			Append:          appendMode,
			Override:        override,
		})
		if err != nil {
			return err
		}
		if result.Noop {
			return nil
		}

		// If in test mode, don't try to send to the daemon.
		if os.Getenv("ENV_LEASE_TEST") == "1" {
			fmt.Fprintln(os.Stderr, "Grant request (test mode) processed successfully.")
			return nil
		}

		if client != nil {
			var resp ipc.GrantResponse
			if err := client.Send(result.Request, &resp); err != nil {
				handleClientError(err)
			}
			for _, msg := range resp.Messages {
				fmt.Fprintln(os.Stderr, msg)
			}
		} else {
			fmt.Fprintln(os.Stderr, "Grant request processed in test mode.")
		}

		if result.NeedsDirenv() {
			HandleDirenv(noDirenv, os.Stderr)
		}

		if shellMode {
			fmt.Fprintln(os.Stderr, "# When using shell lease types run this command like `eval $(env-lease grant)`")
			for _, cmd := range result.ShellCommands {
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
//   - l: The normalized lease object containing the lease details.
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

package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mblarsen/env-lease/internal/config"
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

var grantCmd = &cobra.Command{
	Use:   "grant",
	Short: "Grant all leases defined in env-lease.toml.",
	Long:  `Grant all leases defined in env-lease.toml.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		resetConfirmState()
		var (
			cfg           *config.Config
			err           error
			absConfigFile string
		)

		interactive, _ := cmd.Flags().GetBool("interactive")

		// Check if stdout is a terminal
		stat, _ := os.Stdout.Stat()
		isPiped := (stat.Mode() & os.ModeCharDevice) == 0

		if isPiped && interactive && os.Getenv("ENV_LEASE_TEST") != "1" {
			return fmt.Errorf("interactive mode is not supported when piping output (e.g., inside 'eval $(...)')\n" +
				"Please run 'eval $(env-lease grant)' without the interactive flag.")
		}

		configFile, _ := cmd.Flags().GetString("config")
		cfg, err = config.Load(configFile)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		for _, l := range cfg.Lease {
			if l.LeaseType == "shell" {
				shellMode = true
				break
			}
		}

		absConfigFile, err = filepath.Abs(configFile)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for %s: %w", configFile, err)
		}

		continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
		var errs []grantError
		var shellCommands []string
		var p provider.SecretProvider
		leases := make([]ipc.Lease, 0, len(cfg.Lease))
		var finalLeases []ipc.Lease
		var sc []string

		for _, l := range cfg.Lease {
			// Provider setup
			if os.Getenv("ENV_LEASE_TEST") == "1" {
				p = &provider.MockProvider{}
			} else {
				p = &provider.OnePasswordCLI{Account: l.OpAccount}
			}

			// Duration validation
			duration, err := time.ParseDuration(l.Duration)
			if err != nil {
				errs = append(errs, grantError{Source: l.Source, Err: fmt.Errorf("invalid duration '%s': %w", l.Duration, err)})
				if !continueOnError {
					break
				}
				continue
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
						errs = append(errs, grantError{Source: l.Source, Err: fmt.Errorf("lease for '%s' has no format specified", l.Destination)})
						if !continueOnError {
							break
						}
						continue
					}
				}
			}

			// Handle result: could be a single string or exploded data
			var approvedLeases []ipc.Lease
			var approvedShellCommands []string

			// Pre-determine the prompt string
			isExplode := false
			for _, t := range l.Transform {
				if strings.HasPrefix(t, "explode") {
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

			if !interactive || confirm(prompt) {
				// Fetch secret
				slog.Info("Fetching secret", "source", l.Source)
				secretVal, err := p.Fetch(l.Source)
				if err != nil {
					errs = append(errs, grantError{Source: l.Source, Err: fmt.Errorf("failed to fetch secret: %w", err)})
					if !continueOnError {
						break
					}
					continue
				}
				slog.Info("Fetched secret", "source", l.Source)
				// Run transform pipeline
				var transformResult interface{} = secretVal
				if len(l.Transform) > 0 {
					pipeline, err := transform.NewPipeline(l.Transform)
					if err != nil {
						errs = append(errs, grantError{Source: l.Source, Err: fmt.Errorf("failed to create transform pipeline: %w", err)})
						if !continueOnError {
							break
						}
						continue
					}
					transformResult, err = pipeline.Run(secretVal)
					if err != nil {
						errs = append(errs, grantError{Source: l.Source, Err: fmt.Errorf("failed to transform secret: %w", err)})
						if !continueOnError {
							break
						}
						continue
					}
				}

				switch result := transformResult.(type) {
				case string:
					// SINGLE LEASE CASE
					finalLeases, sc, err = processLease(cmd, l, result, absConfigFile)
					if err != nil {
						errs = append(errs, grantError{Source: l.Source, Err: err})
						if !continueOnError {
							break
						}
						continue
					}
					approvedLeases = append(approvedLeases, finalLeases...)
					approvedShellCommands = append(approvedShellCommands, sc...)

				case transform.ExplodedData:
					// EXPLODED LEASE CASE
					if l.LeaseType == "file" {
						errs = append(errs, grantError{Source: l.Source, Err: fmt.Errorf("'explode' transform cannot be used with lease_type 'file'")})
						if !continueOnError {
							break
						}
						continue
					}

					// Add a parent/container lease for the status command to find
					parentLeaseConfig := l
					parentLeaseConfig.Variable = "" // No single variable for the parent
					parentLeases, _, err := processLease(cmd, parentLeaseConfig, "", absConfigFile)
					if err != nil {
						errs = append(errs, grantError{Source: l.Source, Err: err})
						if !continueOnError {
							break
						}
						continue
					}
					approvedLeases = append(approvedLeases, parentLeases...)
					uniqueParentID := parentLeases[0].Source + "->" + parentLeases[0].Destination

					// Process all the child leases
					for key, value := range result {
						if !interactive || confirm(fmt.Sprintf("Grant lease for '%s'?", key)) {
							explodedLeaseConfig := l
							explodedLeaseConfig.Variable = key
							explodedLeaseConfig.ParentSource = uniqueParentID

							finalLeases, sc, err = processLease(cmd, explodedLeaseConfig, value, absConfigFile)
							if err != nil {
								errs = append(errs, grantError{Source: key, Err: err})
								if !continueOnError {
									break
								}
								continue
							}
							approvedLeases = append(approvedLeases, finalLeases...)
							approvedShellCommands = append(approvedShellCommands, sc...)
						}
					}
				}
			}
			leases = append(leases, approvedLeases...)
			shellCommands = append(shellCommands, approvedShellCommands...)
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
			return nil
		}

		client := newClient()
		var resp ipc.GrantResponse
		if err := client.Send(req, &resp); err != nil {
			handleClientError(err)
		}
		for _, msg := range resp.Messages {
			fmt.Fprintln(os.Stderr, msg)
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

func processLease(cmd *cobra.Command, l config.Lease, secretVal string, configFile string) ([]ipc.Lease, []string, error) {
	var shellCommands []string
	var leases []ipc.Lease
	var absDest string
	var err error

	if l.LeaseType == "shell" {
		if l.Variable != "" {
			shellCommands = append(shellCommands, fmt.Sprintf("export %s=%q", l.Variable, secretVal))
		}
		absDest = filepath.Join(filepath.Dir(configFile), "<shell>")
	} else {
		// For file/env leases, only write if there's a variable.
		// This prevents writing the parent/container lease of an explode.
		if l.Variable != "" {
			override, _ := cmd.Flags().GetBool("override")
			created, err := writeLease(l, secretVal, override)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to write lease: %w", err)
			}
			if created {
				fmt.Fprintf(os.Stderr, "Created file: %s\n", l.Destination)
			}
		}
		absDest, err = filepath.Abs(l.Destination)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get absolute path for %s: %w", l.Destination, err)
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
	grantCmd.Flags().BoolP("interactive", "i", false, "Prompt for confirmation before granting each lease.")
	rootCmd.AddCommand(grantCmd)
}

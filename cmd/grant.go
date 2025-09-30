package cmd

import (
	"fmt"
	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/mblarsen/env-lease/internal/provider"
	"github.com/mblarsen/env-lease/internal/transform"
	"github.com/spf13/cobra"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var shellMode bool

func printInfo(format string, a ...interface{}) {
	var writer io.Writer = os.Stdout
	if shellMode {
		writer = os.Stderr
	}
	fmt.Fprintf(writer, format, a...)
}

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
	Long:  `Grant all leases defined in env--lease.toml.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		configFile, _ := cmd.Flags().GetString("config")
		cfg, err := config.Load(configFile)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		for _, l := range cfg.Lease {
			if l.LeaseType == "shell" {
				shellMode = true
				break
			}
		}

		absConfigFile, err := filepath.Abs(configFile)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for %s: %w", configFile, err)
		}

		continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
		var errs []grantError
		var shellCommands []string

		var p provider.SecretProvider
		leases := make([]ipc.Lease, len(cfg.Lease))
		for i, l := range cfg.Lease {
			if os.Getenv("ENV_LEASE_TEST") == "1" {
				p = &provider.MockProvider{}
			} else {
				p = &provider.OnePasswordCLI{
					Account: l.OpAccount,
				}
			}
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

			// Set default format if not provided
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

			if len(l.Transform) > 0 {
				pipeline, err := transform.NewPipeline(l.Transform)
				if err != nil {
					errs = append(errs, grantError{Source: l.Source, Err: fmt.Errorf("failed to create transform pipeline: %w", err)})
					if !continueOnError {
						break
					}
					continue
				}
				secretVal, err = pipeline.Run(secretVal)
				if err != nil {
					errs = append(errs, grantError{Source: l.Source, Err: fmt.Errorf("failed to transform secret: %w", err)})
					if !continueOnError {
						break
					}
					continue
				}
			}

			if l.LeaseType == "shell" {
				shellCommands = append(shellCommands, fmt.Sprintf("export %s=%q", l.Variable, secretVal))
			} else {
				override, _ := cmd.Flags().GetBool("override")
				created, err := writeLease(l, secretVal, override)
				if err != nil {
					errs = append(errs, grantError{Source: l.Source, Err: fmt.Errorf("failed to write lease: %w", err)})
					if !continueOnError {
						break
					}
					continue
				}
				if created {
					printInfo("Created file: %s\n", l.Destination)
				}
			}
			clearString(secretVal)

			absDest, err := filepath.Abs(l.Destination)
			if err != nil {
				errs = append(errs, grantError{Source: l.Source, Err: fmt.Errorf("failed to get absolute path for %s: %w", l.Destination, err)})
				if !continueOnError {
					break
				}
				continue
			}

			leases[i] = ipc.Lease{
				Source:      l.Source,
				Destination: absDest,
				Duration:    l.Duration,
				LeaseType:   l.LeaseType,
				Variable:    l.Variable,
				Format:      l.Format,
				Transform:   l.Transform,
				FileMode:    l.FileMode,
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
			printInfo("Grant request (test mode) processed successfully.\n")
			return nil
		}

		client := newClient()
		var resp ipc.GrantResponse
		if err := client.Send(req, &resp); err != nil {
			handleClientError(err)
		}
		for _, msg := range resp.Messages {
			printInfo("%s\n", msg)
		}

		noDirenv, _ := cmd.Flags().GetBool("no-direnv")
		for _, l := range leases {
			if filepath.Base(l.Destination) == ".envrc" {
				var writer io.Writer = os.Stdout
				if shellMode {
					writer = os.Stderr
				}
				HandleDirenv(noDirenv, writer)
				break
			}
		}

		if shellMode {
			for _, cmd := range shellCommands {
				fmt.Println(cmd)
			}
		}

		printInfo("Grant request sent successfully.\n")
		return nil
	},
}
		
		
		
		
		func init() {
		    grantCmd.Flags().Bool("override", false, "Override existing values in destination files.")
		    grantCmd.Flags().Bool("continue-on-error", false, "Continue granting leases even if one fails.")
		    grantCmd.Flags().Bool("no-direnv", false, "Do not automatically run 'direnv allow'.")
		    grantCmd.Flags().StringP("config", "c", "env-lease.toml", "Path to config file.")
		    rootCmd.AddCommand(grantCmd)
		}

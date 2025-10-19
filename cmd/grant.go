package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
		if l.LeaseType == "file" || l.Variable != "" {
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

func interactiveGrant(cmd *cobra.Command, cfg *config.Config, absConfigFile string) error {
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

	// Re-categorize based on selection
	var selectedFileLeases, selectedExplodeLeases, selectedBatchableLeases []config.Lease
	slog.Debug("Re-categorizing selected leases", "count", len(selectedLeases))
	for _, l := range selectedLeases {
		isExplode := false
		for _, t := range l.Transform {
			if strings.HasPrefix(strings.TrimSpace(t), "explode") {
				isExplode = true
				break
			}
		}

		// If the source is a file, it's always a file lease, regardless of transforms.
		if strings.HasPrefix(l.Source, "op+file://") {
			selectedFileLeases = append(selectedFileLeases, l)
			slog.Debug("Categorized lease as File", "source", l.Source)
		} else if isExplode {
			selectedExplodeLeases = append(selectedExplodeLeases, l)
			slog.Debug("Categorized lease as Explode", "source", l.Source)
		} else {
			selectedBatchableLeases = append(selectedBatchableLeases, l)
			slog.Debug("Categorized lease as Batchable", "source", l.Source)
		}
	}

	continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
	var errs []grantError
	secrets := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// 1. Build helper map of all leases by source URI to handle de-duplication
	allLeasesForFetching := append(selectedFileLeases, selectedBatchableLeases...)
	allLeasesForFetching = append(allLeasesForFetching, selectedExplodeLeases...)

	sourceToLeases := make(map[string][]config.Lease)
	uniqueLeasesBySource := make(map[string]config.Lease)
	for _, l := range allLeasesForFetching {
		sourceToLeases[l.Source] = append(sourceToLeases[l.Source], l)
		if _, exists := uniqueLeasesBySource[l.Source]; !exists {
			uniqueLeasesBySource[l.Source] = l
		}
	}

	// 2. Categorize unique leases for fetching strategy (individual vs. batch)
	var individualFetchLeases []config.Lease
	batchableLeasesByAccount := make(map[string][]config.Lease)
	for _, l := range uniqueLeasesBySource {
		if strings.HasPrefix(l.Source, "op+file://") {
			individualFetchLeases = append(individualFetchLeases, l)
		} else {
			batchableLeasesByAccount[l.OpAccount] = append(batchableLeasesByAccount[l.OpAccount], l)
		}
	}

	// 3. Execute fetches concurrently, de-duplicated by source URI

	// 3a. Individual fetches for file-based sources
	slog.Debug("Starting individual fetch for unique file-source leases", "count", len(individualFetchLeases))
	for _, l := range individualFetchLeases {
		wg.Add(1)
		go func(l config.Lease) {
			defer wg.Done()
			slog.Info("Fetching secret individually", "source", l.Source)
			var p provider.SecretProvider
			if os.Getenv("ENV_LEASE_TEST") == "1" {
				p = &provider.MockProvider{}
			} else {
				p = &provider.OnePasswordCLI{Account: l.OpAccount}
			}
			secretVal, err := p.Fetch(l.Source)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, grantError{Source: l.Source, Err: err})
				return
			}

			// Populate secrets map for all leases sharing this source
			for _, relatedLease := range sourceToLeases[l.Source] {
				if relatedLease.Variable != "" {
					secrets[relatedLease.Variable] = secretVal
				} else {
					secrets[relatedLease.Source] = secretVal
				}
			}
			slog.Info("Fetched secret individually", "source", l.Source)
		}(l)
	}

	// 3b. Batch fetches for other sources, grouped by account
	slog.Debug("Starting batch fetch for unique op-source leases", "accounts", len(batchableLeasesByAccount))
	for account, accountLeases := range batchableLeasesByAccount {
		wg.Add(1)
		go func(account string, accountLeases []config.Lease) {
			defer wg.Done()
			var p provider.SecretProvider
			if os.Getenv("ENV_LEASE_TEST") == "1" {
				p = &provider.MockProvider{}
			} else {
				p = &provider.OnePasswordCLI{Account: account}
			}

			slog.Info("Batch fetching secrets for account", "account", account, "count", len(accountLeases))
			fetchedSecrets, providerErrors := p.FetchLeases(accountLeases)

			mu.Lock()
			defer mu.Unlock()
			if len(providerErrors) > 0 {
				for _, pe := range providerErrors {
					errs = append(errs, grantError{Source: pe.Lease.Source, Err: pe.Err})
				}
			}

			// Map batch results back to all leases that share the source
			for _, fetchedLease := range accountLeases {
				var secretVal string
				var ok bool
				if fetchedLease.Variable != "" {
					secretVal, ok = fetchedSecrets[fetchedLease.Variable]
				} else {
					secretVal, ok = fetchedSecrets[fetchedLease.Source]
				}

				if ok {
					for _, relatedLease := range sourceToLeases[fetchedLease.Source] {
						if relatedLease.Variable != "" {
							secrets[relatedLease.Variable] = secretVal
						} else {
							secrets[relatedLease.Source] = secretVal
						}
					}
				}
			}
			slog.Info("Batch fetched secrets for account", "account", account, "count", len(fetchedSecrets))
		}(account, accountLeases)
	}

	wg.Wait()

	if len(errs) > 0 && !continueOnError {
		return &GrantErrors{errs: errs}
	}

	finalLeases := make([]ipc.Lease, 0)
	approvedShellCommands := make([]string, 0)

	for _, l := range selectedExplodeLeases {
		secretVal, ok := secrets[l.Source]
		if !ok {
			continue
		}

		pipeline, err := transform.NewPipeline(l.Transform)
		if err != nil {
			return fmt.Errorf("failed to create transform pipeline for %s: %w", l.Source, err)
		}
		transformResult, err := pipeline.Run(secretVal)
		if err != nil {
			return fmt.Errorf("failed to transform secret for %s: %w", l.Source, err)
		}

		explodedData, ok := transformResult.(transform.ExplodedData)
		if !ok {
			return fmt.Errorf("expected exploded data from transform for %s, but got something else", l.Source)
		}

		var explodedOptions []string
		for k := range explodedData {
			explodedOptions = append(explodedOptions, k)
		}

		var selectedKeys []string
		fmt.Fprintf(os.Stderr, "Granting sub-leases from '%s'%s:\n", l.Source, getTransformSummary(l.Transform))
		for _, opt := range explodedOptions {
			if confirm(fmt.Sprintf("Grant lease for '%s'?", opt)) {
				selectedKeys = append(selectedKeys, opt)
			}
		}

		parentLeaseConfig := l
		parentLeaseConfig.Variable = "" // No single variable for the parent
		parentLeases, _, err := processLease(cmd, parentLeaseConfig, "", cfg.Root, absConfigFile)
		if err != nil {
			return err
		}
		finalLeases = append(finalLeases, parentLeases...)
		uniqueParentID := parentLeases[0].Source + "->" + parentLeases[0].Destination

		for _, key := range selectedKeys {
			value := explodedData[key]
			explodedLeaseConfig := l
			explodedLeaseConfig.Variable = key
			explodedLeaseConfig.ParentSource = uniqueParentID

			processed, sc, err := processLease(cmd, explodedLeaseConfig, value, cfg.Root, absConfigFile)
			if err != nil {
				errs = append(errs, grantError{Source: key, Err: err})
				if !continueOnError {
					return &GrantErrors{errs: errs}
				}
				continue
			}
			finalLeases = append(finalLeases, processed...)
			approvedShellCommands = append(approvedShellCommands, sc...)
		}
	}

	// 4. Process non-exploded leases
	for _, l := range selectedBatchableLeases {
		secretVal, ok := secrets[l.Variable]
		if !ok {
			slog.Debug("Secret not found in fetched map for batchable lease", "variable", l.Variable, "source", l.Source)
			continue
		}
		processed, sc, err := processSingleLease(cmd, l, secretVal, cfg.Root, absConfigFile, false, &errs, continueOnError)
		if err != nil {
			errs = append(errs, grantError{Source: l.Source, Err: err})
			if !continueOnError {
				return &GrantErrors{errs: errs}
			}
			continue
		}
		finalLeases = append(finalLeases, processed...)
		approvedShellCommands = append(approvedShellCommands, sc...)
	}

	for _, l := range selectedFileLeases {
		secretVal, ok := secrets[l.Source]
		if !ok {
			continue
		}
		processed, sc, err := processSingleLease(cmd, l, secretVal, cfg.Root, absConfigFile, true, &errs, continueOnError)
		if err != nil {
			errs = append(errs, grantError{Source: l.Source, Err: err})
			if !continueOnError {
				return &GrantErrors{errs: errs}
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

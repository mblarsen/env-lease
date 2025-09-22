package cmd

import (
	"context"
	"fmt"
	"github.com/mblarsen/env-lease/internal/daemon"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/spf13/cobra"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the env-lease daemon.",
	Long:  `Run the env-lease daemon.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Starting daemon...")

		// Configuration paths
		configDir := filepath.Join(os.Getenv("HOME"), ".config", "env-lease")
		if err := os.MkdirAll(configDir, 0700); err != nil {
			return err
		}
		socketPath := filepath.Join(configDir, "daemon.sock")
		statePath := filepath.Join(configDir, "state.json")
		secretPath := filepath.Join(configDir, "auth.token")

		// Get or create secret
		secret, err := ipc.GetOrCreateSecret(secretPath)
		if err != nil {
			return err
		}

		// Load state
		state, err := daemon.LoadState(statePath)
		if err != nil {
			return err
		}

		// Set up dependencies
		clock := &daemon.RealClock{}
		revoker := &daemon.FileRevoker{}
		ipcServer, err := ipc.NewServer(socketPath, secret)
		if err != nil {
			return err
		}

		// Create and run daemon
		d := daemon.NewDaemon(state, statePath, clock, ipcServer, revoker)
		
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		go func() {
			<-ctx.Done()
			fmt.Println("Daemon shutting down...")
			state.SaveState(statePath)
		}()

		return d.Run(ctx)
	},
}

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Cleanup orphaned leases.",
	Long:  `Cleanup orphaned leases.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Cleaning up orphaned leases...")
		// This is where the cleanup will be triggered
		return nil
	},
}

func init() {
	daemonCmd.AddCommand(runCmd)
	daemonCmd.AddCommand(cleanupCmd)
}

package daemon

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/fileutil"
	"log/slog"
	"os"
	"strings"
)

// Revoker is an interface for revoking leases.
type Revoker interface {
	Revoke(lease *config.Lease) error
}

// FileRevoker is a revoker that modifies the filesystem.
type FileRevoker struct{}

// Revoke revokes a lease by either deleting a file or clearing a variable in a file.
func (r *FileRevoker) Revoke(lease *config.Lease) error {
	switch lease.LeaseType {
	case "file":
		if _, err := os.Stat(lease.Destination); os.IsNotExist(err) {
			slog.Info("Lease target file not found, proceeding with revocation", "path", lease.Destination)
			return nil // File is already gone, consider it revoked.
		}
		slog.Debug("Revoking file lease", "path", lease.Destination)
		return os.Remove(lease.Destination)
	case "env":
		slog.Debug("Revoking env lease", "path", lease.Destination, "variable", lease.Variable)
		return r.clearEnvVar(lease.Destination, lease.Variable)
	default:
		return fmt.Errorf("unknown lease type: %s", lease.LeaseType)
	}
}

func (r *FileRevoker) clearEnvVar(path, keyToRevoke string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File is already gone, consider it revoked.
		}
		return err
	}
	defer f.Close()

	var out bytes.Buffer
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		parts := strings.SplitN(line, "=", 2)
		if len(parts) < 2 {
			out.WriteString(line + "\n")
			continue
		}

		keyPart := strings.TrimSpace(parts[0])
		keyPart = strings.TrimPrefix(keyPart, "export ")

		if keyPart == keyToRevoke {
			originalKeyPart := parts[0]
								out.WriteString(originalKeyPart + "=\n")		} else {
			out.WriteString(line + "\n")
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	info, err := f.Stat()
	if err != nil {
		return err
	}

	_, err = fileutil.AtomicWriteFile(path, out.Bytes(), info.Mode())
	return err
}

package daemon

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/mblarsen/env-lease/internal/fileutil"
	"os"
	"strings"
)

// Revoker is an interface for revoking leases.
type Revoker interface {
	Revoke(lease Lease) error
}

// FileRevoker is a revoker that modifies the filesystem.
type FileRevoker struct{}

// Revoke revokes a lease by either deleting a file or clearing a variable in a file.
func (r *FileRevoker) Revoke(lease Lease) error {
	switch lease.LeaseType {
	case "file":
		return os.Remove(lease.Destination)
	case "env":
		return r.clearEnvVar(lease.Destination, lease.Variable)
	default:
		return fmt.Errorf("unknown lease type: %s", lease.LeaseType)
	}
}

func (r *FileRevoker) clearEnvVar(path, key string) error {
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
		if strings.HasPrefix(line, key+"=") || strings.HasPrefix(line, "export "+key+"=") {
			// Clear the value
			parts := strings.SplitN(line, "=", 2)
			out.WriteString(parts[0] + "=\n")
		} else {
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

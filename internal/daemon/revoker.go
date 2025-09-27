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
			comment := ""
			if commentIndex := strings.Index(line, "#"); commentIndex > strings.Index(line, "=") {
				comment = " " + strings.TrimSpace(line[commentIndex:])
			}
			out.WriteString(originalKeyPart + "=" + comment + "\n")
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

// Package destination owns Grant and Revoke effects for lease destinations.
package destination

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/mblarsen/env-lease/internal/fileutil"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/mblarsen/env-lease/internal/lease"
)

// Materializer applies granted Secrets to their configured Destinations.
type Materializer struct {
	ProjectRoot      string
	ConfigFile       string
	Override         bool
	AllowOutsideRoot bool
}

// Materialized describes the observable result of applying a Secret to a Destination.
type Materialized struct {
	Leases        []ipc.Lease
	ShellCommands []string
	Notices       []string
}

// Revoker reverses granted Secrets from their Destinations.
type Revoker struct{}

// Revoked describes the observable result of revoking a Destination.
type Revoked struct {
	ShellCommands []string
}

// Materialize applies secret material for one Lease and returns the Lease data
// that should be registered with the Daemon.
func (m Materializer) Materialize(l lease.Lease, secret string) (Materialized, error) {
	formatted, err := l.WithDefaultFormat()
	if err != nil {
		return Materialized{}, err
	}
	l = formatted

	if err := m.validateDestination(l); err != nil {
		return Materialized{}, err
	}

	var result Materialized
	if l.LeaseType == lease.TypeShell {
		if l.Variable != "" {
			result.ShellCommands = append(result.ShellCommands, fmt.Sprintf("export %s=%q", l.Variable, secret))
		}
	} else if shouldWrite(l) {
		created, err := write(l, secret, m.Override)
		if err != nil {
			return Materialized{}, fmt.Errorf("failed to write lease: %w", err)
		}
		if created {
			result.Notices = append(result.Notices, fmt.Sprintf("Created file: %s", l.Destination))
		}
	}

	ipcLease := l.ToIPC()
	ipcLease.ConfigFile = m.ConfigFile
	result.Leases = append(result.Leases, ipcLease)
	return result, nil
}

func (m Materializer) validateDestination(l lease.Lease) error {
	if l.LeaseType != lease.TypeFile || m.AllowOutsideRoot {
		return nil
	}

	pathToCheck := l.RawDestination
	if pathToCheck == "" {
		pathToCheck = l.Destination
	}
	isInside, err := fileutil.IsPathInsideRoot(m.ProjectRoot, pathToCheck)
	if err != nil {
		return fmt.Errorf("failed to validate destination path: %w", err)
	}
	if !isInside {
		return fmt.Errorf("destination path '%s' is outside the project root. Use --destination-outside-root to override", l.Destination)
	}
	return nil
}

func shouldWrite(l lease.Lease) bool {
	return l.LeaseType == lease.TypeFile || (l.LeaseType == lease.TypeEnv && l.Variable != "")
}

func write(l lease.Lease, secret string, override bool) (bool, error) {
	fileMode, err := l.ParseFileMode(0600)
	if err != nil {
		return false, err
	}

	switch l.LeaseType {
	case lease.TypeEnv:
		return writeEnvFile(l.Destination, l.Variable, secret, l.Format, override, fileMode)
	case lease.TypeFile:
		return writeFile(l.Destination, secret, fileMode)
	case lease.TypeShell:
		return false, fmt.Errorf("the 'shell' lease type should not be handled by write")
	default:
		return false, fmt.Errorf("unknown lease type: %s", l.LeaseType)
	}
}

func writeFile(path, value string, fileMode os.FileMode) (bool, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		_, err := fileutil.AtomicWriteFile(path, []byte(value), fileMode)
		return true, err
	}

	_, err := fileutil.AtomicWriteFile(path, []byte(value), fileMode)
	return false, err
}

func writeEnvFile(path, key, value, format string, override bool, fileMode os.FileMode) (bool, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		content := fmt.Sprintf(format+"\n", key, value)
		_, err := fileutil.AtomicWriteFile(path, []byte(content), fileMode)
		return true, err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("failed to read existing file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	keyExists := false
	prefix := strings.Split(format, "%")[0] + key + "="
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			keyExists = true
			newLine := fmt.Sprintf(format, key, value)
			if line == newLine {
				return false, nil
			}

			parts := strings.SplitN(line, "=", 2)
			hasValue := len(parts) > 1 && strings.Trim(parts[1], `"`) != ""
			if hasValue && !override {
				return false, fmt.Errorf("variable '%s' already has a value; use --override to replace it", key)
			}

			lines[i] = newLine
			break
		}
	}

	if !keyExists {
		lines = append(lines, fmt.Sprintf(format, key, value))
	}

	var nonEmptyLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}

	output := strings.Join(nonEmptyLines, "\n") + "\n"
	_, err = fileutil.AtomicWriteFile(path, []byte(output), fileMode)
	return false, err
}

// Revoke removes or clears a granted Secret from its Destination.
func (r Revoker) Revoke(l *lease.Lease) (Revoked, error) {
	if l == nil {
		return Revoked{}, nil
	}

	switch l.LeaseType {
	case lease.TypeShell:
		if l.Variable == "" {
			return Revoked{}, nil
		}
		return Revoked{ShellCommands: []string{fmt.Sprintf("unset %s", l.Variable)}}, nil
	case lease.TypeFile:
		if _, err := os.Stat(l.Destination); os.IsNotExist(err) {
			return Revoked{}, nil
		}
		return Revoked{}, os.Remove(l.Destination)
	case lease.TypeEnv:
		if l.Variable == "" {
			return Revoked{}, nil
		}
		return Revoked{}, clearEnvVar(l.Destination, l.Variable)
	default:
		return Revoked{}, fmt.Errorf("unknown lease type: %s", l.LeaseType)
	}
}

func clearEnvVar(path, keyToRevoke string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
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

package fileutil

import (
	"os"
	"path/filepath"
)

// AtomicWriteFile writes data to a file atomically.
func AtomicWriteFile(filename string, data []byte, perm os.FileMode) error {
	dir, name := filepath.Split(filename)
	if dir == "" {
		dir = "."
	}

	tmpfile, err := os.CreateTemp(dir, name+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmpfile.Name()) // Clean up the temp file if something goes wrong

	if _, err := tmpfile.Write(data); err != nil {
		tmpfile.Close()
		return err
	}

	if err := tmpfile.Close(); err != nil {
		return err
	}

	if err := os.Chmod(tmpfile.Name(), perm); err != nil {
		return err
	}

	return os.Rename(tmpfile.Name(), filename)
}

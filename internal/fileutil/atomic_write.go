package fileutil

import (
	"os"
	"path/filepath"
)

// AtomicWriteFile writes data to a file atomically. It returns true if the file was created.
func AtomicWriteFile(filename string, data []byte, perm os.FileMode) (bool, error) {
	_, err := os.Stat(filename)
	created := os.IsNotExist(err)

	dir, name := filepath.Split(filename)
	if dir == "" {
		dir = "."
	}

	tmpfile, err := os.CreateTemp(dir, name+".*.tmp")
	if err != nil {
		return false, err
	}
	defer os.Remove(tmpfile.Name()) // Clean up the temp file if something goes wrong

	if _, err := tmpfile.Write(data); err != nil {
		tmpfile.Close()
		return false, err
	}

	if err := tmpfile.Close(); err != nil {
		return false, err
	}

	if err := os.Chmod(tmpfile.Name(), perm); err != nil {
		return false, err
	}

	if err := os.Rename(tmpfile.Name(), filename); err != nil {
		return false, err
	}
	return created, nil
}

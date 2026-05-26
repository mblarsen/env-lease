package platforminstall

import (
	"os"
	"path/filepath"

	"github.com/mblarsen/env-lease/internal/fileutil"
)

func writeAtomic(file File) error {
	if err := os.MkdirAll(filepath.Dir(file.Path), 0755); err != nil {
		return err
	}
	_, err := fileutil.AtomicWriteFile(file.Path, []byte(file.Body), file.Mode)
	return err
}

func removeIgnoringMissing(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

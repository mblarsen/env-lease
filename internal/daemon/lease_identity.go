package daemon

import (
	"path/filepath"
	"strings"

	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/fileutil"
)

func leaseIdentity(source, destination, variable string) string {
	return source + ";" + destination + ";" + variable
}

func parentLeaseIdentity(source, destination string) string {
	return source + "->" + destination
}

func canonicalLeaseDestination(root string, lease config.Lease) (string, error) {
	if lease.LeaseType == "shell" {
		return filepath.Join(root, "<shell>"), nil
	}

	destination, err := fileutil.ExpandPath(lease.Destination)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(destination) {
		destination = filepath.Join(root, destination)
	} else {
		destination = filepath.Clean(destination)
	}
	return destination, nil
}

func hasExplodeTransform(transforms []string) bool {
	for _, t := range transforms {
		if strings.HasPrefix(strings.TrimSpace(t), "explode") {
			return true
		}
	}
	return false
}

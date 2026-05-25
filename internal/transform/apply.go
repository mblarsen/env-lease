package transform

import (
	"fmt"
	"sort"

	"github.com/mblarsen/env-lease/internal/lease"
)

// Secret is transformed Secret material attached to the Lease it should grant.
type Secret struct {
	Lease lease.Lease
	Value string
}

// Result is the typed output of applying a transform pipeline to one Lease's raw Secret.
type Result struct {
	Parent  *lease.Lease
	Secrets []Secret
}

// IsExploded reports whether the transform output expanded into child Leases.
func (r Result) IsExploded() bool {
	return r.Parent != nil
}

// Apply applies a Lease's transform pipeline to raw Secret material and returns
// the resulting Grant-ready Secret material without materializing it.
func Apply(l lease.Lease, rawSecret string) (Result, error) {
	transformed := any(rawSecret)
	if len(l.Transform) > 0 {
		pipeline, err := NewPipeline(l.Transform)
		if err != nil {
			return Result{}, err
		}
		transformed, err = pipeline.Run(rawSecret)
		if err != nil {
			return Result{}, err
		}
	}

	switch result := transformed.(type) {
	case string:
		return Result{Secrets: []Secret{{Lease: l, Value: result}}}, nil
	case ExplodedData:
		if l.LeaseType == lease.TypeFile {
			return Result{}, fmt.Errorf("'explode' transform cannot be used with lease_type 'file'")
		}
		return explodedResult(l, result), nil
	default:
		return Result{}, fmt.Errorf("transform pipeline must produce a string or exploded data")
	}
}

func explodedResult(l lease.Lease, data ExplodedData) Result {
	parent := l
	parent.Variable = ""
	parent.ParentSource = ""
	parentID := parent.ParentIdentity()

	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	secrets := make([]Secret, 0, len(keys))
	for _, key := range keys {
		child := l
		child.Variable = key
		child.ParentSource = parentID
		secrets = append(secrets, Secret{Lease: child, Value: data[key]})
	}

	return Result{Parent: &parent, Secrets: secrets}
}

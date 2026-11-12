package core

import (
	"fmt"
	"regexp"
)

type Registration struct {
	Manifest Manifest
	New      func() (Application, error)
}
type Registry struct {
	entries []Registration
	byID    map[string]int
}

var validID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)

func copyManifest(m Manifest) Manifest {
	m.Capabilities = append([]Capability(nil), m.Capabilities...)
	return m
}
func NewRegistry(entries ...Registration) (*Registry, error) {
	r := &Registry{byID: make(map[string]int)}
	for _, e := range entries {
		if !validID.MatchString(e.Manifest.ID) || e.Manifest.Title == "" || e.New == nil {
			return nil, fmt.Errorf("无效的应用注册: %q", e.Manifest.ID)
		}
		if _, exists := r.byID[e.Manifest.ID]; exists {
			return nil, fmt.Errorf("应用 ID 重复: %s", e.Manifest.ID)
		}
		e.Manifest = copyManifest(e.Manifest)
		r.byID[e.Manifest.ID] = len(r.entries)
		r.entries = append(r.entries, e)
	}
	return r, nil
}
func (r *Registry) Manifests() []Manifest {
	result := make([]Manifest, 0, len(r.entries))
	for _, e := range r.entries {
		result = append(result, copyManifest(e.Manifest))
	}
	return result
}

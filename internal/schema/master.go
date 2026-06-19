// Package schema concurrently decodes profile files and aggregates their
// metadata entries into a global Master Schema under mutex protection.
// Phase 2 of the 3-phase profile normalization pipeline.
package schema

import (
	"sort"
	"sync"

	"xml2l/internal/graph"
	"xml2l/internal/profile"
)

// MasterSchema holds the global union of all metadata entries discovered
// across all profile files, keyed by XML section tag.
type MasterSchema struct {
	mu      sync.Mutex
	entries map[string]map[string]bool // keyed by XML section tag
}

// NewMasterSchema creates an empty MasterSchema with entries initialized
// for every known section tag from the graph's canonical ordering.
func NewMasterSchema() *MasterSchema {
	ms := &MasterSchema{entries: make(map[string]map[string]bool)}
	for _, sm := range graph.SectionMetaOrder() {
		ms.entries[sm.Tag] = make(map[string]bool)
	}
	return ms
}

// merge incorporates all entries from a single profile's sections into
// the MasterSchema. Thread-safe; acquires its own lock.
func (ms *MasterSchema) merge(prof *profile.Profile) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	for tag, entries := range prof.Sections {
		m, ok := ms.entries[tag]
		if !ok {
			continue
		}
		for _, e := range entries {
			m[e.Name] = true
		}
	}
}

// Add adds a single name to the schema for the given section tag.
// Silently ignores unknown tags. Exported for test convenience.
func (ms *MasterSchema) Add(tag, name string) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if m, ok := ms.entries[tag]; ok {
		m[name] = true
	}
}

// AllNames returns all known metadata names for a given XML section tag.
// It implements graph.MasterSchemaProvider.
func (ms *MasterSchema) AllNames(tag string) []string {
	ms.mu.Lock()
	m, ok := ms.entries[tag]
	if !ok {
		ms.mu.Unlock()
		return nil
	}
	r := make([]string, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	ms.mu.Unlock()
	sort.Strings(r)
	return r
}

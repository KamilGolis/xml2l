// Package schema concurrently decodes profile files and aggregates their
// metadata entries into a global Master Schema under mutex protection.
// Phase 2 of the 3-phase profile normalization pipeline.
package schema

import (
	"sort"
	"sync"

	"xml2l/internal/profile"
)

// MasterSchema holds the global union of all metadata entries discovered
// across all profile files. Maps are Mutex-protected for concurrent fan-in.
type MasterSchema struct {
	mu                      sync.Mutex
	Fields                  map[string]bool
	Classes                 map[string]bool
	Agents                  map[string]bool
	Apps                    map[string]bool
	CategoryGroups          map[string]bool
	CustomMetadataTypes     map[string]bool
	CustomPermissions       map[string]bool
	CustomSettings          map[string]bool
	ExternalDataSources     map[string]bool
	Flows                   map[string]bool
	GenComputingSummaryDefs map[string]bool
	Layouts                 map[string]bool
	LoginFlows              map[string]bool
	Objects                 map[string]bool
	RecordTypes             map[string]bool
	ServicePresenceStatuses map[string]bool
	Tabs                    map[string]bool
	UserPerms               map[string]bool
}

// NewMasterSchema creates an empty MasterSchema with all maps initialized.
func NewMasterSchema() *MasterSchema {
	return &MasterSchema{
		Fields:                  make(map[string]bool),
		Classes:                 make(map[string]bool),
		Agents:                  make(map[string]bool),
		Apps:                    make(map[string]bool),
		CategoryGroups:          make(map[string]bool),
		CustomMetadataTypes:     make(map[string]bool),
		CustomPermissions:       make(map[string]bool),
		CustomSettings:          make(map[string]bool),
		ExternalDataSources:     make(map[string]bool),
		Flows:                   make(map[string]bool),
		GenComputingSummaryDefs: make(map[string]bool),
		Layouts:                 make(map[string]bool),
		LoginFlows:              make(map[string]bool),
		Objects:                 make(map[string]bool),
		RecordTypes:             make(map[string]bool),
		ServicePresenceStatuses: make(map[string]bool),
		Tabs:                    make(map[string]bool),
		UserPerms:               make(map[string]bool),
	}
}

// merge incorporates all entries from a single profile's sections into
// the Master Schema. Must be called while holding mu.
func (ms *MasterSchema) merge(prof *profile.Profile) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	for tag, entries := range prof.Sections {
		switch tag {
		case "classAccesses":
			addAll(ms.Classes, toMap(entries))
		case "agentAccesses":
			addAll(ms.Agents, toMap(entries))
		case "applicationVisibilities":
			addAll(ms.Apps, toMap(entries))
		case "categoryGroupVisibilities":
			addAll(ms.CategoryGroups, toMap(entries))
		case "customMetadataTypeAccesses":
			addAll(ms.CustomMetadataTypes, toMap(entries))
		case "customPermissions":
			addAll(ms.CustomPermissions, toMap(entries))
		case "customSettingAccesses":
			addAll(ms.CustomSettings, toMap(entries))
		case "externalDataSourceAccesses":
			addAll(ms.ExternalDataSources, toMap(entries))
		case "fieldLevelSecurities", "fieldPermissions":
			addAll(ms.Fields, toMap(entries))
		case "flowAccesses":
			addAll(ms.Flows, toMap(entries))
		case "genComputingSummaryDefAccess":
			addAll(ms.GenComputingSummaryDefs, toMap(entries))
		case "layoutAssignments":
			addAll(ms.Layouts, toMap(entries))
		case "loginFlows":
			addAll(ms.LoginFlows, toMap(entries))
		case "objectPermissions":
			addAll(ms.Objects, toMap(entries))
		case "recordTypeVisibilities":
			addAll(ms.RecordTypes, toMap(entries))
		case "servicePresenceStatusAccesses":
			addAll(ms.ServicePresenceStatuses, toMap(entries))
		case "tabVisibilities":
			addAll(ms.Tabs, toMap(entries))
		case "userPermissions":
			addAll(ms.UserPerms, toMap(entries))
		}
	}
}

// addAll inserts all entries from src into dst.
func addAll(dst, src map[string]bool) {
	for k := range src {
		dst[k] = true
	}
}

// toMap converts a slice of MetadataEntry to a map[string]bool keyed by name.
func toMap(entries []profile.MetadataEntry) map[string]bool {
	m := make(map[string]bool, len(entries))
	for _, e := range entries {
		m[e.Name] = true
	}
	return m
}

// AllNames returns all known metadata names for a given XML section tag.
// It implements graph.MasterSchemaProvider.
func (ms *MasterSchema) AllNames(tag string) []string {
	ms.mu.Lock()

	var m map[string]bool
	switch tag {
	case "classAccesses":
		m = ms.Classes
	case "fieldPermissions", "fieldLevelSecurities":
		m = ms.Fields
	case "agentAccesses":
		m = ms.Agents
	case "applicationVisibilities":
		m = ms.Apps
	case "objectPermissions":
		m = ms.Objects
	case "tabVisibilities":
		m = ms.Tabs
	case "userPermissions":
		m = ms.UserPerms
	case "categoryGroupVisibilities":
		m = ms.CategoryGroups
	case "customMetadataTypeAccesses":
		m = ms.CustomMetadataTypes
	case "customPermissions":
		m = ms.CustomPermissions
	case "customSettingAccesses":
		m = ms.CustomSettings
	case "externalDataSourceAccesses":
		m = ms.ExternalDataSources
	case "flowAccesses":
		m = ms.Flows
	case "genComputingSummaryDefAccess":
		m = ms.GenComputingSummaryDefs
	case "layoutAssignments":
		m = ms.Layouts
	case "loginFlows":
		m = ms.LoginFlows
	case "recordTypeVisibilities":
		m = ms.RecordTypes
	case "servicePresenceStatusAccesses":
		m = ms.ServicePresenceStatuses
	default:
		ms.mu.Unlock()
		return nil
	}
	r := copyKeys(m)
	ms.mu.Unlock()
	return sortedKeys(r)
}

func (ms *MasterSchema) AllFields() []string {
	ms.mu.Lock()
	r := copyKeys(ms.Fields)
	ms.mu.Unlock()
	return sortedKeys(r)
}
func (ms *MasterSchema) AllClasses() []string {
	ms.mu.Lock()
	r := copyKeys(ms.Classes)
	ms.mu.Unlock()
	return sortedKeys(r)
}
func (ms *MasterSchema) AllAgents() []string {
	ms.mu.Lock()
	r := copyKeys(ms.Agents)
	ms.mu.Unlock()
	return sortedKeys(r)
}
func (ms *MasterSchema) AllApps() []string {
	ms.mu.Lock()
	r := copyKeys(ms.Apps)
	ms.mu.Unlock()
	return sortedKeys(r)
}
func (ms *MasterSchema) AllCategoryGroups() []string {
	ms.mu.Lock()
	r := copyKeys(ms.CategoryGroups)
	ms.mu.Unlock()
	return sortedKeys(r)
}
func (ms *MasterSchema) AllCustomMetadataTypes() []string {
	ms.mu.Lock()
	r := copyKeys(ms.CustomMetadataTypes)
	ms.mu.Unlock()
	return sortedKeys(r)
}
func (ms *MasterSchema) AllCustomPermissions() []string {
	ms.mu.Lock()
	r := copyKeys(ms.CustomPermissions)
	ms.mu.Unlock()
	return sortedKeys(r)
}
func (ms *MasterSchema) AllCustomSettings() []string {
	ms.mu.Lock()
	r := copyKeys(ms.CustomSettings)
	ms.mu.Unlock()
	return sortedKeys(r)
}
func (ms *MasterSchema) AllExternalDataSources() []string {
	ms.mu.Lock()
	r := copyKeys(ms.ExternalDataSources)
	ms.mu.Unlock()
	return sortedKeys(r)
}
func (ms *MasterSchema) AllFlows() []string {
	ms.mu.Lock()
	r := copyKeys(ms.Flows)
	ms.mu.Unlock()
	return sortedKeys(r)
}
func (ms *MasterSchema) AllGenComputingSummaryDefs() []string {
	ms.mu.Lock()
	r := copyKeys(ms.GenComputingSummaryDefs)
	ms.mu.Unlock()
	return sortedKeys(r)
}
func (ms *MasterSchema) AllLayouts() []string {
	ms.mu.Lock()
	r := copyKeys(ms.Layouts)
	ms.mu.Unlock()
	return sortedKeys(r)
}
func (ms *MasterSchema) AllLoginFlows() []string {
	ms.mu.Lock()
	r := copyKeys(ms.LoginFlows)
	ms.mu.Unlock()
	return sortedKeys(r)
}
func (ms *MasterSchema) AllObjects() []string {
	ms.mu.Lock()
	r := copyKeys(ms.Objects)
	ms.mu.Unlock()
	return sortedKeys(r)
}
func (ms *MasterSchema) AllRecordTypes() []string {
	ms.mu.Lock()
	r := copyKeys(ms.RecordTypes)
	ms.mu.Unlock()
	return sortedKeys(r)
}
func (ms *MasterSchema) AllServicePresenceStatuses() []string {
	ms.mu.Lock()
	r := copyKeys(ms.ServicePresenceStatuses)
	ms.mu.Unlock()
	return sortedKeys(r)
}
func (ms *MasterSchema) AllTabs() []string {
	ms.mu.Lock()
	r := copyKeys(ms.Tabs)
	ms.mu.Unlock()
	return sortedKeys(r)
}
func (ms *MasterSchema) AllUserPerms() []string {
	ms.mu.Lock()
	r := copyKeys(ms.UserPerms)
	ms.mu.Unlock()
	return sortedKeys(r)
}

// keys copies map keys to a pre-allocated slice and returns it unsorted.
func copyKeys(m map[string]bool) []string {
	r := make([]string, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	return r
}

// sortedKeys returns a sorted copy of the keys of m (no mutex).
func sortedKeys(r []string) []string {
	sort.Strings(r)
	return r
}

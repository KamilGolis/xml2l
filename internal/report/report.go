// Package report defines data structures and serialization for profile diff
// and reconciliation reports.
package report

import (
	"sort"

	"xml2l/internal/graph"
)

// MetaKey identifies a unique metadata node by its type and name.
type MetaKey struct {
	MetaType string `json:"metaType"`
	Name     string `json:"name"`
}

// ValueDiff describes a permission value difference for a shared tag.
type ValueDiff struct {
	MetaKey
	ProfileA string `json:"profileA"`
	ProfileB string `json:"profileB"`
	Field    string `json:"field"`
	ValueA   string `json:"valueA"`
	ValueB   string `json:"valueB"`
}

// ProfileMissing lists all tags missing from a single profile.
type ProfileMissing struct {
	ProfileName string              `json:"profileName"`
	Missing     map[string][]string `json:"missing"` // metaType -> []elementName
}

// RepoEntry describes a single item in a repo cross-reference result.
type RepoEntry struct {
	MetaType     string `json:"metaType"`
	Name         string `json:"name"`
	ExpectedPath string `json:"expectedPath,omitempty"`
}

// RepoCrossRef holds the results of cross-referencing profile metadata against the filesystem.
type RepoCrossRef struct {
	MissingFiles     []RepoEntry `json:"missingFiles"`
	Unreferenced     []RepoEntry `json:"unreferenced"`
	SkippedTypesNote string      `json:"skippedTypesNote,omitempty"`
}

// DiffReport is the top-level result of a profile diff.
type DiffReport struct {
	Profiles         []ProfileMissing `json:"profiles"`
	ValueDifferences []ValueDiff      `json:"valueDifferences,omitempty"`
	CrossRef         *RepoCrossRef    `json:"crossRef,omitempty"`
}

// ComputeDiff builds a DiffReport by comparing all profiles in the graph.
// It finds the union of all metadata nodes across profiles, then reports
// per-profile gaps. Tags present in every profile are suppressed (shared).
// When details is true, shared tags with differing permission values are
// included in ValueDifferences.
// When repoPath is non-empty, the repo is scanned and cross-referenced.
func ComputeDiff(g *graph.SalesforceGraph, details bool, repoPath string) *DiffReport {
	if g == nil || len(g.ProfileNodes) == 0 {
		return &DiffReport{}
	}

	// Sort profile nodes by name for deterministic output.
	profileNodes := g.Profiles()
	sort.Slice(profileNodes, func(i, j int) bool {
		return profileNodes[i].Name < profileNodes[j].Name
	})

	// Collect the set of all (metaType, name) keys across all profiles.
	type metaKey struct {
		metaType string
		name     string
	}
	allKeys := make(map[metaKey]bool)
	profileKeys := make(map[*graph.ProfileNode]map[metaKey]bool)

	for _, e := range g.Edges {
		key := metaKey{metaType: string(e.MetadataNode.MetaType), name: e.MetadataNode.Name}
		allKeys[key] = true
		if profileKeys[e.ProfileNode] == nil {
			profileKeys[e.ProfileNode] = make(map[metaKey]bool)
		}
		profileKeys[e.ProfileNode][key] = true
	}

	// Determine which keys are shared across ALL profiles.
	sharedKeys := make(map[metaKey]bool)
	for key := range allKeys {
		presentInAll := true
		for _, pn := range profileNodes {
			if !profileKeys[pn][key] {
				presentInAll = false
				break
			}
		}
		if presentInAll {
			sharedKeys[key] = true
		}
	}

	// Build per-profile missing lists.
	var profiles []ProfileMissing
	var valueDiffs []ValueDiff

	for _, pn := range profileNodes {
		have := profileKeys[pn]
		var missing []metaKey
		for key := range allKeys {
			if !have[key] {
				missing = append(missing, key)
			}
		}
		if len(missing) == 0 {
			continue
		}
		pm := ProfileMissing{ProfileName: pn.Name, Missing: make(map[string][]string)}
		for _, m := range missing {
			pm.Missing[m.metaType] = append(pm.Missing[m.metaType], m.name)
		}
		// Sort element lists for deterministic output.
		for _, elements := range pm.Missing {
			sort.Strings(elements)
		}
		profiles = append(profiles, pm)
	}

	// Value differences for shared tags (when details=true).
	if details && len(profileNodes) >= 2 {
		// Sort sharedKeys for deterministic iteration.
		sortedSharedKeys := make([]metaKey, 0, len(sharedKeys))
		for k := range sharedKeys {
			sortedSharedKeys = append(sortedSharedKeys, k)
		}
		sort.Slice(sortedSharedKeys, func(i, j int) bool {
			if sortedSharedKeys[i].metaType != sortedSharedKeys[j].metaType {
				return sortedSharedKeys[i].metaType < sortedSharedKeys[j].metaType
			}
			return sortedSharedKeys[i].name < sortedSharedKeys[j].name
		})

		for _, key := range sortedSharedKeys {
			// Find Admin profile to use as base; fallback to any profile if no Admin found.
			var baseProf *graph.ProfileNode
			for _, pn := range profileNodes {
				if pn.Name == "Admin" {
					baseProf = pn
					break
				}
			}
			if baseProf == nil {
				baseProf = profileNodes[0]
			}
			if baseProf == nil || len(profileNodes) < 2 {
				continue
			}

			var baseProps *graph.EdgeProperties
			for _, e := range g.ProfileToEdges[baseProf] {
				if string(e.MetadataNode.MetaType) == key.metaType && e.MetadataNode.Name == key.name {
					baseProps = &e.Properties
					break
				}
			}
			if baseProps == nil {
				continue
			}

			baseName := baseProf.Name
			for _, pn := range profileNodes {
				if pn.Name == baseName {
					continue
				}
				var otherProps *graph.EdgeProperties
				for _, e := range g.ProfileToEdges[pn] {
					if string(e.MetadataNode.MetaType) == key.metaType && e.MetadataNode.Name == key.name {
						otherProps = &e.Properties
						break
					}
				}
				if otherProps == nil {
					continue
				}
				diffs := compareProps(*baseProps, *otherProps)
				for _, d := range diffs {
					valueDiffs = append(valueDiffs, ValueDiff{
						MetaKey:  MetaKey{MetaType: key.metaType, Name: key.name},
						ProfileA: baseName,
						ProfileB: pn.Name,
						Field:    d.field,
						ValueA:   d.valA,
						ValueB:   d.valB,
					})
				}
			}
		}
	}

	// Sort value diffs for deterministic output.
	sort.Slice(valueDiffs, func(i, j int) bool {
		if valueDiffs[i].MetaType != valueDiffs[j].MetaType {
			return valueDiffs[i].MetaType < valueDiffs[j].MetaType
		}
		if valueDiffs[i].Name != valueDiffs[j].Name {
			return valueDiffs[i].Name < valueDiffs[j].Name
		}
		if valueDiffs[i].ProfileA != valueDiffs[j].ProfileA {
			return valueDiffs[i].ProfileA < valueDiffs[j].ProfileA
		}
		return valueDiffs[i].ProfileB < valueDiffs[j].ProfileB
	})

	r := &DiffReport{
		Profiles:         profiles,
		ValueDifferences: valueDiffs,
	}

	// Repo cross-reference when path is provided.
	if repoPath != "" && details {
		repoFiles, err := ScanRepo(repoPath)
		if err == nil && repoFiles != nil {
			r.CrossRef = CrossReference(g, repoFiles)
		}
	}

	return r
}

type propDiff struct {
	field string
	valA  string
	valB  string
}

func compareProps(a, b graph.EdgeProperties) []propDiff {
	var diffs []propDiff
	if diff := boolDiff("enabled", a.Enabled, b.Enabled); diff != nil {
		diffs = append(diffs, *diff)
	}
	if diff := boolDiff("readable", a.Readable, b.Readable); diff != nil {
		diffs = append(diffs, *diff)
	}
	if diff := boolDiff("editable", a.Editable, b.Editable); diff != nil {
		diffs = append(diffs, *diff)
	}
	if diff := boolDiff("allowCreate", a.AllowCreate, b.AllowCreate); diff != nil {
		diffs = append(diffs, *diff)
	}
	if diff := boolDiff("allowDelete", a.AllowDelete, b.AllowDelete); diff != nil {
		diffs = append(diffs, *diff)
	}
	if diff := boolDiff("allowRead", a.AllowRead, b.AllowRead); diff != nil {
		diffs = append(diffs, *diff)
	}
	if diff := boolDiff("allowEdit", a.AllowEdit, b.AllowEdit); diff != nil {
		diffs = append(diffs, *diff)
	}
	if diff := boolDiff("modifyAllRecords", a.ModifyAll, b.ModifyAll); diff != nil {
		diffs = append(diffs, *diff)
	}
	if diff := boolDiff("viewAllRecords", a.ViewAll, b.ViewAll); diff != nil {
		diffs = append(diffs, *diff)
	}
	if diff := boolDiff("default", a.Default, b.Default); diff != nil {
		diffs = append(diffs, *diff)
	}
	if diff := boolDiff("visible", a.Visible, b.Visible); diff != nil {
		diffs = append(diffs, *diff)
	}
	if diff := boolDiff("hidden", a.Hidden, b.Hidden); diff != nil {
		diffs = append(diffs, *diff)
	}
	if diff := strDiff("visibility", a.Visibility, b.Visibility); diff != nil {
		diffs = append(diffs, *diff)
	}
	if diff := strDiff("recordType", a.RecordType, b.RecordType); diff != nil {
		diffs = append(diffs, *diff)
	}
	return diffs
}

func boolDiff(field string, a, b *bool) *propDiff {
	if a == nil && b == nil {
		return nil
	}
	va := formatBool(a)
	vb := formatBool(b)
	if va == vb {
		return nil
	}
	return &propDiff{field: field, valA: va, valB: vb}
}

func strDiff(field string, a, b *string) *propDiff {
	if a == nil && b == nil {
		return nil
	}
	va := formatStr(a)
	vb := formatStr(b)
	if va == vb {
		return nil
	}
	return &propDiff{field: field, valA: va, valB: vb}
}

func formatBool(b *bool) string {
	if b == nil {
		return "<nil>"
	}
	if *b {
		return "true"
	}
	return "false"
}

func formatStr(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

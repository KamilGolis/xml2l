package schema

import (
	"fmt"
	"os"
	"path/filepath"

	"xml2l/internal/graph"
	"xml2l/internal/scanner"
)

// LoadProfiles walks projectPath for *.profile-meta.xml files, runs the
// concurrent Phase 2 pipeline (scan → concurrent decode → Master Schema),
// and builds a SalesforceGraph from the results.
func LoadProfiles(projectPath string) (*graph.SalesforceGraph, error) {
	// Phase 1: Scan for ground-truth metadata.
	gt, err := scanner.Scan(projectPath)
	if err != nil {
		return nil, fmt.Errorf("scan repo: %w", err)
	}

	// Find all profile files.
	var profileFiles []string

	err = filepath.WalkDir(projectPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			return nil
		}

		if filepath.Ext(d.Name()) != ".xml" {
			return nil
		}

		if matched, _ := filepath.Match("*.profile-meta.xml", d.Name()); matched {
			profileFiles = append(profileFiles, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk profiles: %w", err)
	}

	// Phase 2: Concurrent decode + Master Schema aggregation.
	results, ms, decodeErrs := RunConcurrent(profileFiles, gt)

	g := graph.NewGraph()

	g.SetMasterSchema(ms)

	layouts, _ := scanner.ScanLayouts(projectPath)
	g.SetAvailableLayouts(layouts)

	for _, r := range results {
		if r.Profile == nil {
			continue
		}

		profileName := filepath.Base(r.Path)
		profileName = profileName[:len(profileName)-len(".profile-meta.xml")]

		p := g.AddProfile(profileName, r.Path)
		p.RawXML = string(r.Raw)
		p.UserLicense = r.Profile.UserLicense
		p.Description = r.Profile.Description
		p.IsCustom = r.Profile.IsCustom
		p.HasCustomTag = r.Profile.HasCustomTag

		// Map section entries to graph edges.
		for sectionTag, entries := range r.Profile.Sections {
			metaType := graph.TagToMetaType(sectionTag)

			for _, entry := range entries {
				mn := g.GetOrCreateMetadataNode(metaType, entry.Name)
				g.AddEdge(p, mn, entry.ToEdgeProperties())
			}
		}
	}

	if len(decodeErrs) > 0 {
		for _, e := range decodeErrs {
			fmt.Fprintf(os.Stderr, "warning: %v\n", e)
		}
	}

	return g, nil
}

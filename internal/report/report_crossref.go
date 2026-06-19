package report

import (
	"strings"

	"xml2l/internal/graph"
)

// CrossReference compares the graph's metadata nodes against the filesystem scan result.
// Returns a RepoCrossRef with missing files and unreferenced metadata.
func CrossReference(g *graph.SalesforceGraph, repoFiles map[string]map[string]bool) *RepoCrossRef {
	if g == nil {
		return &RepoCrossRef{}
	}

	xr := &RepoCrossRef{}

	// Direction 1: graph → filesystem — find metadata nodes with no matching file.
	seenInGraph := make(map[string]map[string]bool) // metaType -> name -> true (for direction 2)
	for _, mn := range g.MetadataNodes {
		metaType := string(mn.MetaType)
		if metaTypeSkipList[metaType] {
			continue
		}
		// Initialize seen map.
		if seenInGraph[metaType] == nil {
			seenInGraph[metaType] = make(map[string]bool)
		}
		seenInGraph[metaType][mn.Name] = true

		// Check if this node exists in the filesystem.
		if repoFiles[metaType] == nil || !repoFiles[metaType][mn.Name] {
			// Also check for types that map through object subdirs (Field, RecordType).
			// These are already in repoFiles under their MetaType from ScanRepo.
			xr.MissingFiles = append(xr.MissingFiles, RepoEntry{
				MetaType:     metaType,
				Name:         mn.Name,
				ExpectedPath: expectedPath(metaType, mn.Name),
			})
		}
	}

	// Direction 2: filesystem → graph — find files with no matching metadata node.
	for metaType, entries := range repoFiles {
		if metaTypeSkipList[metaType] {
			continue
		}
		for name := range entries {
			if seenInGraph[metaType] == nil || !seenInGraph[metaType][name] {
				xr.Unreferenced = append(xr.Unreferenced, RepoEntry{
					MetaType: metaType,
					Name:     name,
				})
			}
		}
	}

	// Build skipped-types note.
	var skipped []string
	for mt := range metaTypeSkipList {
		skipped = append(skipped, mt)
	}
	if len(skipped) > 0 {
		xr.SkippedTypesNote = "Skipped " + strings.Join(skipped, ", ")
	}

	return xr
}

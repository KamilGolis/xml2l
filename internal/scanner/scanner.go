// Package scanner walks a Salesforce DX project directory to discover metadata
// files and build a ground-truth map of what exists on disk. Phase 1 of the
// 3-phase profile normalization pipeline.
package scanner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GroundTruth is a lookup set of metadata identifiers that exist on disk.
// Key is the logical metadata name (e.g. "MyClass", "Account.Revenue__c").
// Value is always true — use "if gt[name]" to check membership.
type GroundTruth map[string]bool

// Scan walks root looking for .cls and .field-meta.xml files and returns a
// GroundTruth map of all discovered metadata identifiers. Duplicate entries
// (e.g. MyClass.cls + MyClass.cls-meta.xml) collapse to a single key.
//
// root is typically the force-app/main/default/ directory of an SFDX project.
func Scan(root string) (GroundTruth, error) {
	gt := make(GroundTruth)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skip %s: %v\n", path, err)
			return nil
		}
		if d.IsDir() {
			return nil
		}

		name := d.Name()

		switch {
		case strings.HasSuffix(name, ".cls-meta.xml"):
			ident := strings.TrimSuffix(name, ".cls-meta.xml")
			gt[ident] = true

		case strings.HasSuffix(name, ".cls") && !strings.HasSuffix(name, ".cls-meta.xml"):
			ident := strings.TrimSuffix(name, ".cls")
			gt[ident] = true

		case strings.HasSuffix(name, ".field-meta.xml"):
			ident := extractFieldIdentifier(path, name)
			if ident != "" {
				gt[ident] = true
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}

	return gt, nil
}

// extractFieldIdentifier derives the full field identifier (Object.FieldName)
// from the file path. If the path is inside objects/<ObjName>/fields/, the
// result is prefixed with the object name. Otherwise just the field name.
func extractFieldIdentifier(path, fileName string) string {
	ident := strings.TrimSuffix(fileName, ".field-meta.xml")
	if ident == "" {
		return ""
	}

	// Walk up from the file to find objects/<ObjName>/fields/ structure.
	// Expected: .../objects/<ObjectName>/fields/<File>.field-meta.xml
	dir := filepath.Dir(path)
	parentDir := filepath.Dir(dir)

	// Check that the immediate parent is "fields" and the grandparent's
	// parent is "objects" to avoid false prefixes on flat paths.
	if filepath.Base(dir) == "fields" && filepath.Base(parentDir) != "" {
		gpDir := filepath.Dir(parentDir)
		if filepath.Base(gpDir) == "objects" {
			objName := filepath.Base(parentDir)
			return objName + "." + ident
		}
	}

	return ident
}

// ScanLayouts scans the layouts/ subdirectory of root for .layout-meta.xml files
// and returns the layout names (extension stripped) in sorted order.
// Returns an empty slice if the layouts directory does not exist.
func ScanLayouts(root string) ([]string, error) {
	layoutDir := filepath.Join(root, "layouts")
	entries, err := os.ReadDir(layoutDir)
	if err != nil {
		return nil, nil // directory doesn't exist — no layouts available
	}
	var layouts []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".layout-meta.xml") {
			layouts = append(layouts, strings.TrimSuffix(name, ".layout-meta.xml"))
		}
	}
	sort.Strings(layouts)
	return layouts, nil
}

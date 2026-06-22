package report

import (
	"os"
	"path/filepath"
	"strings"

	"xml2l/internal/graph"
)

// metaTypeSkipList contains types that have no corresponding filesystem files.
var metaTypeSkipList = map[string]bool{
	"UserPermission": true,
	"CategoryGroup":  true,
}

// ScanRepo walks the repository directory under root and collects all metadata files
// matching the MetaType-to-directory mapping. Returns a map of MetaType -> Name -> true.
func ScanRepo(root string) (map[string]map[string]bool, error) {
	result := make(map[string]map[string]bool)

	// Initialize result maps for all types with directory entries.
	for mt, de := range graph.MetaTypeDirMap() {
		if result[string(mt)] == nil {
			result[string(mt)] = make(map[string]bool)
		}
		_ = de
	}
	for _, sd := range graph.ObjectSubDirs {
		if result[string(sd.MetaType)] == nil {
			result[string(sd.MetaType)] = make(map[string]bool)
		}
	}

	// Scan flat directories.
	for mt, de := range graph.MetaTypeDirMap() {
		if de.Recursive {
			continue // handled separately
		}
		dirPath := filepath.Join(root, de.Dir)
		info, err := os.Stat(dirPath)
		if err != nil {
			continue // directory doesn't exist, skip
		}
		if !info.IsDir() {
			continue
		}
		for _, pattern := range de.Patterns {
			matches, err := filepath.Glob(filepath.Join(dirPath, pattern))
			if err != nil {
				continue
			}
			for _, m := range matches {
				name := filepath.Base(m)
				name = stripExtensions(name)
				result[string(mt)][name] = true
			}
		}
	}

	// Scan classes recursively (nested subdirectories).
	if de, ok := graph.MetaTypeDirMap()[graph.MetaTypeApexClass]; ok {
		classDir := filepath.Join(root, de.Dir)
		filepath.Walk(classDir, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return nil // skip inaccessible paths
			}
			if fi.IsDir() {
				return nil
			}
			for _, pattern := range de.Patterns {
				matched, err := filepath.Match(pattern, fi.Name())
				if err == nil && matched {
					name := stripExtensions(fi.Name())
					result["ApexClass"][name] = true
					break
				}
			}
			return nil
		})
	}

	// Scan object subdirectories: fields/ and recordTypes/ only.
	objectDir := filepath.Join(root, "objects")
	objEntries, err := os.ReadDir(objectDir)
	if err == nil {
		for _, objEntry := range objEntries {
			if !objEntry.IsDir() {
				continue
			}
			objName := objEntry.Name()
			for _, sd := range graph.ObjectSubDirs {
				subDir := filepath.Join(objectDir, objName, sd.SubDir)
				matches, err := filepath.Glob(filepath.Join(subDir, sd.Pattern))
				if err != nil {
					continue
				}
				for _, m := range matches {
					fileName := filepath.Base(m)
					entryName := stripExtensions(fileName)
					fullName := objName + "." + entryName
					result[string(sd.MetaType)][fullName] = true
				}
			}

			// Also detect object-meta.xml files that match CustomSetting or CustomMetadataType patterns
			objMetaGlob := filepath.Join(objectDir, objName, objName+".object-meta.xml")
			if matches, err := filepath.Glob(objMetaGlob); err == nil {
				for _, m := range matches {
					name := stripExtensions(filepath.Base(m))
					if strings.HasSuffix(name, "__c") {
						result["CustomSetting"][name] = true
					} else if strings.HasSuffix(name, "__mdt") {
						result["CustomMetadata"][name] = true
					} else {
						result["CustomObject"][name] = true
					}
				}
			}
		}
	}

	return result, nil
}

// stripExtensions removes Salesforce metadata file extensions to get the logical name.
func stripExtensions(name string) string {
	for _, ext := range []string{".cls-meta.xml", ".cls", ".page-meta.xml", ".page",
		".trigger-meta.xml", ".trigger", ".component-meta.xml", ".component",
		".app-meta.xml", ".app", ".tab-meta.xml", ".tab", ".layout",
		".customPermission-meta.xml", ".customPermission",
		".flow-meta.xml", ".flow",
		".object-meta.xml", ".object",
		".field-meta.xml", ".field",
		".recordType-meta.xml", ".recordType"} {
		if strings.HasSuffix(name, ext) {
			return strings.TrimSuffix(name, ext)
		}
	}
	return name
}

// expectedPath returns a human-readable filesystem path for a metadata node.
func expectedPath(metaType, name string) string {
	if de, ok := graph.MetaTypeDirMap()[graph.MetadataType(metaType)]; ok {
		return de.Dir + "/" + name
	}
	// Handle Field and RecordType from object subdirs.
	if metaType == "CustomField" || metaType == "RecordType" {
		parts := strings.SplitN(name, ".", 2)
		if len(parts) == 2 {
			subDir := "fields"
			ext := ".field-meta.xml"
			if metaType == "RecordType" {
				subDir = "recordTypes"
				ext = ".recordType-meta.xml"
			}
			return "objects/" + parts[0] + "/" + subDir + "/" + parts[1] + ext
		}
	}
	return metaType + "/" + name
}

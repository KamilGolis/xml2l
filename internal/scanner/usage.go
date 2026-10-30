package scanner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ApexUsage is what a project's Apex entry points require from profiles.
type ApexUsage struct {
	// Required holds the Apex classes Salesforce invokes on behalf of the
	// running user and that exist in the project.
	Required map[string]bool
	// Warnings holds references that cannot be honoured, one per class.
	Warnings []string
}

var (
	restResourceRe  = regexp.MustCompile(`@\s*RestResource\b`)
	webServiceRe    = regexp.MustCompile(`\bwebservice\b`)
	vfControllerRe  = regexp.MustCompile(`(?:controller|extensions)="([^"]+)"`)
	auraController  = regexp.MustCompile(`controller="([^"]+)"`)
	lwcApexImportRe = regexp.MustCompile(`@salesforce/apex/([\w.]+)`)
	flowApexClassRe = regexp.MustCompile(`<apexClass>([^<]+)</apexClass>`)
)

// ScanApexUsage reports the Apex classes that need class access in a profile:
// REST and SOAP endpoints, Visualforce controllers and extensions, Aura and LWC
// controllers, and Flow invocables. A class annotated @AuraEnabled or
// @InvocableMethod without a call site is not required: nothing invokes it.
//
// It returns an error when the project contains no .cls files, so pointing at
// the wrong directory cannot strip class access from every profile.
func ScanApexUsage(root string) (ApexUsage, error) {
	usage := ApexUsage{Required: make(map[string]bool)}

	existing := make(map[string]bool)
	referenced := make(map[string]string) // class -> first file that requires it

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		name := d.Name()
		slashed := filepath.ToSlash(path)

		switch {
		case strings.HasSuffix(name, ".cls-meta.xml"):
			// The companion file carries no annotations; the .cls file does.

		case strings.HasSuffix(name, ".cls"):
			class := strings.TrimSuffix(name, ".cls")
			existing[class] = true

			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}

			if restResourceRe.Match(body) || webServiceRe.Match(body) {
				referenced[class] = path
			}

		case strings.HasSuffix(name, ".page") || strings.HasSuffix(name, ".component"):
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}

			for _, m := range vfControllerRe.FindAllSubmatch(body, -1) {
				for _, ref := range strings.Split(string(m[1]), ",") {
					addReference(referenced, strings.TrimSpace(ref), path)
				}
			}

		case strings.Contains(slashed, "/aura/") && hasAnySuffix(name, ".cmp", ".app", ".evt"):
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}

			for _, m := range auraController.FindAllSubmatch(body, -1) {
				addReference(referenced, strings.TrimSpace(string(m[1])), path)
			}

		case strings.Contains(slashed, "/lwc/") && strings.HasSuffix(name, ".js"):
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}

			for _, m := range lwcApexImportRe.FindAllSubmatch(body, -1) {
				addReference(referenced, classForImport(string(m[1])), path)
			}

		case strings.HasSuffix(name, ".flow-meta.xml"):
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}

			for _, m := range flowApexClassRe.FindAllSubmatch(body, -1) {
				addReference(referenced, strings.TrimSpace(string(m[1])), path)
			}
		}

		return nil
	})
	if err != nil {
		return usage, fmt.Errorf("scan %s: %w", root, err)
	}

	if len(existing) == 0 {
		return usage, fmt.Errorf("no Apex classes found under %s; refusing to prune class access", root)
	}

	for class, file := range referenced {
		if existing[class] {
			usage.Required[class] = true
			continue
		}

		rel, relErr := filepath.Rel(root, file)
		if relErr != nil {
			rel = file
		}

		usage.Warnings = append(usage.Warnings, fmt.Sprintf(
			"entry-point class %s is referenced by %s but has no .cls file",
			class, filepath.ToSlash(rel)))
	}

	sort.Strings(usage.Warnings)

	return usage, nil
}

// hasAnySuffix reports whether name ends with one of the given suffixes.
func hasAnySuffix(name string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}

	return false
}

// addReference records that a file requires a class. The first file wins, so a
// class referenced from several places produces one warning.
func addReference(referenced map[string]string, class, file string) {
	if class == "" {
		return
	}

	if _, seen := referenced[class]; !seen {
		referenced[class] = file
	}
}

// classForImport returns the class in an LWC import such as
// "@salesforce/apex/ns.MyClass.myMethod".
func classForImport(ref string) string {
	parts := strings.Split(ref, ".")
	if len(parts) < 2 {
		return ""
	}

	return parts[len(parts)-2]
}

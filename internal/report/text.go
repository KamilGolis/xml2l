package report

import (
	"fmt"
	"sort"
	"strings"
)

// FormatDiffText produces a human-readable text representation of a DiffReport.
// Output is grouped by profile, then by MetaType, with line width ≤ 80 chars.
func FormatDiffText(r *DiffReport) string {
	if r == nil || (len(r.Profiles) == 0 && len(r.ValueDifferences) == 0 && r.CrossRef == nil) {
		return "No differences found.\n"
	}

	var b strings.Builder

	// Sort profiles by name for deterministic output.
	sort.Slice(r.Profiles, func(i, j int) bool {
		return r.Profiles[i].ProfileName < r.Profiles[j].ProfileName
	})

	for _, pm := range r.Profiles {
		fmt.Fprintf(&b, "[%s]\n", pm.ProfileName)

		// Sort metaTypes for deterministic output.
		metaTypes := make([]string, 0, len(pm.Missing))
		for mt := range pm.Missing {
			metaTypes = append(metaTypes, mt)
		}
		sort.Strings(metaTypes)

		for _, mt := range metaTypes {
			elements := pm.Missing[mt]
			sort.Strings(elements)
			fmt.Fprintf(&b, "  %s (%d):\n", mt, len(elements))
			for _, el := range elements {
				line := fmt.Sprintf("    - %s", el)
				if len(line) > 80 {
					line = line[:77] + "..."
				}
				fmt.Fprintln(&b, line)
			}
		}
	}

	if len(r.ValueDifferences) > 0 {
		fmt.Fprintln(&b, "\n[Value Differences]")
		for _, vd := range r.ValueDifferences {
			line := fmt.Sprintf("  %s/%s: %s=%s (%s) vs %s=%s (%s)",
				vd.MetaType, vd.Name, vd.Field, vd.ValueA, vd.ProfileA,
				vd.Field, vd.ValueB, vd.ProfileB)
			if len(line) > 80 {
				// Compact form for long lines.
				line = fmt.Sprintf("  %s/%s: %s differs (%s: %s, %s: %s)",
					vd.MetaType, vd.Name, vd.Field, vd.ProfileA, vd.ValueA,
					vd.ProfileB, vd.ValueB)
			}
			fmt.Fprintln(&b, line)
		}
	}

	// Repo cross-reference section.
	if r.CrossRef != nil {
		if len(r.CrossRef.MissingFiles) > 0 || len(r.CrossRef.Unreferenced) > 0 {
			if len(r.ValueDifferences) > 0 || len(r.Profiles) > 0 {
				fmt.Fprintln(&b)
			}

			if len(r.CrossRef.MissingFiles) > 0 {
				fmt.Fprintln(&b, "[Missing Files]")
				fmt.Fprintln(&b, "  Metadata in profiles but not found in repository:")
				for _, entry := range r.CrossRef.MissingFiles {
					line := fmt.Sprintf("    %s/%s", entry.MetaType, entry.Name)
					fmt.Fprintln(&b, line)
				}
			}

			if len(r.CrossRef.Unreferenced) > 0 {
				if len(r.CrossRef.MissingFiles) > 0 {
					fmt.Fprintln(&b)
				}
				fmt.Fprintln(&b, "[Unreferenced Metadata]")
				fmt.Fprintln(&b, "  Files in repository but not referenced by any profile:")
				for _, entry := range r.CrossRef.Unreferenced {
					line := fmt.Sprintf("    %s/%s", entry.MetaType, entry.Name)
					fmt.Fprintln(&b, line)
				}
			}
		}

		if r.CrossRef.SkippedTypesNote != "" {
			fmt.Fprintf(&b, "\n  %s\n", r.CrossRef.SkippedTypesNote)
		}
	}

	return b.String()
}

// Package converter converts Salesforce XML files to YAML via a depth-buffered
// XML tokenizer, preserving element order and structure without normalization.
package converter

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// bufNode represents a parsed XML element with its children buffered.
type bufNode struct {
	Name     string    // local element name (namespace stripped)
	Text     string    // text content if leaf element
	Children []bufNode // child elements if non-leaf
}

// nameGroup holds all elements that share a tag name at one depth level.
type nameGroup struct {
	Name     string
	Elements []bufNode
}

// ConvertProfiles discovers *.profile-meta.xml files under projectPath, filters
// by profileName (single) or profilesCSV (comma-separated) if provided, converts
// each to YAML, writes the output to disk, and prints progress to stderr.
// profileName and profilesCSV must not both be set — the caller enforces this.
func ConvertProfiles(projectPath, profileName, profilesCSV string) error {
	profiles, err := findAndFilter(projectPath, profileName, profilesCSV)
	if err != nil {
		return err
	}

	for _, profilePath := range profiles {
		outPath, yamlBytes, err := ConvertFile(profilePath)
		if err != nil {
			return fmt.Errorf("convert %s: %w", profilePath, err)
		}
		if err := os.WriteFile(outPath, yamlBytes, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", outPath, err)
		}

		fmt.Fprintf(os.Stderr, "Converted: %s\n", filepath.Base(profilePath))
	}
	fmt.Fprintf(os.Stderr, "Done. %d profile(s) converted.\n", len(profiles))

	return nil
}

// ConvertFile reads XML from inputPath and returns the derived output path
// and YAML content. The output path replaces the ".profile-meta.xml" suffix
// with ".profile-meta.yaml".
func ConvertFile(inputPath string) (outputPath string, yamlBytes []byte, err error) {
	f, err := os.Open(inputPath)
	if err != nil {
		return "", nil, fmt.Errorf("open %s: %w", inputPath, err)
	}

	defer f.Close()

	root, err := parseXML(xml.NewDecoder(f))
	if err != nil {
		return "", nil, fmt.Errorf("parse %s: %w", inputPath, err)
	}

	var buf bytes.Buffer
	root.emit(&buf, 0)

	outputPath = deriveOutputPath(inputPath)
	return outputPath, buf.Bytes(), nil
}

// findAndFilter discovers all *.profile-meta.xml files under projectPath and
// filters them according to profileName (single) or profilesCSV (comma-separated
// list). If both are empty, all discovered profiles are returned.
// profileName and profilesCSV must not both be set — the caller (CLI) enforces
// this via MarkFlagsMutuallyExclusive.
func findAndFilter(projectPath, profileName, profilesCSV string) ([]string, error) {
	all, err := findProfiles(projectPath)
	if err != nil {
		return nil, err
	}

	if profileName != "" {
		matched, _, err := filterByNames(all, []string{profileName})
		if err != nil {
			return nil, err
		}

		return matched, nil
	}

	if profilesCSV != "" {
		names := strings.Split(profilesCSV, ",")

		for i := range names {
			names[i] = strings.TrimSpace(names[i])
		}

		matched, _, err := filterByNames(all, names)
		if err != nil {
			return nil, err
		}

		return matched, nil
	}

	return all, nil
}

// findProfiles walks dir searching for *.profile-meta.xml files.
func findProfiles(dir string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}

		if d.IsDir() {
			return nil
		}

		if strings.HasSuffix(d.Name(), ".profile-meta.xml") {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", dir, err)
	}

	return files, nil
}

// filterByNames returns only the profile paths whose base name (without
// .profile-meta.xml) matches one of the given names. The available names are
// returned alongside for error reporting.
func filterByNames(profiles []string, names []string) (matched []string, available []string, err error) {
	available = make([]string, 0, len(profiles))
	byName := make(map[string]string, len(profiles))

	for _, p := range profiles {
		base := filepath.Base(p)
		name := strings.TrimSuffix(base, ".profile-meta.xml")
		available = append(available, name)
		byName[name] = p
	}

	missing := make([]string, 0, len(names))

	for _, n := range names {
		if p, ok := byName[n]; ok {
			matched = append(matched, p)
		} else {
			missing = append(missing, n)
		}
	}

	if len(missing) > 0 {
		sort.Strings(available)

		return nil, available, fmt.Errorf("profiles not found: %s; available: %s",
			strings.Join(missing, ", "),
			strings.Join(available, ", "))
	}

	return matched, available, nil
}

// parseXML skips leading non-element tokens (XML declaration, etc.) and
// returns the root element as a buffered node tree.
func parseXML(decoder *xml.Decoder) (bufNode, error) {
	for {
		tok, err := decoder.Token()

		if err == io.EOF {
			return bufNode{}, fmt.Errorf("empty or non-XML input")
		}

		if err != nil {
			return bufNode{}, err
		}

		if se, ok := tok.(xml.StartElement); ok {
			return readElement(decoder, se)
		}
		// Skip ProcInst (<?xml?>), Comment, Directive, CharData.
	}
}

// readElement reads all tokens inside a StartElement until the matching
// EndElement, buffering all children recursively.
func readElement(decoder *xml.Decoder, se xml.StartElement) (bufNode, error) {
	node := bufNode{Name: se.Name.Local}
	var textParts []string

	for {
		tok, err := decoder.Token()
		if err != nil {
			return bufNode{}, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			child, err := readElement(decoder, t)
			if err != nil {
				return bufNode{}, err
			}
			node.Children = append(node.Children, child)

		case xml.CharData:
			trimmed := strings.TrimSpace(string(t))
			if trimmed != "" {
				textParts = append(textParts, trimmed)
			}

		case xml.EndElement:
			if len(textParts) > 0 {
				node.Text = strings.Join(textParts, " ")
			}
			return node, nil
		}
	}
}

// emit writes a single node (no grouping) at the given indent.
func (n bufNode) emit(buf *bytes.Buffer, indent int) {
	pfx := strings.Repeat(" ", indent)

	if len(n.Children) == 0 {
		fmt.Fprintf(buf, "%s%s: %s\n", pfx, n.Name, yq(n.Text))
	} else {
		fmt.Fprintf(buf, "%s%s:\n", pfx, n.Name)
		emitGrouped(buf, n.Children, indent+2)
	}
}

// emitGrouped writes the grouped children to buf at the given indent level.
func emitGrouped(buf *bytes.Buffer, nodes []bufNode, indent int) {
	groups := groupByName(nodes)

	for _, g := range groups {
		pfx := strings.Repeat(" ", indent)

		if len(g.Elements) == 1 {
			// Singleton — plain key-value or nested map.
			g.Elements[0].emit(buf, indent)
		} else {
			// Repeated — YAML sequence.
			fmt.Fprintf(buf, "%s%s:\n", pfx, g.Name)

			for _, el := range g.Elements {
				if len(el.Children) == 0 {
					// Leaf repeated items become scalar list items.
					fmt.Fprintf(buf, "%s  - %s\n", pfx, yq(el.Text))
				} else {
					// Complex repeated — first child inline with "- ".
					emitSeqItem(buf, el.Children, indent+2)
				}
			}
		}
	}
}

// groupByName groups nodes by their Name, preserving the original first-appearance
// order of distinct names.
func groupByName(nodes []bufNode) []nameGroup {
	order := make([]string, 0, len(nodes))

	// name → index in order
	seen := make(map[string]int)
	for _, n := range nodes {
		if _, ok := seen[n.Name]; !ok {
			seen[n.Name] = len(order)
			order = append(order, n.Name)
		}
	}

	groups := make([]nameGroup, len(order))
	lists := make([][]bufNode, len(order))

	for _, n := range nodes {
		idx := seen[n.Name]
		lists[idx] = append(lists[idx], n)
	}
	for i, name := range order {
		groups[i] = nameGroup{Name: name, Elements: lists[i]}
	}

	return groups
}

// emitSeqItem writes one YAML sequence item for a complex element.
// The first child goes inline after "- "; subsequent children are indented
// two more spaces.
func emitSeqItem(buf *bytes.Buffer, children []bufNode, itemIndent int) {
	if len(children) == 0 {
		return
	}

	first := children[0]
	rest := children[1:]

	itemPfx := strings.Repeat(" ", itemIndent)

	if len(first.Children) == 0 {
		fmt.Fprintf(buf, "%s- %s: %s\n", itemPfx, first.Name, yq(first.Text))
	} else {
		fmt.Fprintf(buf, "%s- %s:\n", itemPfx, first.Name)
		emitGrouped(buf, first.Children, itemIndent+2)
	}

	childPfx := strings.Repeat(" ", itemIndent+2)

	for _, child := range rest {
		if len(child.Children) == 0 {
			fmt.Fprintf(buf, "%s%s: %s\n", childPfx, child.Name, yq(child.Text))
		} else {
			fmt.Fprintf(buf, "%s%s:\n", childPfx, child.Name)
			emitGrouped(buf, child.Children, itemIndent+4)
		}
	}
}

// yq wraps a string in YAML-safe double quotes.
func yq(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return `"` + s + `"`
}

// deriveOutputPath replaces the .profile-meta.xml suffix with .profile-meta.yaml.
func deriveOutputPath(inputPath string) string {
	return strings.TrimSuffix(inputPath, ".profile-meta.xml") + ".profile-meta.yaml"
}

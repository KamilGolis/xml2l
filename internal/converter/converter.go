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

	"gopkg.in/yaml.v3"

	"xml2l/internal/graph"
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
func ConvertProfiles(projectPath, profileName, profilesCSV, fromType string) error {
	direction, err := ParseFromType(fromType)
	if err != nil {
		return err
	}

	inSuffix, outSuffix := profileSuffixes(direction)

	profiles, err := findAndFilter(projectPath, profileName, profilesCSV, inSuffix)
	if err != nil {
		return err
	}

	for _, profilePath := range profiles {
		outPath, content, err := convertFile(profilePath, direction, inSuffix, outSuffix, "Profile")
		if err != nil {
			return fmt.Errorf("convert %s: %w", profilePath, err)
		}
		if err := os.WriteFile(outPath, content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", outPath, err)
		}

		fmt.Fprintf(os.Stderr, "Converted: %s\n", filepath.Base(profilePath))
	}
	fmt.Fprintf(os.Stderr, "Done. %d profile(s) converted.\n", len(profiles))

	return nil
}

// profileSuffixes returns the input and output suffixes for a direction.
func profileSuffixes(direction string) (inSuffix, outSuffix string) {
	if direction == FromTypeYAML {
		return ".profile-meta.yaml", ".profile-meta.xml"
	}

	return ".profile-meta.xml", ".profile-meta.yaml"
}

// permSetSuffixes returns the input and output suffixes for a direction.
func permSetSuffixes(direction string) (inSuffix, outSuffix string) {
	if direction == FromTypeYAML {
		return ".permissionset-meta.yaml", ".permissionset-meta.xml"
	}

	return ".permissionset-meta.xml", ".permissionset-meta.yaml"
}

// convertFile converts one file in the given direction and returns the output
// path and content.
func convertFile(inputPath, direction, inSuffix, outSuffix, rootName string) (string, []byte, error) {
	if direction == FromTypeYAML {
		data, err := os.ReadFile(inputPath)
		if err != nil {
			return "", nil, fmt.Errorf("read %s: %w", inputPath, err)
		}

		content, err := YAMLToXML(data, rootName)
		if err != nil {
			return "", nil, err
		}

		return strings.TrimSuffix(inputPath, inSuffix) + outSuffix, content, nil
	}

	return convertXmlToYaml(inputPath, inSuffix, outSuffix)
}

// convertXmlToYaml reads XML from inputPath and returns the derived output path
// and YAML content. The output path replaces the inSuffix suffix with outSuffix.
func convertXmlToYaml(inputPath, inSuffix, outSuffix string) (outputPath string, yamlBytes []byte, err error) {
	f, err := os.Open(inputPath)
	if err != nil {
		return "", nil, fmt.Errorf("open %s: %w", inputPath, err)
	}

	defer f.Close()

	root, err := parseXML(xml.NewDecoder(f))
	if err != nil {
		return "", nil, fmt.Errorf("parse %s: %w", inputPath, err)
	}

	yamlBytes, err = encodeYAML(root)
	if err != nil {
		return "", nil, fmt.Errorf("encode %s: %w", inputPath, err)
	}

	outputPath = strings.TrimSuffix(inputPath, inSuffix) + outSuffix
	return outputPath, yamlBytes, nil
}

// ConvertProfileFile reads XML from inputPath and returns the derived output path
// and YAML content. The output path replaces the ".profile-meta.xml" suffix
// with ".profile-meta.yaml".
func ConvertProfileFile(inputPath string) (outputPath string, yamlBytes []byte, err error) {
	return convertXmlToYaml(inputPath, ".profile-meta.xml", ".profile-meta.yaml")
}

// ConvertPermSetFile reads XML from inputPath and returns the derived output path
// and YAML content. The output path replaces the ".permissionset-meta.xml" suffix
// with ".permissionset-meta.yaml".
func ConvertPermSetFile(inputPath string) (outputPath string, yamlBytes []byte, err error) {
	return convertXmlToYaml(inputPath, ".permissionset-meta.xml", ".permissionset-meta.yaml")
}

// findAndFilter discovers all *.profile-meta.xml files under projectPath and
// filters them according to profileName (single) or profilesCSV (comma-separated
// list). If both are empty, all discovered profiles are returned.
// profileName and profilesCSV must not both be set — the caller (CLI) enforces
// this via MarkFlagsMutuallyExclusive.
func findAndFilter(projectPath, profileName, profilesCSV, suffix string) ([]string, error) {
	all, err := findProfiles(projectPath, suffix)
	if err != nil {
		return nil, err
	}

	if profileName != "" {
		matched, _, err := filterByNames(all, []string{profileName}, suffix)
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

		matched, _, err := filterByNames(all, names, suffix)
		if err != nil {
			return nil, err
		}

		return matched, nil
	}

	return all, nil
}

// findProfiles walks dir searching for files with the given suffix.
func findProfiles(dir, suffix string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}

		if d.IsDir() {
			return nil
		}

		if strings.HasSuffix(d.Name(), suffix) {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", dir, err)
	}

	return files, nil
}

// metadataSuffixes are stripped from user-supplied names, so "Admin.xml" and
// "Admin.profile-meta.yaml" both select the profile "Admin". Longest first.
var metadataSuffixes = []string{
	".profile-meta.xml",
	".profile-meta.yaml",
	".permissionset-meta.xml",
	".permissionset-meta.yaml",
	".xml",
	".yaml",
	".yml",
}

// normalizeMetadataName removes a known file suffix from a user-supplied name.
func normalizeMetadataName(name string) string {
	for _, suffix := range metadataSuffixes {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix)
		}
	}

	return name
}

// filterByNames returns only the profile paths whose base name (without
// .profile-meta.xml) matches one of the given names. The available names are
// returned alongside for error reporting.
func filterByNames(profiles []string, names []string, suffix string) (matched []string, available []string, err error) {
	available = make([]string, 0, len(profiles))
	byName := make(map[string]string, len(profiles))

	for _, p := range profiles {
		base := filepath.Base(p)
		name := strings.TrimSuffix(base, suffix)
		available = append(available, name)
		byName[name] = p
	}

	missing := make([]string, 0, len(names))

	for _, n := range names {
		n = normalizeMetadataName(strings.TrimSpace(n))

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

// encodeYAML renders the element tree as YAML. Keys stay plain element names
// and every value is double-quoted, so a value that looks like a boolean or a
// number cannot be reinterpreted when the YAML is read back.
func encodeYAML(root bufNode) ([]byte, error) {
	node := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: root.Name},
			mappingNode(root.Children, ""),
		},
	}

	var buf bytes.Buffer

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)

	if err := enc.Encode(node); err != nil {
		return nil, err
	}

	if err := enc.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// mappingNode renders sibling elements: each distinct name once, in
// first-appearance order. When anchor is one of the names, that child is moved
// first so a human sees the entry's identifier at the top of its item.
func mappingNode(nodes []bufNode, anchor string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode}

	for _, group := range moveGroupFirst(groupByName(nodes), anchor) {
		key := &yaml.Node{Kind: yaml.ScalarNode, Value: group.Name}
		node.Content = append(node.Content, key, groupNode(group))
	}

	return node
}

// groupNode renders one element name: a single element as a value, repeated
// elements as a sequence in document order.
func groupNode(group nameGroup) *yaml.Node {
	if len(group.Elements) == 1 {
		return elementNode(group.Name, group.Elements[0])
	}

	seq := &yaml.Node{Kind: yaml.SequenceNode}

	for _, element := range group.Elements {
		seq.Content = append(seq.Content, elementNode(group.Name, element))
	}

	return seq
}

// moveGroupFirst moves the group named anchor to the front, leaving the rest in
// their original order.
func moveGroupFirst(groups []nameGroup, anchor string) []nameGroup {
	if anchor == "" {
		return groups
	}

	for i, group := range groups {
		if group.Name != anchor || i == 0 {
			continue
		}

		moved := make([]nameGroup, 0, len(groups))
		moved = append(moved, group)
		moved = append(moved, groups[:i]...)
		moved = append(moved, groups[i+1:]...)

		return moved
	}

	return groups
}

// groupByName groups nodes by their Name, preserving the original first-appearance
// order of distinct names.
func groupByName(nodes []bufNode) []nameGroup {
	order := make([]string, 0, len(nodes))
	seen := make(map[string]int)

	for _, n := range nodes {
		if _, ok := seen[n.Name]; !ok {
			seen[n.Name] = len(order)
			order = append(order, n.Name)
		}
	}

	lists := make([][]bufNode, len(order))

	for _, n := range nodes {
		idx := seen[n.Name]
		lists[idx] = append(lists[idx], n)
	}

	groups := make([]nameGroup, len(order))

	for i, name := range order {
		groups[i] = nameGroup{Name: name, Elements: lists[i]}
	}

	return groups
}

// elementNode renders one element: its children as a mapping, or its text as a
// quoted scalar. tag is the element's section name, used to hoist its
// identifying child.
func elementNode(tag string, element bufNode) *yaml.Node {
	if len(element.Children) == 0 {
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Style: yaml.DoubleQuotedStyle,
			Value: element.Text,
		}
	}

	return mappingNode(element.Children, graph.SectionNameField(tag))
}

// findPermSets walks dir searching for files with the given suffix.
func findPermSets(dir, suffix string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}

		if d.IsDir() {
			return nil
		}

		if strings.HasSuffix(d.Name(), suffix) {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", dir, err)
	}

	return files, nil
}

// filterByPermSetNames returns only the perm set paths whose base name (without
// .permissionset-meta.xml) matches one of the given names. The available names are
// returned alongside for error reporting.
func filterByPermSetNames(permsets []string, names []string, suffix string) (matched []string, available []string, err error) {
	available = make([]string, 0, len(permsets))
	byName := make(map[string]string, len(permsets))

	for _, p := range permsets {
		base := filepath.Base(p)
		name := strings.TrimSuffix(base, suffix)
		available = append(available, name)
		byName[name] = p
	}

	missing := make([]string, 0, len(names))

	for _, n := range names {
		n = normalizeMetadataName(strings.TrimSpace(n))

		if p, ok := byName[n]; ok {
			matched = append(matched, p)
		} else {
			missing = append(missing, n)
		}
	}

	if len(missing) > 0 {
		sort.Strings(available)

		return nil, available, fmt.Errorf("permission sets not found: %s; available: %s",
			strings.Join(missing, ", "),
			strings.Join(available, ", "))
	}

	return matched, available, nil
}

// findAndFilterPermSets discovers all *.permissionset-meta.xml files under
// projectPath and filters them according to permsetName (single) or
// permsetsCSV (comma-separated list). If both are empty, all discovered perm
// sets are returned. permsetName and permsetsCSV must not both be set — the
// caller (CLI) enforces this via MarkFlagsMutuallyExclusive.
func findAndFilterPermSets(projectPath, permsetName, permsetsCSV, suffix string) ([]string, error) {
	all, err := findPermSets(projectPath, suffix)
	if err != nil {
		return nil, err
	}

	if permsetName != "" {
		matched, _, err := filterByPermSetNames(all, []string{permsetName}, suffix)
		if err != nil {
			return nil, err
		}

		return matched, nil
	}

	if permsetsCSV != "" {
		names := strings.Split(permsetsCSV, ",")

		for i := range names {
			names[i] = strings.TrimSpace(names[i])
		}

		matched, _, err := filterByPermSetNames(all, names, suffix)
		if err != nil {
			return nil, err
		}

		return matched, nil
	}

	return all, nil
}

// ConvertPermSets discovers *.permissionset-meta.xml files under projectPath,
// filters by permsetName (single) or permsetsCSV (comma-separated) if provided,
// converts each to YAML, writes the output to disk, and prints progress to stderr.
// permsetName and permsetsCSV must not both be set — the caller enforces this.
func ConvertPermSets(projectPath, permsetName, permsetsCSV, fromType string) error {
	direction, err := ParseFromType(fromType)
	if err != nil {
		return err
	}

	inSuffix, outSuffix := permSetSuffixes(direction)

	permsets, err := findAndFilterPermSets(projectPath, permsetName, permsetsCSV, inSuffix)
	if err != nil {
		return err
	}

	for _, permsetPath := range permsets {
		outPath, content, err := convertFile(permsetPath, direction, inSuffix, outSuffix, "PermissionSet")
		if err != nil {
			return fmt.Errorf("convert %s: %w", permsetPath, err)
		}
		if err := os.WriteFile(outPath, content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", outPath, err)
		}

		fmt.Fprintf(os.Stderr, "Converted: %s\n", filepath.Base(permsetPath))
	}
	fmt.Fprintf(os.Stderr, "Done. %d permset(s) converted.\n", len(permsets))

	return nil
}

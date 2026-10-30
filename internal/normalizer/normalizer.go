// Package normalizer backfills missing Master Schema entries into each profile,
// orders every top-level element and permission property by tag name the way
// Salesforce writes them, serializes to XML, and writes profiles to disk
// concurrently. Phase 3 of the 3-phase profile normalization pipeline.
package normalizer

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"xml2l/internal/graph"
)

// NormalizeGraph is the interface the normalizer requires to access profile
// data from the graph without depending on the concrete SalesforceGraph type.
type NormalizeGraph interface {
	Profiles() []*graph.ProfileNode
	ProfileEdges(p *graph.ProfileNode) []*graph.Edge
	MasterSchema() graph.MasterSchemaProvider
	AvailableLayouts() []string
	OrgSchema() graph.OrgSchemaProvider
	ExcludePatterns() []string
	EntryPointClasses() map[string]bool
}

// DefaultEdgeProperties returns an EdgeProperties with all boolean fields set to false
// for the given metadata type. This is used when backfilling missing Master Schema entries.
func DefaultEdgeProperties(mt graph.MetadataType) graph.EdgeProperties {
	f := false
	s := "Hidden"

	switch mt {
	case graph.MetaTypeApexClass, graph.MetaTypeAgent, graph.MetaTypeCustomMetadataType,
		graph.MetaTypeCustomPermission, graph.MetaTypeCustomSetting,
		graph.MetaTypeExternalDataSource, graph.MetaTypeFlow,
		graph.MetaTypeGenComputingSummaryDef, graph.MetaTypeServicePresenceStatus,
		graph.MetaTypeUserPerm, graph.MetaTypePage:
		return graph.EdgeProperties{Enabled: &f}
	case graph.MetaTypeApp, graph.MetaTypeRecordType:
		return graph.EdgeProperties{Visible: &f, Default: &f}
	case graph.MetaTypeField:
		return graph.EdgeProperties{Readable: &f, Editable: &f}
	case graph.MetaTypeObject:
		return graph.EdgeProperties{
			AllowCreate: &f, AllowDelete: &f, AllowRead: &f, AllowEdit: &f,
			ModifyAll: &f, ViewAll: &f,
		}
	case graph.MetaTypeCategoryGroup, graph.MetaTypeLayout:
		return graph.EdgeProperties{}
	case graph.MetaTypeTab:
		return graph.EdgeProperties{Visibility: &s}
	default:
		return graph.EdgeProperties{}
	}
}

// NormalizeProfile generates normalized XML for a single profile by backfilling
// missing Master Schema entries, sorting every top-level element and every
// permission property by tag name, and writing the result.
func NormalizeProfile(p *graph.ProfileNode, g NormalizeGraph, unknown *unknownReport) ([]byte, error) {
	if g.MasterSchema() == nil {
		// No MasterSchema available — fall back to raw XML.
		if p.RawXML != "" {
			return []byte(p.RawXML), nil
		}

		return nil, nil
	}

	rawEntries, err := buildRawEntryIndex(p.RawXML)
	if err != nil {
		return nil, err
	}

	blocks, err := profileBlocks(p, g, rawEntries, unknown)
	if err != nil {
		return nil, err
	}

	// Sort every top-level element by tag so output matches what Salesforce
	// writes on retrieve (ADR-0002). The sort is stable, so entries inside a
	// section keep their name order.
	sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].tag < blocks[j].tag })

	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	buf.WriteString(`<Profile xmlns="http://soap.sforce.com/2006/04/metadata">` + "\n")

	for _, b := range blocks {
		buf.WriteString(b.text)
	}

	buf.WriteString("</Profile>\n")

	return buf.Bytes(), nil
}

// profileBlock is one rendered top-level <Profile> child, tagged with the
// element name used to sort it into place.
type profileBlock struct {
	tag  string
	text string
}

// profileBlocks renders every top-level element the profile should contain:
// profile-level values, modelled sections (with backfill), and sections the
// normalizer does not model, preserved verbatim.
func profileBlocks(p *graph.ProfileNode, g NormalizeGraph, rawEntries rawEntryIndex, unknown *unknownReport) ([]profileBlock, error) {
	blocks := profileFieldBlocks(p)

	blocks = append(blocks, sectionBlocks(p, g, rawEntries, unknown)...)

	preserved, err := preservedBlocks(p.RawXML)
	if err != nil {
		return nil, err
	}

	blocks = append(blocks, preserved...)

	return blocks, nil
}

// preservedBlocks returns the top-level elements the normalizer does not model,
// re-indented and in document order, so a save never drops a section outside
// the graph's scope.
func preservedBlocks(rawXML string) ([]profileBlock, error) {
	elems, err := scanDirectChildren(rawXML)
	if err != nil {
		return nil, fmt.Errorf("scan profile sections: %w", err)
	}

	modeled := modeledTopLevelTags()

	var blocks []profileBlock

	for _, el := range elems {
		if modeled[el.tag] {
			continue
		}

		blocks = append(blocks, profileBlock{el.tag, reindentTo(el.body, 4)})
	}

	return blocks, nil
}

// profileFieldBlocks renders the profile-level values that live directly under
// <Profile> rather than inside a permission section.
func profileFieldBlocks(p *graph.ProfileNode) []profileBlock {
	var blocks []profileBlock

	if p.UserLicense != "" {
		blocks = append(blocks, profileBlock{"userLicense", "    <userLicense>" + xmlEscape(p.UserLicense) + "</userLicense>\n"})
	}

	if p.HasCustomTag {
		val := "false"
		if p.IsCustom {
			val = "true"
		}

		blocks = append(blocks, profileBlock{"custom", "    <custom>" + val + "</custom>\n"})
	}

	if p.HasDescription || p.Description != "" {
		blocks = append(blocks, profileBlock{"description", "    <description>" + xmlEscape(p.Description) + "</description>\n"})
	}

	return blocks
}

// sectionBlocks renders every permission entry of every modelled section,
// backfilling entries the profile lacks and filtering per org schema.
func sectionBlocks(p *graph.ProfileNode, g NormalizeGraph, rawEntries rawEntryIndex, unknown *unknownReport) []profileBlock {
	// Pre-group edges by MetadataType so the per-section loop doesn't
	// iterate all edges for every section type (O(sections × edges) → O(edges + sections)).
	type nameAndProps struct {
		name  string
		props graph.EdgeProperties
	}

	edgesByType := make(map[graph.MetadataType][]nameAndProps)

	for _, e := range g.ProfileEdges(p) {
		edgesByType[e.MetadataNode.MetaType] = append(edgesByType[e.MetadataNode.MetaType], nameAndProps{
			name:  e.MetadataNode.Name,
			props: e.Properties,
		})
	}

	var blocks []profileBlock

	// Process each section type in stable order.
	for _, sm := range graph.SectionMetaOrder() {
		sectionTag := sm.Tag
		metaType := sm.MetaType

		sectionEntries := edgesByType[metaType]
		existing := make(map[string]bool, len(sectionEntries))

		for _, e := range sectionEntries {
			existing[e.name] = true
		}

		entries := make([]nameAndProps, 0, len(sectionEntries))
		entries = append(entries, sectionEntries...)

		entryPoints := g.EntryPointClasses()

		switch {
		case sectionTag == "classAccesses" && entryPoints != nil:
			// Entry-point mode replaces class-access backfill: only classes
			// Salesforce invokes on behalf of the running user are written, and
			// always enabled. Excluded entries are copied from the source, so
			// they stay exactly as they are and are never pruned.
			patterns := g.ExcludePatterns()
			kept := make([]nameAndProps, 0, len(entryPoints))
			excluded := make(map[string]bool)

			for _, e := range sectionEntries {
				if len(patterns) > 0 && matchesExclude(e.name, patterns) {
					kept = append(kept, e)
					excluded[e.name] = true
				}
			}

			enabled := true

			for name := range entryPoints {
				if excluded[name] || (len(patterns) > 0 && matchesExclude(name, patterns)) {
					continue
				}

				kept = append(kept, nameAndProps{
					name:  name,
					props: graph.EdgeProperties{Enabled: &enabled},
				})
			}

			entries = kept

		case sectionTag != "layoutAssignments":
			// Backfill: add Master Schema entries missing from this profile.
			// Skip backfill for layoutAssignments — Salesforce requires at most
			// one layout per object without a recordType, so backfilling from
			// other profiles creates invalid configurations.
			msNamesList := g.MasterSchema().AllNames(sectionTag)

			for _, name := range msNamesList {
				if !existing[name] {
					entries = append(entries, nameAndProps{
						name:  name,
						props: DefaultEdgeProperties(metaType),
					})
				}
			}
		}

		// For layoutAssignments: add a default layout for objects with no entries.
		if sectionTag == "layoutAssignments" {
			available := g.AvailableLayouts()

			if len(available) > 0 {
				objectsHaveLayout := make(map[string]bool)

				for _, e := range entries {
					objectsHaveLayout[objectFromLayoutName(e.name)] = true
				}

				seenAdded := make(map[string]bool)

				for _, layout := range available {
					obj := objectFromLayoutName(layout)

					if !objectsHaveLayout[obj] && !seenAdded[obj] {
						seenAdded[obj] = true
						entries = append(entries, nameAndProps{
							name:  layout,
							props: graph.EdgeProperties{}, // no recordType
						})
					}
				}
			}
		}

		// Org schema filtering: if org schema is active for this type, skip
		// entries that don't exist in the org (overrides all other checks).
		if os := g.OrgSchema(); os != nil && os.HasType(string(metaType)) {
			filtered := make([]nameAndProps, 0, len(entries))

			for _, e := range entries {
				if os.Has(string(metaType), e.name) {
					filtered = append(filtered, e)
				}
			}

			entries = filtered
		}

		if len(entries) == 0 {
			continue
		}

		// Sort alphabetically by name, then by record type for deterministic output.
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].name != entries[j].name {
				return entries[i].name < entries[j].name
			}

			return strVal(entries[i].props.RecordType) < strVal(entries[j].props.RecordType)
		})

		// Filter layoutAssignments: enforce Salesforce deployment rules.
		// - At most one layout per object without a recordType.
		// - No duplicate recordType values.
		if sectionTag == "layoutAssignments" {
			seenNoRT := make(map[string]bool)
			seenRT := make(map[string]bool)
			filtered := make([]nameAndProps, 0, len(entries))

			for _, e := range entries {
				obj := objectFromLayoutName(e.name)
				rt := strVal(e.props.RecordType)

				if rt == "" {
					if !seenNoRT[obj] {
						seenNoRT[obj] = true
						filtered = append(filtered, e)
					}
				} else {
					if !seenRT[rt] {
						seenRT[rt] = true
						filtered = append(filtered, e)
					}
				}
			}

			entries = filtered
		}

		// Marshal entries in sorted order; excluded entries preserve original XML.
		patterns := g.ExcludePatterns()
		hasNameField := graph.SectionNameField(sectionTag) != ""

		for _, e := range entries {
			if len(patterns) > 0 && matchesExclude(e.name, patterns) {
				// Excluded entry: preserve original XML if it exists, otherwise drop.
				if hasNameField {
					if raw := rawEntries[sectionTag][e.name]; raw != "" {
						blocks = append(blocks, profileBlock{sectionTag, reindentTo(raw, 4)})
					}
				}
				// If no nameField mapping or entry not in raw XML, silently drop.
				continue
			}

			xmlBytes, err := renderEntry(sectionTag, e.name, e.props, rawEntries[sectionTag][e.name], unknown)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: marshal %s/%s: %v\n", sectionTag, e.name, err)
				continue
			}

			blocks = append(blocks, profileBlock{sectionTag, string(xmlBytes)})
		}
	}

	return blocks
}

// unknownReport counts permission properties preserved from the source that this
// build does not model. A run reports each (section, tag) once, so one shared
// property does not print a line per profile and entry.
type unknownReport struct {
	mu     sync.Mutex
	counts map[unknownKey]int
}

type unknownKey struct {
	section string
	tag     string
}

func newUnknownReport() *unknownReport {
	return &unknownReport{counts: make(map[unknownKey]int)}
}

// add records one preserved property. It is a no-op on a nil report, so callers
// that only need the bytes can pass nil.
func (r *unknownReport) add(section, tag string) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.counts[unknownKey{section, tag}]++
}

// print writes one line per (section, tag) in a stable order.
func (r *unknownReport) print(w io.Writer) {
	if r == nil {
		return
	}

	r.mu.Lock()

	keys := make([]unknownKey, 0, len(r.counts))
	for key := range r.counts {
		keys = append(keys, key)
	}

	counts := make(map[unknownKey]int, len(r.counts))
	for key, n := range r.counts {
		counts[key] = n
	}

	r.mu.Unlock()

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].section != keys[j].section {
			return keys[i].section < keys[j].section
		}

		return keys[i].tag < keys[j].tag
	})

	for _, key := range keys {
		unit := "entries"
		if counts[key] == 1 {
			unit = "entry"
		}

		fmt.Fprintf(w, "warning: %s: preserving unknown element <%s> in %d %s\n", key.section, key.tag, counts[key], unit)
	}
}

// ClassAccessChange is what entry-point mode does to one profile's class access.
type ClassAccessChange struct {
	Profile string
	Before  int // enabled class entries before pruning
	After   int // class entries written after pruning
	Removed int
	Added   int
}

// ClassAccessPlan reports what SetEntryPointClasses would change for every
// profile. It reads the same inputs as sectionBlocks and writes nothing.
func ClassAccessPlan(g NormalizeGraph) []ClassAccessChange {
	required := g.EntryPointClasses()
	if required == nil {
		return nil
	}

	patterns := g.ExcludePatterns()
	profiles := g.Profiles()

	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })

	changes := make([]ClassAccessChange, 0, len(profiles))

	for _, p := range profiles {
		rawEntries, _ := buildRawEntryIndex(p.RawXML)

		enabled := make(map[string]bool)
		after := make(map[string]bool)

		for _, e := range g.ProfileEdges(p) {
			if e.MetadataNode.MetaType != graph.MetaTypeApexClass {
				continue
			}

			name := e.MetadataNode.Name
			if e.Properties.Enabled != nil && *e.Properties.Enabled {
				enabled[name] = true
			}

			if len(patterns) > 0 && matchesExclude(name, patterns) {
				// Excluded entries are copied from the source; without a source
				// body there is nothing to write.
				if rawEntries["classAccesses"][name] != "" {
					after[name] = true
				}

				continue
			}
		}

		for name := range required {
			if len(patterns) > 0 && matchesExclude(name, patterns) {
				continue
			}

			after[name] = true
		}

		change := ClassAccessChange{Profile: p.Name, Before: len(enabled), After: len(after)}

		for name := range enabled {
			if !after[name] {
				change.Removed++
			}
		}

		for name := range after {
			if !enabled[name] {
				change.Added++
			}
		}

		changes = append(changes, change)
	}

	return changes
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// entryChild is one permission property rendered at its final indentation
// inside an entry.
type entryChild struct {
	tag  string
	text string
}

// childLine renders a single permission property at entry-child indentation.
func childLine(tag, value string) entryChild {
	return entryChild{tag: tag, text: "        <" + tag + ">" + xmlEscape(value) + "</" + tag + ">\n"}
}

// boolChild renders a boolean permission property. A nil flag renders false,
// matching how the tool has always written a flag the source left out.
func boolChild(tag string, v *bool) entryChild {
	return childLine(tag, strconv.FormatBool(boolVal(v)))
}

// entryChildren renders the permission properties modelled for one entry.
func entryChildren(sectionTag, name string, props graph.EdgeProperties) ([]entryChild, error) {
	switch sectionTag {
	case "classAccesses":
		return []entryChild{childLine("apexClass", name), boolChild("enabled", props.Enabled)}, nil
	case "agentAccesses":
		return []entryChild{childLine("agent", name), boolChild("enabled", props.Enabled)}, nil
	case "applicationVisibilities":
		return []entryChild{
			childLine("application", name), boolChild("default", props.Default), boolChild("visible", props.Visible),
		}, nil
	case "categoryGroupVisibilities":
		return []entryChild{childLine("dataCategoryGroup", name), childLine("visibility", strVal(props.Visibility))}, nil
	case "customMetadataTypeAccesses", "customPermissions", "customSettingAccesses":
		return []entryChild{childLine("name", name), boolChild("enabled", props.Enabled)}, nil
	case "fieldPermissions":
		return []entryChild{
			childLine("field", name), boolChild("readable", props.Readable), boolChild("editable", props.Editable),
		}, nil
	case "flowAccesses":
		return []entryChild{childLine("flow", name), boolChild("enabled", props.Enabled)}, nil
	case "genComputingSummaryDefAccess":
		return []entryChild{childLine("enhancedSummaryDef", name), boolChild("enabled", props.Enabled)}, nil
	case "externalDataSourceAccesses":
		return []entryChild{childLine("externalDataSource", name), boolChild("enabled", props.Enabled)}, nil
	case "layoutAssignments":
		children := []entryChild{childLine("layout", name)}

		if rt := strVal(props.RecordType); rt != "" {
			children = append(children, childLine("recordType", rt))
		}

		return children, nil
	case "objectPermissions":
		children := []entryChild{
			boolChild("allowCreate", props.AllowCreate),
			boolChild("allowDelete", props.AllowDelete),
			boolChild("allowRead", props.AllowRead),
			boolChild("allowEdit", props.AllowEdit),
			boolChild("modifyAllRecords", props.ModifyAll),
			childLine("object", name),
			boolChild("viewAllRecords", props.ViewAll),
		}

		// viewAllFields is preserved, never backfilled (ADR-0003): emit it only
		// when the source entry carried it.
		if props.ViewAllFields != nil {
			children = append(children, boolChild("viewAllFields", props.ViewAllFields))
		}

		return children, nil
	case "pageAccesses":
		return []entryChild{childLine("apexPage", name), boolChild("enabled", props.Enabled)}, nil
	case "recordTypeVisibilities":
		return []entryChild{
			childLine("recordType", name), boolChild("visible", props.Visible), boolChild("default", props.Default),
		}, nil
	case "servicePresenceStatusAccesses":
		return []entryChild{childLine("servicePresenceStatus", name), boolChild("enabled", props.Enabled)}, nil
	case "tabVisibilities":
		return []entryChild{childLine("tab", name), childLine("visibility", strVal(props.Visibility))}, nil
	case "userPermissions":
		return []entryChild{childLine("name", name), boolChild("enabled", props.Enabled)}, nil
	default:
		return nil, fmt.Errorf("unknown section tag: %s", sectionTag)
	}
}

// renderEntry renders one entry, merging the properties modelled in the graph
// with any property this build does not model that the source entry carried.
// Unknown properties are preserved verbatim and reported on stderr so they can
// be modelled instead of silently lost.
func renderEntry(sectionTag, name string, props graph.EdgeProperties, rawEntry string, unknown *unknownReport) ([]byte, error) {
	children, err := entryChildren(sectionTag, name, props)
	if err != nil {
		return nil, err
	}

	if rawEntry != "" {
		known := make(map[string]bool, len(children))
		for _, c := range children {
			known[c.tag] = true
		}

		if rawChildren, err := scanDirectChildren(rawEntry); err == nil {
			for _, rc := range rawChildren {
				if known[rc.tag] {
					continue
				}

				unknown.add(sectionTag, rc.tag)

				children = append(children, entryChild{tag: rc.tag, text: reindentTo(rc.body, 8)})
			}
		}
	}

	sort.SliceStable(children, func(i, j int) bool { return children[i].tag < children[j].tag })

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "    <%s>\n", sectionTag)

	for _, c := range children {
		buf.WriteString(c.text)
	}

	fmt.Fprintf(&buf, "    </%s>\n", sectionTag)

	return buf.Bytes(), nil
}

func boolVal(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

func strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// objectFromLayoutName extracts the object name from a layout name.
// Layout names follow the pattern "ObjectName-LayoutFriendlyName".
// If no hyphen is present, the full name is returned.
func objectFromLayoutName(layoutName string) string {
	if idx := strings.Index(layoutName, "-"); idx >= 0 {
		return layoutName[:idx]
	}
	return layoutName
}

// matchesExclude returns true if the entry name starts with any of the exclude
// patterns (case-insensitive prefix match). Patterns are expected to be
// pre-lowercased by the caller.
func matchesExclude(name string, patterns []string) bool {
	lowerName := strings.ToLower(name)

	for _, p := range patterns {
		if strings.HasPrefix(lowerName, p) {
			return true
		}
	}

	return false
}

// rawEntryIndex maps a section tag to its entries' raw XML, keyed by the
// section's name field. It is built once per profile so rendering an entry can
// reach the properties the graph does not model without re-scanning the
// document for every entry.
type rawEntryIndex map[string]map[string]string

// buildRawEntryIndex indexes every entry of every section that has a name field.
func buildRawEntryIndex(rawXML string) (rawEntryIndex, error) {
	idx := make(rawEntryIndex)

	elems, err := scanDirectChildren(rawXML)
	if err != nil {
		return nil, err
	}

	for _, el := range elems {
		nameField := graph.SectionNameField(el.tag)
		if nameField == "" {
			continue
		}

		name := entryName(el.body, nameField)
		if name == "" {
			continue
		}

		if idx[el.tag] == nil {
			idx[el.tag] = make(map[string]string)
		}

		if _, exists := idx[el.tag][name]; !exists {
			idx[el.tag][name] = el.body
		}
	}

	return idx, nil
}

// entryName returns the text of an entry's name child, or "" when the entry
// does not carry one. It tokenizes the entry the same way as the document
// rather than guessing at the name field with a regular expression.
func entryName(body, nameField string) string {
	children, err := scanDirectChildren(body)
	if err != nil {
		return ""
	}

	for _, child := range children {
		if child.tag != nameField {
			continue
		}

		open := strings.Index(child.body, ">")
		close := strings.LastIndex(child.body, "<")

		if open < 0 || close <= open {
			return ""
		}

		return strings.TrimSpace(child.body[open+1 : close])
	}

	return ""
}

// modeledTopLevelTags returns the top-level <Profile> child tags the normalizer
// writes itself. Every other top-level element is preserved verbatim, so
// sections the normalizer does not model (loginHours, loginIpRanges,
// loginFlows, profileActionOverrides, fieldLevelSecurities, ...) survive a save.
func modeledTopLevelTags() map[string]bool {
	tags := map[string]bool{
		"userLicense": true,
		"custom":      true,
		"description": true,
	}

	for _, sm := range graph.SectionMetaOrder() {
		tags[sm.Tag] = true
	}

	return tags
}

// rawElement is a top-level child element captured verbatim.
type rawElement struct {
	tag  string
	body string
}

// scanDirectChildren returns the raw text of every direct child of the document
// root, in document order. Offsets come from the tokenizer, so nested elements
// and repeated tags are captured exactly as they appear in the source.
func scanDirectChildren(rawXML string) ([]rawElement, error) {
	if rawXML == "" {
		return nil, nil
	}

	dec := xml.NewDecoder(strings.NewReader(rawXML))

	var (
		elems    []rawElement
		depth    int
		start    = int64(-1)
		startTag string
	)

	for {
		before := dec.InputOffset()

		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if depth == 1 {
				start = nextTagOffset(rawXML, before)
				startTag = t.Name.Local
			}

			depth++
		case xml.EndElement:
			depth--

			if depth == 1 && start >= 0 {
				elems = append(elems, rawElement{tag: startTag, body: rawXML[start:dec.InputOffset()]})
				start = -1
			}
		}
	}

	return elems, nil
}

// nextTagOffset returns the offset of the next '<' at or after from. Callers
// pass the tokenizer offset captured just before reading a StartElement, which
// sits immediately after the preceding token (whitespace and comments are
// tokens too), so the next '<' begins that element.
func nextTagOffset(s string, from int64) int64 {
	i := int(from)

	for i < len(s) && s[i] != '<' {
		i++
	}

	return int64(i)
}

// reindentTo normalizes a block's base indentation to base spaces while
// preserving the relative depth of nested elements. The block's first line is
// its opening tag and its last non-empty line is its closing tag.
func reindentTo(block string, base int) string {
	lines := strings.Split(block, "\n")
	// Find the closing tag's indentation (last non-empty line).
	baseIndent := 0

	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "" {
			baseIndent = len(lines[i]) - len(trimmed)
			break
		}
	}

	shift := base - baseIndent
	var buf bytes.Buffer

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if i == 0 {
			buf.WriteString(strings.Repeat(" ", base) + trimmed + "\n")
			continue
		}

		newIndent := len(line) - len(strings.TrimLeft(line, " \t")) + shift
		if newIndent < 0 {
			newIndent = 0
		}

		buf.WriteString(strings.Repeat(" ", newIndent) + trimmed + "\n")
	}

	return buf.String()
}

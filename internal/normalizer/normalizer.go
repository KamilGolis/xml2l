// Package normalizer backfills missing Master Schema entries into each profile,
// sorts all sections alphabetically, serializes to XML, and writes profiles
// to disk concurrently. Phase 3 of the 3-phase profile normalization pipeline.
package normalizer

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"xml2l/internal/graph"
)

var (
	reLoginHours       = regexp.MustCompile(`(?s)<loginHours>.*?</loginHours>`)
	reLoginIpRanges    = regexp.MustCompile(`(?s)<loginIpRanges>.*?</loginIpRanges>`)
	reLoginFlows       = regexp.MustCompile(`(?s)<loginFlows>.*?</loginFlows>`)
	reProfileActionOvr = regexp.MustCompile(`(?s)<profileActionOverrides>.*?</profileActionOverrides>`)
)

// -- XML marshaling structs for each section type.

type classAccessXML struct {
	XMLName   xml.Name `xml:"classAccesses"`
	ApexClass string   `xml:"apexClass"`
	Enabled   bool     `xml:"enabled"`
}

type agentAccessXML struct {
	XMLName xml.Name `xml:"agentAccesses"`
	Agent   string   `xml:"agent"`
	Enabled bool     `xml:"enabled"`
}

type appVisibilityXML struct {
	XMLName     xml.Name `xml:"applicationVisibilities"`
	Application string   `xml:"application"`
	Default     bool     `xml:"default"`
	Visible     bool     `xml:"visible"`
}

type categoryGroupXML struct {
	XMLName           xml.Name `xml:"categoryGroupVisibilities"`
	DataCategoryGroup string   `xml:"dataCategoryGroup"`
	Visibility        string   `xml:"visibility"`
}

type customMetadataTypeXML struct {
	XMLName xml.Name `xml:"customMetadataTypeAccesses"`
	Name    string   `xml:"name"`
	Enabled bool     `xml:"enabled"`
}

type customPermissionXML struct {
	XMLName xml.Name `xml:"customPermissions"`
	Name    string   `xml:"name"`
	Enabled bool     `xml:"enabled"`
}

type customSettingAccessXML struct {
	XMLName xml.Name `xml:"customSettingAccesses"`
	Name    string   `xml:"name"`
	Enabled bool     `xml:"enabled"`
}

type fieldPermXML struct {
	XMLName  xml.Name `xml:"fieldPermissions"`
	Field    string   `xml:"field"`
	Readable bool     `xml:"readable"`
	Editable bool     `xml:"editable"`
}

type layoutXML struct {
	XMLName    xml.Name `xml:"layoutAssignments"`
	Layout     string   `xml:"layout"`
	RecordType string   `xml:"recordType,omitempty"`
}

type objectPermXML struct {
	XMLName     xml.Name `xml:"objectPermissions"`
	Object      string   `xml:"object"`
	AllowCreate bool     `xml:"allowCreate"`
	AllowDelete bool     `xml:"allowDelete"`
	AllowRead   bool     `xml:"allowRead"`
	AllowEdit   bool     `xml:"allowEdit"`
	ModifyAll   bool     `xml:"modifyAllRecords"`
	ViewAll     bool     `xml:"viewAllRecords"`
}

type recordTypeXML struct {
	XMLName    xml.Name `xml:"recordTypeVisibilities"`
	RecordType string   `xml:"recordType"`
	Visible    bool     `xml:"visible"`
	Default    bool     `xml:"default"`
}

type servicePresenceXML struct {
	XMLName               xml.Name `xml:"servicePresenceStatusAccesses"`
	ServicePresenceStatus string   `xml:"servicePresenceStatus"`
	Enabled               bool     `xml:"enabled"`
}

type tabVisibilityXML struct {
	XMLName    xml.Name `xml:"tabVisibilities"`
	Tab        string   `xml:"tab"`
	Visibility string   `xml:"visibility"`
}

type userPermXML struct {
	XMLName xml.Name `xml:"userPermissions"`
	Name    string   `xml:"name"`
	Enabled bool     `xml:"enabled"`
}

type pageAccessXML struct {
	XMLName  xml.Name `xml:"pageAccesses"`
	ApexPage string   `xml:"apexPage"`
	Enabled  bool     `xml:"enabled"`
}

type flowAccessXML struct {
	XMLName xml.Name `xml:"flowAccesses"`
	Flow    string   `xml:"flow"`
	Enabled bool     `xml:"enabled"`
}

type extDataSourceXML struct {
	XMLName            xml.Name `xml:"externalDataSourceAccesses"`
	ExternalDataSource string   `xml:"externalDataSource"`
	Enabled            bool     `xml:"enabled"`
}

type enhancedSummaryXML struct {
	XMLName            xml.Name `xml:"genComputingSummaryDefAccess"`
	EnhancedSummaryDef string   `xml:"enhancedSummaryDef"`
	Enabled            bool     `xml:"enabled"`
}

// -- Helpers

// NormalizeGraph is the interface the normalizer requires to access profile
// data from the graph without depending on the concrete SalesforceGraph type.
type NormalizeGraph interface {
	Profiles() []*graph.ProfileNode
	ProfileEdges(p *graph.ProfileNode) []*graph.Edge
	MasterSchema() graph.MasterSchemaProvider
}

// -- DefaultEdgeProperties

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
// missing Master Schema entries, sorting all sections, and marshaling to XML.
func NormalizeProfile(p *graph.ProfileNode, g NormalizeGraph) []byte {
	if g.MasterSchema() == nil {
		// No MasterSchema available — fall back to raw XML.
		if p.RawXML != "" {
			return []byte(p.RawXML)
		}
		return nil
	}

	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	buf.WriteString(`<Profile xmlns="http://soap.sforce.com/2006/04/metadata">` + "\n")

	// Profile-level fields.
	writeProfileFields(&buf, p)

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

		// Backfill: add Master Schema entries missing from this profile.
		msNamesList := g.MasterSchema().AllNames(sectionTag)
		for _, name := range msNamesList {
			if !existing[name] {
				entries = append(entries, nameAndProps{
					name:  name,
					props: DefaultEdgeProperties(metaType),
				})
			}
		}

		if len(entries) == 0 {
			continue
		}

		// Sort alphabetically by name.
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].name < entries[j].name
		})

		// Marshal each entry.
		for _, e := range entries {
			xmlBytes, err := marshalEntry(sectionTag, e.name, e.props)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: marshal %s/%s: %v\n", sectionTag, e.name, err)
				continue
			}
			buf.Write(xmlBytes)
		}
	}

	// Append unmapped sections.
	unmapped := extractUnmappedSections(p.RawXML)
	buf.WriteString(unmapped)

	buf.WriteString("</Profile>\n")
	return buf.Bytes()
}

// writeProfileFields writes profile-level XML elements.
func writeProfileFields(buf *bytes.Buffer, p *graph.ProfileNode) {
	if p.UserLicense != "" {
		buf.WriteString("    <userLicense>" + xmlEscape(p.UserLicense) + "</userLicense>\n")
	}
	if p.HasCustomTag {
		val := "false"
		if p.IsCustom {
			val = "true"
		}
		buf.WriteString("    <custom>" + val + "</custom>\n")
	}
	if p.Description != "" {
		buf.WriteString("    <description>" + xmlEscape(p.Description) + "</description>\n")
	}
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// marshalEntry marshals a single permission entry to XML bytes with 4-space indent.
func marshalEntry(sectionTag, name string, props graph.EdgeProperties) ([]byte, error) {
	var v interface{}

	switch sectionTag {
	case "classAccesses":
		v = classAccessXML{ApexClass: name, Enabled: boolVal(props.Enabled)}
	case "agentAccesses":
		v = agentAccessXML{Agent: name, Enabled: boolVal(props.Enabled)}
	case "applicationVisibilities":
		v = appVisibilityXML{Application: name, Default: boolVal(props.Default), Visible: boolVal(props.Visible)}
	case "categoryGroupVisibilities":
		vis := ""
		if props.Visibility != nil {
			vis = *props.Visibility
		}
		v = categoryGroupXML{DataCategoryGroup: name, Visibility: vis}
	case "customMetadataTypeAccesses":
		v = customMetadataTypeXML{Name: name, Enabled: boolVal(props.Enabled)}
	case "customPermissions":
		v = customPermissionXML{Name: name, Enabled: boolVal(props.Enabled)}
	case "customSettingAccesses":
		v = customSettingAccessXML{Name: name, Enabled: boolVal(props.Enabled)}
	case "fieldPermissions":
		v = fieldPermXML{Field: name, Readable: boolVal(props.Readable), Editable: boolVal(props.Editable)}
	case "flowAccesses":
		v = flowAccessXML{Flow: name, Enabled: boolVal(props.Enabled)}
	case "genComputingSummaryDefAccess":
		v = enhancedSummaryXML{EnhancedSummaryDef: name, Enabled: boolVal(props.Enabled)}
	case "externalDataSourceAccesses":
		v = extDataSourceXML{ExternalDataSource: name, Enabled: boolVal(props.Enabled)}
	case "layoutAssignments":
		rt := ""
		if props.RecordType != nil {
			rt = *props.RecordType
		}
		v = layoutXML{Layout: name, RecordType: rt}
	case "objectPermissions":
		v = objectPermXML{
			Object:      name,
			AllowCreate: boolVal(props.AllowCreate), AllowDelete: boolVal(props.AllowDelete),
			AllowRead: boolVal(props.AllowRead), AllowEdit: boolVal(props.AllowEdit),
			ModifyAll: boolVal(props.ModifyAll), ViewAll: boolVal(props.ViewAll),
		}
	case "pageAccesses":
		v = pageAccessXML{ApexPage: name, Enabled: boolVal(props.Enabled)}
	case "recordTypeVisibilities":
		v = recordTypeXML{RecordType: name, Visible: boolVal(props.Visible), Default: boolVal(props.Default)}
	case "servicePresenceStatusAccesses":
		v = servicePresenceXML{ServicePresenceStatus: name, Enabled: boolVal(props.Enabled)}
	case "tabVisibilities":
		v = tabVisibilityXML{Tab: name, Visibility: strVal(props.Visibility)}
	case "userPermissions":
		v = userPermXML{Name: name, Enabled: boolVal(props.Enabled)}
	default:
		return nil, fmt.Errorf("unknown section tag: %s", sectionTag)
	}

	out, err := xml.MarshalIndent(v, "    ", "    ")
	if err != nil {
		return nil, err
	}
	// xml.MarshalIndent produces leading indent, but we want the element at 4-space depth.
	// The MarshalIndent prefix is "    " (4 spaces), so the element already has correct indent.
	// Add newline after the element.
	return append(out, '\n'), nil
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

// extractUnmappedSections extracts unmapped sections from raw XML for preservation.
// These sections are not modeled in the graph but must be preserved verbatim.
func extractUnmappedSections(rawXML string) string {
	if rawXML == "" {
		return ""
	}
	var result strings.Builder
	for _, re := range []*regexp.Regexp{reLoginHours, reLoginIpRanges, reLoginFlows, reProfileActionOvr} {
		for _, m := range re.FindAllString(rawXML, -1) {
			result.WriteString(indentXMLSection(m))
		}
	}
	return strings.TrimSpace(result.String())
}

// indentXMLSection adds 4-space indent to each line of an XML fragment.
func indentXMLSection(s string) string {
	lines := strings.Split(s, "\n")
	var buf bytes.Buffer
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		buf.WriteString("    " + trimmed + "\n")
	}
	return buf.String()
}

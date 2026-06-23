// Package graph implements a bipartite graph model for Salesforce profile
// permissions. ProfileNodes represent .profile-meta.xml files and are connected
// via Edges to MetadataNodes that represent permission targets (classes,
// fields, objects, tabs, etc.).
package graph

// MetadataType enumerates the categories of permission targets found in
// Salesforce profile definitions.
type MetadataType string

const (
	MetaTypeApexClass              MetadataType = "ApexClass"
	MetaTypeField                  MetadataType = "CustomField"
	MetaTypeObject                 MetadataType = "CustomObject"
	MetaTypePage                   MetadataType = "ApexPage"
	MetaTypeRecordType             MetadataType = "RecordType"
	MetaTypeTab                    MetadataType = "CustomTab"
	MetaTypeApp                    MetadataType = "CustomApplication"
	MetaTypeUserPerm               MetadataType = "UserPermission"
	MetaTypeLayout                 MetadataType = "Layout"
	MetaTypeCustomMetadataType     MetadataType = "CustomMetadata"
	MetaTypeCustomPermission       MetadataType = "CustomPermission"
	MetaTypeCustomSetting          MetadataType = "CustomSetting"
	MetaTypeFlow                   MetadataType = "Flow"
	MetaTypeAgent                  MetadataType = "Agent"
	MetaTypeCategoryGroup          MetadataType = "CategoryGroup"
	MetaTypeExternalDataSource     MetadataType = "ExternalDataSource"
	MetaTypeServicePresenceStatus  MetadataType = "ServicePresenceStatus"
	MetaTypeGenComputingSummaryDef MetadataType = "GenComputingSummaryDef"
)

// ProfileNode represents a single .profile-meta.xml file in the graph.
type ProfileNode struct {
	Name         string
	SourcePath   string
	UserLicense  string // value of <userLicense>
	Description  string // value of <description>
	IsCustom     bool   // value of <custom>; false when tag absent
	HasCustomTag bool   // true when <custom> tag was present (distinguish absent from false)
	RawXML       string // original XML content for round-trip preservation
}

// MetadataNode represents a permission target that profiles reference.
type MetadataNode struct {
	Name     string
	MetaType MetadataType
}

// EdgeProperties holds the optional boolean permission flags on an edge.
// A nil pointer means the flag was absent in the source XML (not specified).
type EdgeProperties struct {
	Enabled     *bool
	Readable    *bool
	Editable    *bool
	AllowCreate *bool
	AllowDelete *bool
	AllowRead   *bool
	AllowEdit   *bool
	ModifyAll   *bool
	ViewAll     *bool
	Default     *bool
	Visible     *bool
	Hidden      *bool   // fieldLevelSecurities (API ≤22) hidden flag
	Visibility  *string // categoryGroupVisibilities visibility ("Hidden"/"DefaultOff"/"DefaultOn")
	RecordType  *string // layoutAssignment RecordType
}

// Edge connects a ProfileNode to a MetadataNode with associated permission
// properties.
type Edge struct {
	ProfileNode  *ProfileNode
	MetadataNode *MetadataNode
	Properties   EdgeProperties
}

// MasterSchemaProvider is the interface the graph expects for schema-backed
// operations such as normalization backfilling. It exposes a single method
type MasterSchemaProvider interface {
	AllNames(tag string) []string
}

// OrgSchemaProvider is the interface for checking metadata existence against
// a Salesforce org schema. Implementations are provided by the orgschema package.
type OrgSchemaProvider interface {
	Has(xmlName, fullName string) bool
	HasType(xmlName string) bool
}

// SalesforceGraph is a bipartite graph of profile and metadata nodes with
// indexed edges for fast lookup from either side.
type SalesforceGraph struct {
	ProfileNodes     map[string]*ProfileNode  // keyed by profile name
	MetadataNodes    map[string]*MetadataNode // keyed by "MetaType:Name"
	Edges            []*Edge
	ProfileToEdges   map[*ProfileNode][]*Edge
	MetaToEdges      map[*MetadataNode][]*Edge
	masterSchema     MasterSchemaProvider
	availableLayouts []string
	orgSchema        OrgSchemaProvider
	excludePatterns  []string
}

// NewGraph creates an empty SalesforceGraph.
func NewGraph() *SalesforceGraph {
	return &SalesforceGraph{
		ProfileNodes:   make(map[string]*ProfileNode),
		MetadataNodes:  make(map[string]*MetadataNode),
		Edges:          make([]*Edge, 0),
		ProfileToEdges: make(map[*ProfileNode][]*Edge),
		MetaToEdges:    make(map[*MetadataNode][]*Edge),
	}
}

// AddProfile creates a ProfileNode and registers it in the graph. If a profile
// with the same name already exists, the existing node is returned.
func (g *SalesforceGraph) AddProfile(name, path string) *ProfileNode {
	if existing, ok := g.ProfileNodes[name]; ok {
		return existing
	}
	p := &ProfileNode{Name: name, SourcePath: path}
	g.ProfileNodes[name] = p
	return p
}

// GetOrCreateMetadataNode returns the MetadataNode for the given type and name
// pair, creating it if it does not exist.
func (g *SalesforceGraph) GetOrCreateMetadataNode(metaType MetadataType, name string) *MetadataNode {
	key := string(metaType) + ":" + name
	if existing, ok := g.MetadataNodes[key]; ok {
		return existing
	}
	m := &MetadataNode{Name: name, MetaType: metaType}
	g.MetadataNodes[key] = m
	return m
}

// AddEdge creates an Edge from a ProfileNode to a MetadataNode and registers
// it in the graph's indexes.
func (g *SalesforceGraph) AddEdge(p *ProfileNode, m *MetadataNode, props EdgeProperties) *Edge {
	e := &Edge{
		ProfileNode:  p,
		MetadataNode: m,
		Properties:   props,
	}
	g.Edges = append(g.Edges, e)
	g.ProfileToEdges[p] = append(g.ProfileToEdges[p], e)
	g.MetaToEdges[m] = append(g.MetaToEdges[m], e)
	return e
}

// Profiles returns all ProfileNodes in the graph as a slice.
func (g *SalesforceGraph) Profiles() []*ProfileNode {
	r := make([]*ProfileNode, 0, len(g.ProfileNodes))
	for _, p := range g.ProfileNodes {
		r = append(r, p)
	}
	return r
}

// ProfileEdges returns all edges originating from the given profile node.
func (g *SalesforceGraph) ProfileEdges(p *ProfileNode) []*Edge {
	return g.ProfileToEdges[p]
}

// MasterSchema returns the graph's master schema provider.
func (g *SalesforceGraph) MasterSchema() MasterSchemaProvider {
	return g.masterSchema
}

// SetMasterSchema sets the graph's master schema provider.
func (g *SalesforceGraph) SetMasterSchema(ms MasterSchemaProvider) {
	g.masterSchema = ms
}

// SetAvailableLayouts stores the list of layout names available on disk.
func (g *SalesforceGraph) SetAvailableLayouts(layouts []string) {
	g.availableLayouts = layouts
}

// AvailableLayouts returns the list of layout names available on disk, sorted.
func (g *SalesforceGraph) AvailableLayouts() []string {
	return g.availableLayouts
}

// SetOrgSchema stores the org schema provider for org-based filtering.
func (g *SalesforceGraph) SetOrgSchema(os OrgSchemaProvider) {
	g.orgSchema = os
}

// SetExcludePatterns stores the exclude patterns for normalization filtering.
func (g *SalesforceGraph) SetExcludePatterns(patterns []string) {
	g.excludePatterns = patterns
}

// OrgSchema returns the org schema provider for org-based filtering.
// Returns nil when not set — the normalizer skips org filtering in that case.
func (g *SalesforceGraph) OrgSchema() OrgSchemaProvider {
	return g.orgSchema
}

// ExcludePatterns returns the exclude patterns for normalization filtering.
func (g *SalesforceGraph) ExcludePatterns() []string {
	return g.excludePatterns
}

// -- Canonical metadata type registry

// SectionMeta maps an XML section tag to its corresponding MetadataType.
type SectionMeta struct {
	Tag      string
	MetaType MetadataType
}

// sectionOrder defines the canonical ordering of all supported profile XML
// section types and their MetadataType correspondence.
var sectionOrder = []SectionMeta{
	{"agentAccesses", MetaTypeAgent},
	{"applicationVisibilities", MetaTypeApp},
	{"categoryGroupVisibilities", MetaTypeCategoryGroup},
	{"classAccesses", MetaTypeApexClass},
	{"customMetadataTypeAccesses", MetaTypeCustomMetadataType},
	{"customPermissions", MetaTypeCustomPermission},
	{"customSettingAccesses", MetaTypeCustomSetting},
	{"externalDataSourceAccesses", MetaTypeExternalDataSource},
	{"fieldPermissions", MetaTypeField},
	{"flowAccesses", MetaTypeFlow},
	{"genComputingSummaryDefAccess", MetaTypeGenComputingSummaryDef},
	{"layoutAssignments", MetaTypeLayout},
	{"objectPermissions", MetaTypeObject},
	{"pageAccesses", MetaTypePage},
	{"recordTypeVisibilities", MetaTypeRecordType},
	{"servicePresenceStatusAccesses", MetaTypeServicePresenceStatus},
	{"tabVisibilities", MetaTypeTab},
	{"userPermissions", MetaTypeUserPerm},
}

// SectionMetaOrder returns a copy of the canonical section ordering.
func SectionMetaOrder() []SectionMeta {
	return append([]SectionMeta(nil), sectionOrder...)
}

// tagMetaType maps a profile XML section tag to its graph MetadataType.
var tagMetaType = buildTagMetaType()

func buildTagMetaType() map[string]MetadataType {
	m := make(map[string]MetadataType, len(sectionOrder))
	for _, sm := range sectionOrder {
		m[sm.Tag] = sm.MetaType
	}
	return m
}

// TagToMetaType maps a profile XML section tag to its canonical MetadataType.
// Returns the type verbatim if unknown.
func TagToMetaType(tag string) MetadataType {
	if mt, ok := tagMetaType[tag]; ok {
		return mt
	}
	return MetadataType(tag)
}

// DirMapping describes filesystem mapping for a metadata type.
type DirMapping struct {
	Dir       string
	Patterns  []string
	Recursive bool
}

// metaTypeDirMap defines the filesystem layout for each metadata type.
var metaTypeDirMap = map[MetadataType]DirMapping{
	MetaTypeApexClass:          {Dir: "classes", Patterns: []string{"*.cls", "*.cls-meta.xml"}, Recursive: true},
	MetaTypeApp:                {Dir: "apps", Patterns: []string{"*.app", "*.app-meta.xml"}},
	MetaTypeTab:                {Dir: "tabs", Patterns: []string{"*.tab", "*.tab-meta.xml"}},
	MetaTypeLayout:             {Dir: "layouts", Patterns: []string{"*.layout"}},
	MetaTypeCustomPermission:   {Dir: "customPermissions", Patterns: []string{"*.customPermission", "*.customPermission-meta.xml"}},
	MetaTypeFlow:               {Dir: "flows", Patterns: []string{"*.flow-meta.xml"}},
	MetaTypePage:               {Dir: "pages", Patterns: []string{"*.page", "*.page-meta.xml"}},
	MetaTypeObject:             {Dir: "objects", Patterns: []string{"*.object-meta.xml"}},
	MetaTypeCustomSetting:      {Dir: "objects", Patterns: []string{"*__c.object-meta.xml"}},
	MetaTypeCustomMetadataType: {Dir: "objects", Patterns: []string{"*__mdt.object-meta.xml"}},
}

// DirMappingFor returns the filesystem directory mapping for a metadata type.
// The second return value is false when no mapping exists.
func DirMappingFor(mt MetadataType) (DirMapping, bool) {
	m, ok := metaTypeDirMap[mt]
	return m, ok
}

// MetaTypeDirMap returns the full metadata-type-to-directory mapping.
func MetaTypeDirMap() map[MetadataType]DirMapping {
	return metaTypeDirMap
}

// ObjectSubDirs lists subdirectories under objects/<Name>/ that correspond to
// profile permission types with their MetadataType and glob pattern.
var ObjectSubDirs = []struct {
	SubDir   string
	MetaType MetadataType
	Pattern  string
}{
	{SubDir: "fields", MetaType: MetaTypeField, Pattern: "*.field-meta.xml"},
	{SubDir: "recordTypes", MetaType: MetaTypeRecordType, Pattern: "*.recordType-meta.xml"},
}

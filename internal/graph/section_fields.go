package graph

// sectionNameFields maps a profile section tag to the element that identifies
// one of its entries, for example fieldPermissions -> field. It is the single
// source of truth for "what names an entry", used by the normalizer and the YAML
// converter.
var sectionNameFields = map[string]string{
	"classAccesses":                 "apexClass",
	"agentAccesses":                 "agent",
	"applicationVisibilities":       "application",
	"categoryGroupVisibilities":     "dataCategoryGroup",
	"customMetadataTypeAccesses":    "name",
	"customPermissions":             "name",
	"customSettingAccesses":         "name",
	"fieldPermissions":              "field",
	"layoutAssignments":             "layout",
	"objectPermissions":             "object",
	"pageAccesses":                  "apexPage",
	"recordTypeVisibilities":        "recordType",
	"servicePresenceStatusAccesses": "servicePresenceStatus",
	"tabVisibilities":               "tab",
	"userPermissions":               "name",
	"flowAccesses":                  "flow",
	"externalDataSourceAccesses":    "externalDataSource",
	"genComputingSummaryDefAccess":  "enhancedSummaryDef",
}

// SectionNameField returns the element that identifies an entry of the given
// section tag, or "" when the tag has no entry form.
func SectionNameField(tag string) string {
	return sectionNameFields[tag]
}

package normalizer

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"xml2l/internal/graph"
	"xml2l/internal/schema"
)

func TestDefaultEdgeProperties(t *testing.T) {
	t.Run("apex class", func(t *testing.T) {
		p := DefaultEdgeProperties(graph.MetaTypeApexClass)

		if p.Enabled == nil || *p.Enabled != false {
			t.Error("expected Enabled=false")
		}
	})

	t.Run("field permissions", func(t *testing.T) {
		p := DefaultEdgeProperties(graph.MetaTypeField)

		if p.Readable == nil || *p.Readable != false {
			t.Error("expected Readable=false")
		}

		if p.Editable == nil || *p.Editable != false {
			t.Error("expected Editable=false")
		}
	})

	t.Run("object permissions", func(t *testing.T) {
		p := DefaultEdgeProperties(graph.MetaTypeObject)

		if p.AllowCreate == nil || *p.AllowCreate != false {
			t.Error("expected AllowCreate=false")
		}

		if p.ViewAll == nil || *p.ViewAll != false {
			t.Error("expected ViewAll=false")
		}
	})

	t.Run("application visibility", func(t *testing.T) {
		p := DefaultEdgeProperties(graph.MetaTypeApp)

		if p.Visible == nil || *p.Visible != false {
			t.Error("expected Visible=false")
		}

		if p.Default == nil || *p.Default != false {
			t.Error("expected Default=false")
		}
	})
}

func TestMarshalEntry(t *testing.T) {
	t.Run("class access", func(t *testing.T) {
		enabled := true
		p := graph.EdgeProperties{Enabled: &enabled}
		out, err := renderEntry("classAccesses", "MyClass", p, "", nil)
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(string(out), "<apexClass>MyClass</apexClass>") {
			t.Errorf("missing apexClass in output: %s", out)
		}

		if !strings.Contains(string(out), "<enabled>true</enabled>") {
			t.Errorf("missing enabled=true in output: %s", out)
		}
	})

	t.Run("field permission backfill false", func(t *testing.T) {
		// Default (false) values — simulates a backfilled entry.
		p := DefaultEdgeProperties(graph.MetaTypeField)
		out, err := renderEntry("fieldPermissions", "Account.Revenue__c", p, "", nil)
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(string(out), "<field>Account.Revenue__c</field>") {
			t.Errorf("missing field: %s", out)
		}

		if !strings.Contains(string(out), "<readable>false</readable>") {
			t.Errorf("expected readable=false: %s", out)
		}
	})

	t.Run("tab visibility with string value", func(t *testing.T) {
		p := graph.EdgeProperties{Visibility: strPtr("DefaultOn")}
		out, err := renderEntry("tabVisibilities", "standard-Account", p, "", nil)
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(string(out), "<visibility>DefaultOn</visibility>") {
			t.Errorf("expected visibility=DefaultOn: %s", out)
		}
	})

	t.Run("object permissions with correct tags", func(t *testing.T) {
		p := graph.EdgeProperties{
			AllowCreate: &[]bool{true}[0], AllowRead: &[]bool{true}[0],
			AllowEdit: &[]bool{false}[0], AllowDelete: &[]bool{false}[0],
			ModifyAll: &[]bool{false}[0], ViewAll: &[]bool{false}[0],
		}

		out, err := renderEntry("objectPermissions", "Account", p, "", nil)
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(string(out), "<modifyAllRecords>false</modifyAllRecords>") {
			t.Errorf("expected modifyAllRecords=false, got: %s", out)
		}

		if !strings.Contains(string(out), "<viewAllRecords>false</viewAllRecords>") {
			t.Errorf("expected viewAllRecords=false, got: %s", out)
		}
	})

	t.Run("page access", func(t *testing.T) {
		enabled := true
		p := graph.EdgeProperties{Enabled: &enabled}
		out, err := renderEntry("pageAccesses", "MyPage", p, "", nil)
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(string(out), "<apexPage>MyPage</apexPage>") {
			t.Errorf("missing apexPage in output: %s", out)
		}

		if !strings.Contains(string(out), "<enabled>true</enabled>") {
			t.Errorf("missing enabled=true in output: %s", out)
		}
	})
}

func TestNormalizeProfileBackfill(t *testing.T) {
	ms := schema.NewMasterSchema()
	ms.Add("fieldPermissions", "Account.Revenue__c")
	ms.Add("fieldPermissions", "Account.Existing__c")
	ms.Add("classAccesses", "ClassA")
	ms.Add("classAccesses", "ClassB")

	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	p, _ := g.AddProfile("Admin", "Admin.profile-meta.xml")

	// Profile has Existing__c and ClassA, but NOT Revenue__c or ClassB.
	fieldNode := g.GetOrCreateMetadataNode(graph.MetaTypeField, "Account.Existing__c")
	classNode := g.GetOrCreateMetadataNode(graph.MetaTypeApexClass, "ClassA")
	g.AddEdge(p, fieldNode, graph.EdgeProperties{Readable: boolPtr(true), Editable: boolPtr(true)})
	g.AddEdge(p, classNode, graph.EdgeProperties{Enabled: boolPtr(true)})

	xmlBytes := mustNormalize(t, p, g)
	output := string(xmlBytes)

	// Should contain existing entry with correct permissions.
	if !strings.Contains(output, "Account.Existing__c") {
		t.Error("missing existing field")
	}

	if !strings.Contains(output, "<readable>true</readable>") {
		t.Error("existing field should have readable=true")
	}

	// Should contain backfilled entry with false defaults.
	if !strings.Contains(output, "Account.Revenue__c") {
		t.Error("missing backfilled field")
	}

	if !strings.Contains(output, "<readable>false</readable>") {
		t.Error("backfilled field should have readable=false")
	}

	// Should contain backfilled class.
	if !strings.Contains(output, "ClassB") {
		t.Error("missing backfilled class")
	}
}

func TestNormalizeProfileLayoutNoBackfill(t *testing.T) {
	ms := schema.NewMasterSchema()
	ms.Add("layoutAssignments", "Account-Account Layout")
	ms.Add("layoutAssignments", "Account-Sales Layout")
	ms.Add("layoutAssignments", "Contact-Patient Layout")

	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	p, _ := g.AddProfile("Admin", "Admin.profile-meta.xml")

	// Profile has Account-Account Layout only.
	layoutNode := g.GetOrCreateMetadataNode(graph.MetaTypeLayout, "Account-Account Layout")
	g.AddEdge(p, layoutNode, graph.EdgeProperties{})

	xmlBytes := mustNormalize(t, p, g)
	output := string(xmlBytes)

	// Existing layout must be present.
	if !strings.Contains(output, "Account-Account Layout") {
		t.Error("missing existing layout assignment")
	}

	// Backfilled layouts must NOT appear.
	if strings.Contains(output, "Account-Sales Layout") {
		t.Error("layoutAssignments should not be backfilled from Master Schema")
	}

	if strings.Contains(output, "Contact-Patient Layout") {
		t.Error("layoutAssignments should not be backfilled from Master Schema")
	}
}

func TestNormalizeProfileLayoutSortByRecordType(t *testing.T) {
	ms := schema.NewMasterSchema()
	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	p, _ := g.AddProfile("Admin", "Admin.profile-meta.xml")

	// Add same layout with two record types + one without.
	layoutNode := g.GetOrCreateMetadataNode(graph.MetaTypeLayout, "Account-Account Layout")
	rt2 := "Account.RT2"
	rt1 := "Account.RT1"
	g.AddEdge(p, layoutNode, graph.EdgeProperties{RecordType: &rt2})
	g.AddEdge(p, layoutNode, graph.EdgeProperties{RecordType: &rt1})
	emptyRT := ""
	g.AddEdge(p, layoutNode, graph.EdgeProperties{RecordType: &emptyRT})

	xmlBytes := mustNormalize(t, p, g)
	output := string(xmlBytes)

	// Entries should appear in order: no recordType, then RT1, then RT2.
	rtNonePos := strings.Index(output, "<layoutAssignments>\n        <layout>Account-Account Layout</layout>\n    </layoutAssignments>")
	rt1Pos := strings.Index(output, "Account.RT1")
	rt2Pos := strings.Index(output, "Account.RT2")

	if rtNonePos < 0 {
		t.Fatal("missing layout entry without recordType")
	}

	if rt1Pos < 0 || rt2Pos < 0 {
		t.Fatal("missing layout entries with recordTypes")
	}

	if !(rtNonePos < rt1Pos && rt1Pos < rt2Pos) {
		t.Errorf("layout entries not sorted by recordType: none=%d, RT1=%d, RT2=%d", rtNonePos, rt1Pos, rt2Pos)
	}
}

func TestNormalizeProfileLayoutDefaultFromDisk(t *testing.T) {
	ms := schema.NewMasterSchema()
	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	g.SetAvailableLayouts([]string{
		"Account-Account Layout",
		"Account-HCO Layout",
		"Contact-Patient Layout",
	})
	p, _ := g.AddProfile("Admin", "Admin.profile-meta.xml")

	// Profile has NO layout assignments at all.
	xmlBytes := mustNormalize(t, p, g)
	output := string(xmlBytes)

	if !strings.Contains(output, "Account-Account Layout") {
		t.Error("expected default Account-Account Layout for Account object")
	}

	if strings.Contains(output, "Account-HCO Layout") {
		t.Error("should not add second Account layout as default")
	}

	if !strings.Contains(output, "Contact-Patient Layout") {
		t.Error("expected default Contact-Patient Layout for Contact object")
	}
}

func TestNormalizeProfileLayoutNoDefaultWhenEntriesExist(t *testing.T) {
	ms := schema.NewMasterSchema()
	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	g.SetAvailableLayouts([]string{
		"Account-Account Layout",
		"Account-HCO Layout",
		"Contact-Patient Layout",
	})
	p, _ := g.AddProfile("Admin", "Admin.profile-meta.xml")

	rt := "Account.HCO"
	node := g.GetOrCreateMetadataNode(graph.MetaTypeLayout, "Account-Custom Layout")
	g.AddEdge(p, node, graph.EdgeProperties{RecordType: &rt})

	xmlBytes := mustNormalize(t, p, g)
	output := string(xmlBytes)

	if !strings.Contains(output, "Account-Custom Layout") {
		t.Error("existing Account layout should be kept")
	}

	if strings.Contains(output, "Account-Account Layout") {
		t.Error("should not add default Account layout when profile already has Account entries")
	}

	if !strings.Contains(output, "Contact-Patient Layout") {
		t.Error("expected default Contact layout — profile has no Contact entries")
	}
}

func TestNormalizeProfileLayoutFiltersMultipleNoRecordType(t *testing.T) {
	ms := schema.NewMasterSchema()
	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	p, _ := g.AddProfile("Admin", "Admin.profile-meta.xml")

	layouts := []struct {
		name string
		rt   string
	}{
		{"Account-Sales Layout", ""},
		{"Account-Account Layout", ""},
		{"Account-Other Layout", ""},
		{"Account-HCO Layout", "Account.HCO"},
		{"Account-HCS Layout", "Account.HCS"},
	}
	for _, l := range layouts {
		node := g.GetOrCreateMetadataNode(graph.MetaTypeLayout, l.name)
		var rtPtr *string

		if l.rt != "" {
			rtPtr = &l.rt
		}

		g.AddEdge(p, node, graph.EdgeProperties{RecordType: rtPtr})
	}

	xmlBytes := mustNormalize(t, p, g)
	output := string(xmlBytes)

	if !strings.Contains(output, "Account-Account Layout") {
		t.Error("expected Account-Account Layout (first no-RT) to be kept")
	}

	if strings.Contains(output, "Account-Sales Layout") {
		t.Error("Account-Sales Layout (no-RT, not first) should be filtered")
	}

	if strings.Contains(output, "Account-Other Layout") {
		t.Error("Account-Other Layout (no-RT, not first) should be filtered")
	}

	if !strings.Contains(output, "Account-HCO Layout") {
		t.Error("Account-HCO Layout (with RT) should be kept")
	}

	if !strings.Contains(output, "Account-HCS Layout") {
		t.Error("Account-HCS Layout (with RT) should be kept")
	}
}

func TestNormalizeProfileLayoutDedupRecordTypes(t *testing.T) {
	ms := schema.NewMasterSchema()
	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	p, _ := g.AddProfile("Admin", "Admin.profile-meta.xml")

	rt := "Account.HCO"
	node1 := g.GetOrCreateMetadataNode(graph.MetaTypeLayout, "Account-Alpha Layout")
	node2 := g.GetOrCreateMetadataNode(graph.MetaTypeLayout, "Account-Beta Layout")
	g.AddEdge(p, node1, graph.EdgeProperties{RecordType: &rt})
	g.AddEdge(p, node2, graph.EdgeProperties{RecordType: &rt})

	xmlBytes := mustNormalize(t, p, g)
	output := string(xmlBytes)

	if !strings.Contains(output, "Account-Alpha Layout") {
		t.Error("expected Account-Alpha Layout (first with RT) to be kept")
	}

	if strings.Contains(output, "Account-Beta Layout") {
		t.Error("Account-Beta Layout (duplicate recordType) should be filtered")
	}

	if strings.Count(output, "Account.HCO") != 1 {
		t.Errorf("expected exactly 1 occurrence of Account.HCO recordType, got %d", strings.Count(output, "Account.HCO"))
	}
}

func TestNormalizeProfileXMLHeader(t *testing.T) {
	ms := schema.NewMasterSchema()
	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	p, _ := g.AddProfile("Admin", "test.profile-meta.xml")
	p.UserLicense = "Salesforce"

	xmlBytes := mustNormalize(t, p, g)
	output := string(xmlBytes)

	if !strings.HasPrefix(output, `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Errorf("XML should start with header, got: %s", output[:50])
	}

	if !strings.Contains(output, `<Profile xmlns="http://soap.sforce.com/2006/04/metadata">`) {
		t.Error("missing Profile root element with namespace")
	}
}

func TestPreservedBlocks(t *testing.T) {
	rawXML := `<?xml version="1.0"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <userLicense>Salesforce</userLicense>
    <classAccesses>
        <apexClass>MyClass</apexClass>
        <enabled>true</enabled>
    </classAccesses>
    <loginHours>
        <dayOfWeek>Monday</dayOfWeek>
    </loginHours>
    <loginIpRanges>
        <startAddress>10.0.0.0</startAddress>
    </loginIpRanges>
    <loginIpRanges>
        <startAddress>10.0.0.1</startAddress>
    </loginIpRanges>
    <loginFlows>
        <flowType>LoginFlow</flowType>
        <flow>MyFlow</flow>
    </loginFlows>
    <fieldLevelSecurities>
        <field>Account.Legacy__c</field>
        <hidden>false</hidden>
    </fieldLevelSecurities>
    <profileActionOverrides>
        <actionName>Tab</actionName>
    </profileActionOverrides>
</Profile>`

	blocks, err := preservedBlocks(rawXML)
	if err != nil {
		t.Fatalf("preservedBlocks: %v", err)
	}

	byTag := make(map[string]int)

	var text strings.Builder

	for _, b := range blocks {
		byTag[b.tag]++
		text.WriteString(b.text)
	}

	for _, tag := range []string{"loginHours", "loginIpRanges", "loginFlows", "fieldLevelSecurities", "profileActionOverrides"} {
		if byTag[tag] == 0 {
			t.Errorf("missing %s", tag)
		}
	}

	if byTag["loginIpRanges"] != 2 {
		t.Errorf("expected 2 loginIpRanges blocks, got %d", byTag["loginIpRanges"])
	}

	if !strings.Contains(text.String(), "Monday") {
		t.Error("missing loginHours content")
	}

	if !strings.Contains(text.String(), "MyFlow") {
		t.Error("missing loginFlows content")
	}

	// Modeled sections are written by the normalizer, so they must not be
	// preserved here.
	if byTag["classAccesses"] != 0 || byTag["userLicense"] != 0 {
		t.Errorf("modeled sections must not be preserved: %v", byTag)
	}
}

func TestPreservedBlocksEmpty(t *testing.T) {
	if got, err := preservedBlocks(""); err != nil || len(got) != 0 {
		t.Errorf("expected none, got %v (err %v)", got, err)
	}

	if got, err := preservedBlocks("<Profile/>"); err != nil || len(got) != 0 {
		t.Errorf("expected none for Profile-only, got %v (err %v)", got, err)
	}

	// Malformed input must be reported, not silently dropped.
	if _, err := preservedBlocks("<Profile><oops>"); err == nil {
		t.Error("expected an error for malformed XML")
	}
}

func TestNormalizeProfileRejectsUnparseableRawXML(t *testing.T) {
	g := graph.NewGraph()
	g.SetMasterSchema(schema.NewMasterSchema())

	p, err := g.AddProfile("Broken", "")
	if err != nil {
		t.Fatal(err)
	}

	p.RawXML = `<Profile xmlns="http://soap.sforce.com/2006/04/metadata"><loginIpRanges>`

	if _, err := NormalizeProfile(p, g, nil); err == nil {
		t.Fatal("expected an error instead of a profile without its unmodelled sections")
	}
}

func TestNormalizeProfilePreservesLoginHours(t *testing.T) {
	rawXML := `<?xml version="1.0"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <userLicense>Salesforce</userLicense>
    <loginHours>
        <dayOfWeek>Monday</dayOfWeek>
        <loginStartTime>08:00:00.000Z</loginStartTime>
        <loginEndTime>17:00:00.000Z</loginEndTime>
    </loginHours>
</Profile>`

	ms := schema.NewMasterSchema()
	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	p, _ := g.AddProfile("Admin", "test.profile-meta.xml")
	p.RawXML = rawXML
	p.UserLicense = "Salesforce"

	xmlBytes := mustNormalize(t, p, g)
	output := string(xmlBytes)

	if !strings.Contains(output, "loginHours") {
		t.Error("loginHours section was not preserved")
	}

	if !strings.Contains(output, "Monday") {
		t.Error("loginHours content was not preserved")
	}
}

func TestNormalizeProfilePreservesLoginFlows(t *testing.T) {
	rawXML := `<?xml version="1.0"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <loginFlows>
        <flowType>LoginFlow</flowType>
        <userLicense>Salesforce</userLicense>
        <flow>TestFlow</flow>
    </loginFlows>
    <userLicense>Salesforce</userLicense>
</Profile>`

	ms := schema.NewMasterSchema()
	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	p, _ := g.AddProfile("Admin", "test.profile-meta.xml")
	p.RawXML = rawXML
	p.UserLicense = "Salesforce"

	xmlBytes := mustNormalize(t, p, g)
	output := string(xmlBytes)

	if !strings.Contains(output, "loginFlows") {
		t.Error("loginFlows section was not preserved")
	}

	if !strings.Contains(output, "LoginFlow") {
		t.Error("loginFlows flowType was not preserved")
	}

	if !strings.Contains(output, "TestFlow") {
		t.Error("loginFlows flow was not preserved")
	}
}

func TestWriteProfiles(t *testing.T) {
	ms := schema.NewMasterSchema()
	ms.Add("classAccesses", "TestClass")

	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	p, _ := g.AddProfile("Test", "test_save_output.profile-meta.xml")
	_ = p

	// Since there's no real file path to write to, just verify the function
	// doesn't panic with empty/nil profiles.
	// WriteProfiles will skip profiles with empty SourcePath.
	emptyGraph := graph.NewGraph()

	if err := WriteProfiles(emptyGraph); err != nil {
		t.Errorf("WriteProfiles empty: %v", err)
	}
}

// Verify the output is valid XML by round-tripping through xml.Decoder.
func TestNormalizeProfileValidXML(t *testing.T) {
	ms := schema.NewMasterSchema()
	ms.Add("fieldPermissions", "A.Test")
	ms.Add("fieldPermissions", "B.Test")
	ms.Add("classAccesses", "MyClass")

	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	p, _ := g.AddProfile("Admin", "test.profile-meta.xml")
	p.UserLicense = "Salesforce"

	xmlBytes := mustNormalize(t, p, g)

	// Parse back to verify it's well-formed XML.
	decoder := xml.NewDecoder(bytes.NewReader(xmlBytes))
	tokenCount := 0

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		_ = tok
		tokenCount++
	}

	if tokenCount == 0 {
		t.Error("no tokens parsed — invalid XML")
	}
}

func TestNormalizeProfileTabVisBackfillNoEmptyTag(t *testing.T) {
	ms := schema.NewMasterSchema()
	ms.Add("tabVisibilities", "standard-Account")
	ms.Add("fieldPermissions", "A.Field") // add a field so something produces edges

	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	p, _ := g.AddProfile("Admin", "test.profile-meta.xml")

	xmlBytes := mustNormalize(t, p, g)
	output := string(xmlBytes)

	// Tab vis entry should be present.
	if !strings.Contains(output, "standard-Account") {
		t.Error("missing backfilled tab entry")
	}

	// The empty <visibility></visibility> tag MUST NOT appear.
	if strings.Contains(output, "<visibility></visibility>") {
		t.Error("backfilled tab entry should not emit empty visibility tag")
	}

	if strings.Contains(output, "<visibility/>") {
		t.Error("backfilled tab entry should not emit self-closing visibility tag")
	}
}

func TestNormalizeProfileAlphabeticalSort(t *testing.T) {
	ms := schema.NewMasterSchema()
	ms.Add("fieldPermissions", "ZField")
	ms.Add("fieldPermissions", "AField")
	ms.Add("fieldPermissions", "MField")

	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	p, _ := g.AddProfile("Admin", "test.profile-meta.xml")

	xmlBytes := mustNormalize(t, p, g)

	// Check that fields appear in alphabetical order.
	aPos := bytes.Index(xmlBytes, []byte("AField"))
	mPos := bytes.Index(xmlBytes, []byte("MField"))
	zPos := bytes.Index(xmlBytes, []byte("ZField"))

	if aPos < 0 || mPos < 0 || zPos < 0 {
		t.Fatal("missing expected field entries")
	}

	if !(aPos < mPos && mPos < zPos) {
		t.Error("fields not in alphabetical order: A < M < Z expected")
	}
}

// mockOrgSchema is a test implementation of graph.OrgSchemaProvider.
type mockOrgSchema struct {
	entries map[string]map[string]bool
}

func (m *mockOrgSchema) Has(xmlName, fullName string) bool {
	if m == nil {
		return false
	}

	names, ok := m.entries[xmlName]

	if !ok {
		return false
	}

	return names[fullName]
}

func (m *mockOrgSchema) HasType(xmlName string) bool {
	if m == nil {
		return false
	}

	_, ok := m.entries[xmlName]

	return ok
}

func newMockOrgSchema(entries map[string]map[string]bool) *mockOrgSchema {
	return &mockOrgSchema{entries: entries}
}

func TestNormalizeProfileOrgSchemaFiltersApexClass(t *testing.T) {
	ms := schema.NewMasterSchema()
	ms.Add("classAccesses", "ClassA")
	ms.Add("classAccesses", "ClassB")
	ms.Add("classAccesses", "ClassC")

	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	// Org schema only has ClassA — ClassB and ClassC should be filtered out.
	g.SetOrgSchema(newMockOrgSchema(map[string]map[string]bool{
		"CustomField": {},
		"ApexClass":   {"ClassA": true},
	}))

	p, _ := g.AddProfile("Admin", "Admin.profile-meta.xml")
	// Profile has ClassA (from graph edges).
	nodeA := g.GetOrCreateMetadataNode(graph.MetaTypeApexClass, "ClassA")
	g.AddEdge(p, nodeA, graph.EdgeProperties{Enabled: boolPtr(true)})

	xmlBytes := mustNormalize(t, p, g)
	output := string(xmlBytes)

	if !strings.Contains(output, "ClassA") {
		t.Error("expected ClassA (present in org) to be in output")
	}

	if strings.Contains(output, "ClassB") {
		t.Error("ClassB (not in org) should be filtered out")
	}

	if strings.Contains(output, "ClassC") {
		t.Error("ClassC (not in org) should be filtered out")
	}
}

func TestNormalizeProfileOrgSchemaFiltersLayouts(t *testing.T) {
	ms := schema.NewMasterSchema()
	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	g.SetAvailableLayouts([]string{
		"Account-Account Layout",
		"Account-Other Layout",
		"Contact-Patient Layout",
	})
	// Org schema only has Account-Account Layout and Contact-Patient Layout.
	g.SetOrgSchema(newMockOrgSchema(map[string]map[string]bool{
		"ApexClass": {},
		"Layout":    {"Account-Account Layout": true, "Contact-Patient Layout": true},
	}))

	p, _ := g.AddProfile("Admin", "Admin.profile-meta.xml")

	xmlBytes := mustNormalize(t, p, g)
	output := string(xmlBytes)

	if !strings.Contains(output, "Account-Account Layout") {
		t.Error("expected Account-Account Layout (in org) to be in output")
	}

	if strings.Contains(output, "Account-Other Layout") {
		t.Error("Account-Other Layout (not in org) should be filtered out")
	}

	if !strings.Contains(output, "Contact-Patient Layout") {
		t.Error("expected Contact-Patient Layout (in org) to be in output")
	}
}

func TestMatchesExclude(t *testing.T) {
	tests := []struct {
		name     string
		entry    string
		patterns []string
		want     bool
	}{
		{"single pattern match", "ChatterInternal", []string{"chatter"}, true},
		{"multiple patterns second match", "CanUseNewDashboardBuilder", []string{"chatter", "canuse"}, true},
		{"no match", "ViewAllData", []string{"chatter"}, false},
		{"case insensitive entry", "chatterinternal", []string{"chatter"}, true},
		{"case insensitive pattern", "ChatterInternal", []string{"chatter"}, true},
		{"prefix match", "AdminUserCustomPermission", []string{"admin"}, true},
		{"non-prefix substring does not match", "ViewAdminRecords", []string{"admin"}, false},
		{"empty patterns", "Chatter", []string{}, false},
		{"nil patterns", "Chatter", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesExclude(tt.entry, tt.patterns)

			if got != tt.want {
				t.Errorf("matchesExclude(%q, %v) = %v, want %v", tt.entry, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestBuildRawEntryIndex(t *testing.T) {
	raw := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <userPermissions>
        <name>ChatterInternal</name>
        <enabled>true</enabled>
    </userPermissions>
    <userPermissions>
        <name>StandardUserPerm</name>
        <enabled>false</enabled>
    </userPermissions>
    <classAccesses>
        <apexClass>MyClass</apexClass>
        <enabled>true</enabled>
    </classAccesses>
    <fieldPermissions>
        <field>Account.Revenue__c</field>
        <readable>true</readable>
        <editable>false</editable>
    </fieldPermissions>
</Profile>`

	idx, err := buildRawEntryIndex(raw)
	if err != nil {
		t.Fatalf("buildRawEntryIndex: %v", err)
	}

	if got := idx["userPermissions"]["ChatterInternal"]; !strings.Contains(got, "<enabled>true</enabled>") {
		t.Errorf("expected ChatterInternal entry, got %q", got)
	}

	if got := idx["userPermissions"]["StandardUserPerm"]; !strings.Contains(got, "<enabled>false</enabled>") {
		t.Errorf("expected StandardUserPerm entry, got %q", got)
	}

	if got := idx["classAccesses"]["MyClass"]; !strings.Contains(got, "<apexClass>MyClass</apexClass>") {
		t.Errorf("expected MyClass entry, got %q", got)
	}

	if got := idx["fieldPermissions"]["Account.Revenue__c"]; !strings.Contains(got, "<readable>true</readable>") {
		t.Errorf("expected Account.Revenue__c entry, got %q", got)
	}

	if _, ok := idx["userPermissions"]["NonExistent"]; ok {
		t.Error("expected no index entry for a name absent from the source")
	}
}

func TestNormalizeProfileExcludePreservesOriginal(t *testing.T) {
	ms := schema.NewMasterSchema()
	ms.Add("userPermissions", "ChatterInternal")
	ms.Add("userPermissions", "StandardUserPerm")

	raw := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <userPermissions>
        <name>ChatterInternal</name>
        <enabled>false</enabled>
    </userPermissions>
    <userPermissions>
        <name>StandardUserPerm</name>
        <enabled>true</enabled>
    </userPermissions>
</Profile>`

	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	g.SetExcludePatterns([]string{"chatter"})

	p, _ := g.AddProfile("Admin", "Admin.profile-meta.xml")
	p.RawXML = raw
	p.UserLicense = "Salesforce"

	// Graph edge says ChatterInternal is enabled, but raw XML says false.
	node := g.GetOrCreateMetadataNode(graph.MetaTypeUserPerm, "ChatterInternal")
	g.AddEdge(p, node, graph.EdgeProperties{Enabled: boolPtr(true)})

	node2 := g.GetOrCreateMetadataNode(graph.MetaTypeUserPerm, "StandardUserPerm")
	g.AddEdge(p, node2, graph.EdgeProperties{Enabled: boolPtr(true)})

	xmlBytes := mustNormalize(t, p, g)
	output := string(xmlBytes)

	// ChatterInternal should be preserved from raw XML (enabled=false),
	// NOT marshaled with enabled=true.
	if !strings.Contains(output, "<enabled>false</enabled>") {
		t.Error("expected preserved ChatterInternal with enabled=false from raw XML")
	}

	// StandardUserPerm (not excluded) should be marshaled with enabled=true from graph.
	if !strings.Contains(output, "<enabled>true</enabled>") {
		t.Error("expected marshaled StandardUserPerm with enabled=true")
	}
}

func TestNormalizeProfileExcludeDropsBackfilled(t *testing.T) {
	ms := schema.NewMasterSchema()
	ms.Add("userPermissions", "ChatterInternal")
	ms.Add("userPermissions", "ChatterOwnGroups") // backfilled, not in profile

	raw := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <userPermissions>
        <name>ChatterInternal</name>
        <enabled>true</enabled>
    </userPermissions>
</Profile>`

	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	g.SetExcludePatterns([]string{"chatter"})

	p, _ := g.AddProfile("Admin", "Admin.profile-meta.xml")
	p.RawXML = raw
	p.UserLicense = "Salesforce"

	node := g.GetOrCreateMetadataNode(graph.MetaTypeUserPerm, "ChatterInternal")
	g.AddEdge(p, node, graph.EdgeProperties{Enabled: boolPtr(true)})

	xmlBytes := mustNormalize(t, p, g)
	output := string(xmlBytes)

	// ChatterInternal (in raw XML) should be preserved.
	if !strings.Contains(output, "ChatterInternal") {
		t.Error("expected ChatterInternal to be in output")
	}

	// ChatterOwnGroups (backfilled, matches exclude, not in raw XML) should be dropped.
	if strings.Contains(output, "ChatterOwnGroups") {
		t.Error("expected ChatterOwnGroups (backfilled + excluded) to be dropped")
	}
}

func TestNormalizeProfileExcludeOrdering(t *testing.T) {
	ms := schema.NewMasterSchema()
	ms.Add("userPermissions", "A_Normal")
	ms.Add("userPermissions", "B_Normal")
	ms.Add("userPermissions", "X_Excluded")
	ms.Add("userPermissions", "Y_Excluded")

	raw := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <userPermissions>
        <name>X_Excluded</name>
        <enabled>true</enabled>
    </userPermissions>
    <userPermissions>
        <name>Y_Excluded</name>
        <enabled>true</enabled>
    </userPermissions>
    <userPermissions>
        <name>A_Normal</name>
        <enabled>true</enabled>
    </userPermissions>
    <userPermissions>
        <name>B_Normal</name>
        <enabled>true</enabled>
    </userPermissions>
</Profile>`

	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	g.SetExcludePatterns([]string{"excluded"})

	p, _ := g.AddProfile("Admin", "Admin.profile-meta.xml")
	p.RawXML = raw
	p.UserLicense = "Salesforce"

	for _, name := range []string{"A_Normal", "B_Normal", "X_Excluded", "Y_Excluded"} {
		node := g.GetOrCreateMetadataNode(graph.MetaTypeUserPerm, name)
		g.AddEdge(p, node, graph.EdgeProperties{Enabled: boolPtr(true)})
	}

	xmlBytes := mustNormalize(t, p, g)
	output := string(xmlBytes)

	// All entries should be in alphabetical order: A_Normal, B_Normal, X_Excluded, Y_Excluded.
	aIdx := strings.Index(output, "A_Normal")
	bIdx := strings.Index(output, "B_Normal")
	xIdx := strings.Index(output, "X_Excluded")
	yIdx := strings.Index(output, "Y_Excluded")

	if aIdx < 0 || bIdx < 0 || xIdx < 0 || yIdx < 0 {
		t.Fatal("all entries should be present in output")
	}

	if !(aIdx < bIdx && bIdx < xIdx && xIdx < yIdx) {
		t.Error("entries should be in alphabetical order: A_Normal < B_Normal < X_Excluded < Y_Excluded")
	}
}

func TestNormalizeProfileExcludeNoEffectWhenOmitted(t *testing.T) {
	ms := schema.NewMasterSchema()
	ms.Add("userPermissions", "ChatterInternal")
	ms.Add("userPermissions", "StandardUserPerm")

	raw := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <userPermissions>
        <name>ChatterInternal</name>
        <enabled>false</enabled>
    </userPermissions>
    <userPermissions>
        <name>StandardUserPerm</name>
        <enabled>false</enabled>
    </userPermissions>
</Profile>`

	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	// Deliberately NOT setting ExcludePatterns.

	p, _ := g.AddProfile("Admin", "Admin.profile-meta.xml")
	p.RawXML = raw
	p.UserLicense = "Salesforce"

	for _, name := range []string{"ChatterInternal", "StandardUserPerm"} {
		node := g.GetOrCreateMetadataNode(graph.MetaTypeUserPerm, name)
		g.AddEdge(p, node, graph.EdgeProperties{Enabled: boolPtr(true)})
	}

	xmlBytes := mustNormalize(t, p, g)
	output := string(xmlBytes)

	// Both entries should be marshaled (enabled=true from graph), not preserved from raw XML.
	if !strings.Contains(output, "<enabled>true</enabled>") {
		t.Error("expected all entries to be marshaled with enabled=true from graph")
	}
}

func strPtr(s string) *string { return &s }

func boolPtr(b bool) *bool { return &b }

func TestNormalizeProfilePreservesUnmodeledSections(t *testing.T) {
	rawXML := `<?xml version="1.0"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <userLicense>Salesforce</userLicense>
    <fieldLevelSecurities>
        <field>Account.Legacy__c</field>
        <hidden>false</hidden>
    </fieldLevelSecurities>
    <loginHours>
        <dayOfWeek>Monday</dayOfWeek>
    </loginHours>
</Profile>`

	ms := schema.NewMasterSchema()
	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	p, _ := g.AddProfile("Admin", "test.profile-meta.xml")
	p.RawXML = rawXML
	p.UserLicense = "Salesforce"

	out := string(mustNormalize(t, p, g))

	for _, tag := range []string{"fieldLevelSecurities", "loginHours"} {
		if !strings.Contains(out, tag) {
			t.Errorf("unmodeled section %s was dropped", tag)
		}
	}

	if strings.Contains(out, "</loginHours></Profile>") {
		t.Error("preserved sections must not be glued to the closing tag")
	}

	if strings.Count(out, "<userLicense>") != 1 {
		t.Errorf("userLicense should appear once, got %d", strings.Count(out, "<userLicense>"))
	}
}

func TestNormalizeProfileOrgSchemaUnqueriedTypeKeepsEntries(t *testing.T) {
	ms := schema.NewMasterSchema()
	g := graph.NewGraph()
	g.SetMasterSchema(ms)

	// The org schema knows CustomField but never queried ApexClass (its query
	// failed or the type is unsupported). Entries of an unqueried type must
	// survive, not be treated as absent.
	g.SetOrgSchema(newMockOrgSchema(map[string]map[string]bool{
		"CustomField": {},
	}))

	p, _ := g.AddProfile("Admin", "Admin.profile-meta.xml")
	node := g.GetOrCreateMetadataNode(graph.MetaTypeApexClass, "ClassA")
	g.AddEdge(p, node, graph.EdgeProperties{Enabled: boolPtr(true)})

	out := string(mustNormalize(t, p, g))

	if !strings.Contains(out, "ClassA") {
		t.Error("entries of an unqueried type must not be filtered out")
	}
}

// mustNormalize renders a profile and fails the test on a parse error.
func mustNormalize(t *testing.T, p *graph.ProfileNode, g NormalizeGraph) []byte {
	t.Helper()

	out, err := NormalizeProfile(p, g, nil)
	if err != nil {
		t.Fatalf("normalize %s: %v", p.Name, err)
	}

	return out
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "X.profile-meta.xml")

	// 0640 rather than 0600: the temporary file writeFileAtomic creates is 0600,
	// so a 0600 source would keep passing even if the mode were never preserved.
	if err := os.WriteFile(path, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}

	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := writeFileAtomic(path, []byte("new")); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// Compare against the mode the file actually had. Windows has no POSIX
	// permission bits: Go maps only the owner-write bit to the read-only
	// attribute, so 0640 is reported back as 0666 there and a literal 0600
	// assertion can never hold.
	if info.Mode().Perm() != before.Mode().Perm() {
		t.Errorf("mode = %v, want %v (preserved)", info.Mode().Perm(), before.Mode().Perm())
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 1 {
		t.Errorf("expected only the target file, got %d entries (temp file left behind?)", len(entries))
	}
}

func TestWriteFileAtomicReadOnlyTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "X.profile-meta.xml")

	if err := os.WriteFile(path, []byte("old"), 0o444); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}

	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if before.Mode().Perm()&0o200 != 0 {
		t.Skipf("filesystem does not honour a read-only mode: %v", before.Mode().Perm())
	}

	if err := writeFileAtomic(path, []byte("new")); err != nil {
		t.Fatalf("writeFileAtomic on a read-only file: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}

	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if after.Mode().Perm() != before.Mode().Perm() {
		t.Errorf("mode = %v, want %v (read-only preserved)", after.Mode().Perm(), before.Mode().Perm())
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 1 {
		t.Errorf("expected only the target file, got %d entries (temp file left behind?)", len(entries))
	}
}

// TestNormalizeProfileRetrieveFidelity pins the saved shape against two real
// profiles retrieved from an org. The saved output may differ from the retrieved
// files only by backfilled entries and by ghost references dropped for having no
// file in the project (ADR-0001). Ordering must match exactly (ADR-0002).
func TestNormalizeProfileRetrieveFidelity(t *testing.T) {
	const fixtureDir = "testdata/retrieve-fidelity"

	profileNames := []string{"Admin", "Coupons Profile"}
	dir := t.TempDir()

	fields := make(map[string]bool)
	classes := make(map[string]bool)
	retrieved := make(map[string]string)

	for _, name := range profileNames {
		raw := readFixture(t, filepath.Join(fixtureDir, name+".retrieved.xml"))

		dst := filepath.Join(dir, "profiles", name+".profile-meta.xml")
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(dst, raw, 0o644); err != nil {
			t.Fatal(err)
		}

		collectEntryNames(t, name, raw, fields, classes)
		retrieved[name] = string(raw)
	}

	// Ghost references: the org profiles still list these two standard fields,
	// but the project never retrieved field metadata for them, so they are
	// dropped and must stay out of the saved output.
	droppedFields := map[string]bool{
		"Case.ClosedOnCreate": true,
		"Case.IsVisibleInCss": true,
	}

	for field := range fields {
		if droppedFields[field] {
			continue
		}

		obj, fieldName, ok := strings.Cut(field, ".")
		if !ok {
			t.Fatalf("field entry %q has no object prefix", field)
		}

		path := filepath.Join(dir, "objects", obj, "fields", fieldName+".field-meta.xml")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte("<CustomField/>\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for class := range classes {
		path := filepath.Join(dir, "classes", class+".cls")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte("// fixture\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	g, err := schema.LoadProfiles(dir)
	if err != nil {
		t.Fatalf("load fixture project: %v", err)
	}

	if len(g.Profiles()) != len(profileNames) {
		t.Fatalf("expected %d profiles, got %d", len(profileNames), len(g.Profiles()))
	}

	for _, p := range g.Profiles() {
		want := readFixture(t, filepath.Join(fixtureDir, p.Name+".golden.xml"))

		got := string(mustNormalize(t, p, g))
		if got != string(want) {
			t.Errorf("%s: saved output differs from golden at %s", p.Name, firstDifference(string(want), got))
		}

		for field := range droppedFields {
			if strings.Contains(got, "<field>"+field+"</field>") {
				t.Errorf("%s: ghost reference %s must be dropped, not saved", p.Name, field)
			}
		}

		assertRetrieveCompatible(t, p.Name, retrieved[p.Name], got, droppedFields)
	}
}

// assertRetrieveCompatible checks the saved output against the retrieved file
// itself: ADR-0002 claims a save keeps the retrieved structure, and comparing
// with a golden alone cannot prove it, because the golden can be regenerated
// from the same wrong renderer.
func assertRetrieveCompatible(t *testing.T, profileName, retrieved, saved string, droppedFields map[string]bool) {
	t.Helper()

	want := shapeProfile(t, profileName+" retrieved", retrieved)
	got := shapeProfile(t, profileName+" saved", saved)

	if i, ok := firstUnsorted(got.tags); ok {
		t.Errorf("%s: top-level elements are not alphabetical: %s before %s", profileName, got.tags[i-1], got.tags[i])
	}

	// The saved file may add entries and sections (backfill), but it must not
	// reorder or drop the sections the source carried. Runs are collapsed
	// because a section's entries are contiguous in both files.
	wantSections := compressRuns(want.tags)
	gotSections := compressRuns(got.tags)
	next := 0

	for _, tag := range gotSections {
		if next < len(wantSections) && tag == wantSections[next] {
			next++
		}
	}

	if next != len(wantSections) {
		t.Errorf("%s: saved sections %v do not retain the retrieved sections %v", profileName, gotSections, wantSections)
	}

	dropped := make(map[string]bool, len(droppedFields))

	for field := range droppedFields {
		dropped["fieldPermissions/"+field] = true
	}

	for key, wantChildren := range want.entries {
		if dropped[key] {
			continue
		}

		gotChildren, ok := got.entries[key]
		if !ok {
			t.Errorf("%s: entry %s disappeared", profileName, key)
			continue
		}

		if !slices.Equal(gotChildren, wantChildren) {
			t.Errorf("%s: entry %s has children %v, retrieved %v", profileName, key, gotChildren, wantChildren)
		}
	}

	for key, children := range got.entries {
		if i, ok := firstUnsorted(children); ok {
			t.Errorf("%s: entry %s children are not alphabetical: %s before %s", profileName, key, children[i-1], children[i])
		}
	}
}

// firstUnsorted reports the first position whose tag sorts before its
// predecessor.
func firstUnsorted(tags []string) (int, bool) {
	for i := 1; i < len(tags); i++ {
		if tags[i-1] > tags[i] {
			return i, true
		}
	}

	return 0, false
}

// compressRuns turns ["a","a","b"] into ["a","b"].
func compressRuns(tags []string) []string {
	var out []string

	for _, tag := range tags {
		if len(out) == 0 || out[len(out)-1] != tag {
			out = append(out, tag)
		}
	}

	return out
}

// shapedProfile is the part of a profile the fidelity assertions compare: the
// top-level element sequence and, for every named entry, its child sequence.
type shapedProfile struct {
	tags    []string
	entries map[string][]string // "<section>/<entry name>" -> child tags
}

// shapeProfile reads a profile document into a shapedProfile. Entries are keyed
// by section and name because saving reorders them.
func shapeProfile(t *testing.T, label, doc string) shapedProfile {
	t.Helper()

	shape := shapedProfile{entries: make(map[string][]string)}
	dec := xml.NewDecoder(strings.NewReader(doc))

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			t.Fatalf("%s: no <Profile> root", label)
		}
		if err != nil {
			t.Fatalf("%s: parse root: %v", label, err)
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "Profile" {
			break
		}
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return shape
		}
		if err != nil {
			t.Fatalf("%s: parse: %v", label, err)
		}

		switch t2 := tok.(type) {
		case xml.StartElement:
			tag := t2.Name.Local
			shape.tags = append(shape.tags, tag)

			nameField, ok := sectionNameField[tag]
			if !ok {
				if err := dec.Skip(); err != nil {
					t.Fatalf("%s/%s: skip: %v", label, tag, err)
				}

				continue
			}

			name, children := shapeEntry(t, label, tag, nameField, t2, dec)
			shape.entries[tag+"/"+name] = children
		case xml.EndElement:
			return shape
		}
	}
}

// shapeEntry returns an entry's name and the tags of its children, in order.
func shapeEntry(t *testing.T, label, section, nameField string, se xml.StartElement, dec *xml.Decoder) (string, []string) {
	t.Helper()

	var entry struct {
		Inner []byte `xml:",innerxml"`
	}
	if err := dec.DecodeElement(&entry, &se); err != nil {
		t.Fatalf("%s/%s: decode entry: %v", label, section, err)
	}

	var (
		name     string
		children []string
	)

	inner := xml.NewDecoder(bytes.NewReader(entry.Inner))

	for {
		tok, err := inner.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("%s/%s: parse entry: %v", label, section, err)
		}

		child, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		children = append(children, child.Name.Local)

		if child.Name.Local == nameField {
			if err := inner.DecodeElement(&name, &child); err != nil {
				t.Fatalf("%s/%s: decode %s: %v", label, section, nameField, err)
			}

			continue
		}

		if err := inner.Skip(); err != nil {
			t.Fatalf("%s/%s: skip %s: %v", label, section, child.Name.Local, err)
		}
	}

	return name, children
}

// readFixture reads a fixture and normalises its line endings to LF. The
// writer always emits LF, so the comparison must not depend on how the checkout
// materialised the fixture (a Windows working tree is CRLF).
func readFixture(t *testing.T, path string) []byte {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}

	return bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
}

// collectEntryNames records the field and Apex class names a profile references.
func collectEntryNames(t *testing.T, label string, raw []byte, fields, classes map[string]bool) {
	t.Helper()

	dec := xml.NewDecoder(bytes.NewReader(raw))

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return
		}

		if err != nil {
			t.Fatalf("parse %s: %v", label, err)
		}

		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		if se.Name.Local != "field" && se.Name.Local != "apexClass" {
			continue
		}

		var value string

		if err := dec.DecodeElement(&value, &se); err != nil {
			t.Fatalf("decode %s/%s: %v", label, se.Name.Local, err)
		}

		if se.Name.Local == "field" {
			fields[value] = true
		} else {
			classes[value] = true
		}
	}
}

// firstDifference names the line where two XML documents start to disagree.
func firstDifference(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")

	for i := 0; i < len(wantLines) && i < len(gotLines); i++ {
		if wantLines[i] != gotLines[i] {
			return fmt.Sprintf("line %d: want %q, got %q", i+1, wantLines[i], gotLines[i])
		}
	}

	return fmt.Sprintf("line %d: one document ended first", min(len(wantLines), len(gotLines))+1)
}

func TestRenderEntrySortsChildrenAlphabetically(t *testing.T) {
	out, err := renderEntry("fieldPermissions", "Account.Name", graph.EdgeProperties{
		Readable: boolPtr(true),
		Editable: boolPtr(false),
	}, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.Join(childTags(t, string(out)), ",")

	if got != "editable,field,readable" {
		t.Errorf("expected alphabetical child order, got %s", got)
	}
}

func TestUnknownReportDeduplicates(t *testing.T) {
	report := newUnknownReport()
	report.add("userPermissions", "futureFlag")
	report.add("userPermissions", "futureFlag")
	report.add("userPermissions", "futureFlag")
	report.add("objectPermissions", "futurePermission")

	var out bytes.Buffer

	report.print(&out)

	want := "warning: objectPermissions: preserving unknown element <futurePermission> in 1 entry\n" +
		"warning: userPermissions: preserving unknown element <futureFlag> in 3 entries\n"

	if out.String() != want {
		t.Errorf("report = %q, want %q", out.String(), want)
	}
}

func TestNormalizeProfileCountsUnknownChildrenOnce(t *testing.T) {
	rawXML := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <userPermissions>
        <enabled>true</enabled>
        <futureFlag>true</futureFlag>
        <name>First</name>
    </userPermissions>
    <userPermissions>
        <enabled>true</enabled>
        <futureFlag>true</futureFlag>
        <name>Second</name>
    </userPermissions>
</Profile>`

	g := graph.NewGraph()
	ms := schema.NewMasterSchema()
	ms.Add("userPermissions", "First")
	ms.Add("userPermissions", "Second")
	g.SetMasterSchema(ms)

	p, err := g.AddProfile("Admin", "")
	if err != nil {
		t.Fatal(err)
	}

	p.RawXML = rawXML

	for _, name := range []string{"First", "Second"} {
		mn := g.GetOrCreateMetadataNode(graph.MetaTypeUserPerm, name)
		g.AddEdge(p, mn, graph.EdgeProperties{Enabled: boolPtr(true)})
	}

	report := newUnknownReport()
	if _, err := NormalizeProfile(p, g, report); err != nil {
		t.Fatalf("NormalizeProfile: %v", err)
	}

	var out bytes.Buffer

	report.print(&out)

	want := "warning: userPermissions: preserving unknown element <futureFlag> in 2 entries\n"

	if out.String() != want {
		t.Errorf("report = %q, want %q", out.String(), want)
	}
}

func TestRenderEntryPreservesUnknownChild(t *testing.T) {
	raw := `<objectPermissions>
        <allowCreate>true</allowCreate>
        <futurePermission>true</futurePermission>
        <object>Account</object>
        <viewAllRecords>false</viewAllRecords>
    </objectPermissions>`

	out, err := renderEntry("objectPermissions", "Account", graph.EdgeProperties{
		AllowCreate: boolPtr(true),
		ViewAll:     boolPtr(false),
	}, raw, nil)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.Join(childTags(t, string(out)), ",")

	if got != "allowCreate,allowDelete,allowEdit,allowRead,futurePermission,modifyAllRecords,object,viewAllRecords" {
		t.Errorf("expected unknown child preserved in alphabetical position, got %s:\n%s", got, out)
	}

	if !strings.Contains(string(out), "<futurePermission>true</futurePermission>") {
		t.Errorf("unknown child value was not preserved verbatim:\n%s", out)
	}
}

func TestEntryName(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		field string
		want  string
	}{
		{
			name:  "plain",
			body:  "<classAccesses><apexClass>MyClass</apexClass><enabled>true</enabled></classAccesses>",
			field: "apexClass",
			want:  "MyClass",
		},
		{
			name:  "surrounding whitespace",
			body:  "<fieldPermissions>\n    <field> Account.Name </field>\n</fieldPermissions>",
			field: "field",
			want:  "Account.Name",
		},
		{
			name:  "name field is not the first child",
			body:  "<layoutAssignments><layout>Account-Layout</layout><recordType>RT</recordType></layoutAssignments>",
			field: "layout",
			want:  "Account-Layout",
		},
		{
			name:  "no name field",
			body:  "<classAccesses><enabled>true</enabled></classAccesses>",
			field: "apexClass",
			want:  "",
		},
		{
			name:  "empty name field",
			body:  "<classAccesses><apexClass></apexClass></classAccesses>",
			field: "apexClass",
			want:  "",
		},
		{
			name:  "self-closing name field",
			body:  "<classAccesses><apexClass/></classAccesses>",
			field: "apexClass",
			want:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := entryName(tc.body, tc.field); got != tc.want {
				t.Errorf("entryName = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildRawEntryIndexSkipsUnnamedEntries(t *testing.T) {
	raw := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <classAccesses>
        <apexClass></apexClass>
        <enabled>true</enabled>
    </classAccesses>
    <classAccesses>
        <apexClass>Kept</apexClass>
        <enabled>false</enabled>
    </classAccesses>
</Profile>`

	idx, err := buildRawEntryIndex(raw)
	if err != nil {
		t.Fatalf("buildRawEntryIndex: %v", err)
	}

	if _, ok := idx["classAccesses"][""]; ok {
		t.Error("entry without a name must not be indexed")
	}

	if _, ok := idx["classAccesses"]["Kept"]; !ok {
		t.Error("named entry missing from the index")
	}
}

func TestProfileBlocksSortTopLevelElements(t *testing.T) {
	rawXML := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <userLicense>Salesforce</userLicense>
    <classAccesses>
        <apexClass>MyClass</apexClass>
        <enabled>true</enabled>
    </classAccesses>
    <custom>false</custom>
    <loginIpRanges>
        <startAddress>10.0.0.0</startAddress>
    </loginIpRanges>
    <description>Fixture profile</description>
</Profile>`

	ms := schema.NewMasterSchema()
	g := graph.NewGraph()
	g.SetMasterSchema(ms)

	p, _ := g.AddProfile("Admin", "Admin.profile-meta.xml")
	p.RawXML = rawXML
	p.UserLicense = "Salesforce"
	p.IsCustom = false
	p.HasCustomTag = true
	p.Description = "Fixture profile"

	classNode := g.GetOrCreateMetadataNode(graph.MetaTypeApexClass, "MyClass")
	g.AddEdge(p, classNode, graph.EdgeProperties{Enabled: boolPtr(true)})

	out := string(mustNormalize(t, p, g))
	got := strings.Join(childTags(t, out), ",")

	if got != "classAccesses,custom,description,loginIpRanges,userLicense" {
		t.Errorf("expected alphabetical top-level order, got %s:\n%s", got, out)
	}
}

func TestNormalizeProfilePreservesEmptyDescription(t *testing.T) {
	dir := t.TempDir()
	rawXML := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <custom>false</custom>
    <description></description>
    <userLicense>Salesforce</userLicense>
</Profile>`

	dst := filepath.Join(dir, "profiles", "Empty.profile-meta.xml")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(dst, []byte(rawXML), 0o644); err != nil {
		t.Fatal(err)
	}

	g, err := schema.LoadProfiles(dir)
	if err != nil {
		t.Fatalf("load project: %v", err)
	}

	profiles := g.Profiles()
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}

	got := string(mustNormalize(t, profiles[0], g))

	if !strings.Contains(got, "<description></description>") {
		t.Errorf("empty <description> was dropped:\n%s", got)
	}

	if !strings.Contains(got, "<userLicense>Salesforce</userLicense>") {
		t.Errorf("userLicense value was dropped:\n%s", got)
	}
}

func TestNormalizeProfileViewAllFieldsPreservedNotBackfilled(t *testing.T) {
	rawXML := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <objectPermissions>
        <allowCreate>false</allowCreate>
        <object>Account</object>
        <viewAllFields>true</viewAllFields>
        <viewAllRecords>false</viewAllRecords>
    </objectPermissions>
</Profile>`

	ms := schema.NewMasterSchema()
	ms.Add("objectPermissions", "Account")
	ms.Add("objectPermissions", "Contact")

	g := graph.NewGraph()
	g.SetMasterSchema(ms)

	p, _ := g.AddProfile("Admin", "Admin.profile-meta.xml")
	p.RawXML = rawXML

	account := g.GetOrCreateMetadataNode(graph.MetaTypeObject, "Account")
	g.AddEdge(p, account, graph.EdgeProperties{AllowCreate: boolPtr(false), ViewAll: boolPtr(false), ViewAllFields: boolPtr(true)})

	out := string(mustNormalize(t, p, g))

	if !strings.Contains(out, "<object>Account</object>\n        <viewAllFields>true</viewAllFields>") {
		t.Errorf("viewAllFields from the source entry must be preserved:\n%s", out)
	}

	backfilled := out[strings.Index(out, "<object>Contact</object>"):]

	if strings.Contains(backfilled, "viewAllFields") {
		t.Errorf("backfilled entries must not gain viewAllFields:\n%s", backfilled)
	}
}

// childTags lists the direct child element names of the single element in doc.
func childTags(t *testing.T, doc string) []string {
	t.Helper()

	root := struct {
		XMLName xml.Name
		Inner   []byte `xml:",innerxml"`
	}{}

	if err := xml.Unmarshal([]byte(doc), &root); err != nil {
		t.Fatalf("parse %q: %v", doc, err)
	}

	dec := xml.NewDecoder(bytes.NewReader(root.Inner))

	var tags []string

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return tags
		}

		if err != nil {
			t.Fatalf("parse children: %v", err)
		}

		if se, ok := tok.(xml.StartElement); ok {
			tags = append(tags, se.Name.Local)
			dec.Skip()
		}
	}
}

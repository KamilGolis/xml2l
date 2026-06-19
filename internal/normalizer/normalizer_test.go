package normalizer

import (
	"bytes"
	"encoding/xml"
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
		out, err := marshalEntry("classAccesses", "MyClass", p)
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
		out, err := marshalEntry("fieldPermissions", "Account.Revenue__c", p)
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
		out, err := marshalEntry("tabVisibilities", "standard-Account", p)
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
		out, err := marshalEntry("objectPermissions", "Account", p)
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
		out, err := marshalEntry("pageAccesses", "MyPage", p)
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
	p := g.AddProfile("Admin", "Admin.profile-meta.xml")

	// Profile has Existing__c and ClassA, but NOT Revenue__c or ClassB.
	fieldNode := g.GetOrCreateMetadataNode(graph.MetaTypeField, "Account.Existing__c")
	classNode := g.GetOrCreateMetadataNode(graph.MetaTypeApexClass, "ClassA")
	g.AddEdge(p, fieldNode, graph.EdgeProperties{Readable: boolPtr(true), Editable: boolPtr(true)})
	g.AddEdge(p, classNode, graph.EdgeProperties{Enabled: boolPtr(true)})

	xmlBytes := NormalizeProfile(p, g)
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
	p := g.AddProfile("Admin", "Admin.profile-meta.xml")

	// Profile has Account-Account Layout only.
	layoutNode := g.GetOrCreateMetadataNode(graph.MetaTypeLayout, "Account-Account Layout")
	g.AddEdge(p, layoutNode, graph.EdgeProperties{})

	xmlBytes := NormalizeProfile(p, g)
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
	p := g.AddProfile("Admin", "Admin.profile-meta.xml")

	// Add same layout with two record types + one without.
	layoutNode := g.GetOrCreateMetadataNode(graph.MetaTypeLayout, "Account-Account Layout")
	rt2 := "Account.RT2"
	rt1 := "Account.RT1"
	g.AddEdge(p, layoutNode, graph.EdgeProperties{RecordType: &rt2})
	g.AddEdge(p, layoutNode, graph.EdgeProperties{RecordType: &rt1})
	emptyRT := ""
	g.AddEdge(p, layoutNode, graph.EdgeProperties{RecordType: &emptyRT})

	xmlBytes := NormalizeProfile(p, g)
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
	p := g.AddProfile("Admin", "Admin.profile-meta.xml")

	// Profile has NO layout assignments at all.
	xmlBytes := NormalizeProfile(p, g)
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
	p := g.AddProfile("Admin", "Admin.profile-meta.xml")

	rt := "Account.HCO"
	node := g.GetOrCreateMetadataNode(graph.MetaTypeLayout, "Account-Custom Layout")
	g.AddEdge(p, node, graph.EdgeProperties{RecordType: &rt})

	xmlBytes := NormalizeProfile(p, g)
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
	p := g.AddProfile("Admin", "Admin.profile-meta.xml")

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

	xmlBytes := NormalizeProfile(p, g)
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
	p := g.AddProfile("Admin", "Admin.profile-meta.xml")

	rt := "Account.HCO"
	node1 := g.GetOrCreateMetadataNode(graph.MetaTypeLayout, "Account-Alpha Layout")
	node2 := g.GetOrCreateMetadataNode(graph.MetaTypeLayout, "Account-Beta Layout")
	g.AddEdge(p, node1, graph.EdgeProperties{RecordType: &rt})
	g.AddEdge(p, node2, graph.EdgeProperties{RecordType: &rt})

	xmlBytes := NormalizeProfile(p, g)
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
	p := g.AddProfile("Admin", "test.profile-meta.xml")
	p.UserLicense = "Salesforce"

	xmlBytes := NormalizeProfile(p, g)
	output := string(xmlBytes)

	if !strings.HasPrefix(output, `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Errorf("XML should start with header, got: %s", output[:50])
	}
	if !strings.Contains(output, `<Profile xmlns="http://soap.sforce.com/2006/04/metadata">`) {
		t.Error("missing Profile root element with namespace")
	}
}

func TestExtractUnmappedSections(t *testing.T) {
	rawXML := `<?xml version="1.0"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <userLicense>Salesforce</userLicense>
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
        <userLicense>Salesforce</userLicense>
        <flow>MyFlow</flow>
    </loginFlows>
    <profileActionOverrides>
        <actionName>Tab</actionName>
    </profileActionOverrides>
</Profile>`

	unmapped := extractUnmappedSections(rawXML)

	if !strings.Contains(unmapped, "loginHours") {
		t.Error("missing loginHours")
	}
	if !strings.Contains(unmapped, "loginIpRanges") {
		t.Error("missing loginIpRanges")
	}
	if !strings.Contains(unmapped, "loginFlows") {
		t.Error("missing loginFlows")
	}
	if !strings.Contains(unmapped, "profileActionOverrides") {
		t.Error("missing profileActionOverrides")
	}
	if !strings.Contains(unmapped, "Monday") {
		t.Error("missing loginHours content")
	}
	// Verify both loginIpRanges entries are preserved.
	if strings.Count(unmapped, "    <loginIpRanges>") != 2 {
		t.Errorf("expected 2 loginIpRanges entries, got %d", strings.Count(unmapped, "    <loginIpRanges>"))
	}
	if !strings.Contains(unmapped, "MyFlow") {
		t.Error("missing loginFlows content")
	}
}

func TestExtractUnmappedSectionsEmpty(t *testing.T) {
	if got := extractUnmappedSections(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	if got := extractUnmappedSections("<Profile/>"); got != "" {
		t.Errorf("expected empty for Profile-only, got %q", got)
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
	p := g.AddProfile("Admin", "test.profile-meta.xml")
	p.RawXML = rawXML
	p.UserLicense = "Salesforce"

	xmlBytes := NormalizeProfile(p, g)
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
	p := g.AddProfile("Admin", "test.profile-meta.xml")
	p.RawXML = rawXML
	p.UserLicense = "Salesforce"

	xmlBytes := NormalizeProfile(p, g)
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
	p := g.AddProfile("Test", "test_save_output.profile-meta.xml")
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
	p := g.AddProfile("Admin", "test.profile-meta.xml")
	p.UserLicense = "Salesforce"

	xmlBytes := NormalizeProfile(p, g)

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
	p := g.AddProfile("Admin", "test.profile-meta.xml")

	xmlBytes := NormalizeProfile(p, g)
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
	p := g.AddProfile("Admin", "test.profile-meta.xml")

	xmlBytes := NormalizeProfile(p, g)

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

func strPtr(s string) *string { return &s }

func boolPtr(b bool) *bool { return &b }

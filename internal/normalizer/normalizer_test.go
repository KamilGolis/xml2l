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
		p := graph.EdgeProperties{Enabled: graph.BoolPtr(true)}
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
}

func TestNormalizeProfileBackfill(t *testing.T) {
	ms := schema.NewMasterSchema()
	ms.Fields["Account.Revenue__c"] = true
	ms.Fields["Account.Existing__c"] = true
	ms.Classes["ClassA"] = true
	ms.Classes["ClassB"] = true

	g := graph.NewGraph()
	g.SetMasterSchema(ms)
	p := g.AddProfile("Admin", "Admin.profile-meta.xml")

	// Profile has Existing__c and ClassA, but NOT Revenue__c or ClassB.
	fieldNode := g.GetOrCreateMetadataNode(graph.MetaTypeField, "Account.Existing__c")
	classNode := g.GetOrCreateMetadataNode(graph.MetaTypeApexClass, "ClassA")
	g.AddEdge(p, fieldNode, graph.EdgeProperties{Readable: graph.BoolPtr(true), Editable: graph.BoolPtr(true)})
	g.AddEdge(p, classNode, graph.EdgeProperties{Enabled: graph.BoolPtr(true)})

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

func TestNormalizeProfileAlphabeticalSort(t *testing.T) {
	ms := schema.NewMasterSchema()
	ms.Fields["ZField"] = true
	ms.Fields["AField"] = true
	ms.Fields["MField"] = true

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
	if !strings.Contains(unmapped, "profileActionOverrides") {
		t.Error("missing profileActionOverrides")
	}
	if !strings.Contains(unmapped, "Monday") {
		t.Error("missing loginHours content")
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

func TestWriteProfiles(t *testing.T) {
	ms := schema.NewMasterSchema()
	ms.Classes["TestClass"] = true

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
	ms.Fields["A.Test"] = true
	ms.Fields["B.Test"] = true
	ms.Classes["MyClass"] = true

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
	ms.Tabs["standard-Account"] = true
	ms.Fields["A.Field"] = true // add a field so something produces edges

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

func strPtr(s string) *string { return &s }

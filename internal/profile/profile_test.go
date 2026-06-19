package profile

import (
	"strings"
	"testing"

	"xml2l/internal/scanner"
)

func TestDecodeBasicProfile(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <userLicense>Salesforce</userLicense>
    <custom>false</custom>
    <classAccesses>
        <apexClass>MyController</apexClass>
        <enabled>true</enabled>
    </classAccesses>
    <classAccesses>
        <apexClass>MyService</apexClass>
        <enabled>false</enabled>
    </classAccesses>
</Profile>`

	gt := scanner.GroundTruth{
		"MyController": true,
		"MyService":    true,
	}

	prof, err := Decode(strings.NewReader(xml), gt)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if prof.UserLicense != "Salesforce" {
		t.Errorf("expected UserLicense=Salesforce, got %q", prof.UserLicense)
	}
	if prof.HasCustomField != true {
		t.Error("expected HasCustomField=true")
	}
	if prof.IsCustom != false {
		t.Error("expected IsCustom=false")
	}

	entries, ok := prof.Sections["classAccesses"]
	if !ok {
		t.Fatal("expected classAccesses section")
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 classAccesses entries, got %d", len(entries))
	}

	if entries[0].Name != "MyController" || *entries[0].Enabled != true {
		t.Errorf("expected MyController enabled, got %+v", entries[0])
	}
	if entries[1].Name != "MyService" || *entries[1].Enabled != false {
		t.Errorf("expected MyService disabled, got %+v", entries[1])
	}
}

func TestDecodeGhostFiltering(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <fieldPermissions>
        <field>Account.Revenue__c</field>
        <readable>true</readable>
        <editable>true</editable>
    </fieldPermissions>
    <fieldPermissions>
        <field>Contact.GhostField__c</field>
        <readable>true</readable>
        <editable>false</editable>
    </fieldPermissions>
</Profile>`

	gt := scanner.GroundTruth{
		"Account.Revenue__c": true,
		// Contact.GhostField__c deliberately absent
	}

	prof, err := Decode(strings.NewReader(xml), gt)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	entries, ok := prof.Sections["fieldPermissions"]
	if !ok {
		t.Fatal("expected fieldPermissions section")
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 fieldPermissions entry (ghost dropped), got %d", len(entries))
	}
	if entries[0].Name != "Account.Revenue__c" {
		t.Errorf("expected Account.Revenue__c, got %q", entries[0].Name)
	}
	if *entries[0].Readable != true || *entries[0].Editable != true {
		t.Errorf("expected Readable=true Editable=true, got %+v", entries[0])
	}
}

func TestDecodeDedupViaGroundTruth(t *testing.T) {
	// Dedup happens because the GroundTruth map has unique keys.
	// Multiple identical entries in XML produce multiple entries in the
	// parsed result; dedup against the GroundTruth is by name check.
	// If the same field appears twice, both produce separate MetadataEntry
	// objects in the slice. True structural dedup (merge same name) is
	// handled by the graph layer (map key), not the parser.
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <fieldPermissions>
        <field>Account.Revenue__c</field>
        <readable>true</readable>
        <editable>true</editable>
    </fieldPermissions>
    <fieldPermissions>
        <field>Account.Revenue__c</field>
        <readable>false</readable>
        <editable>false</editable>
    </fieldPermissions>
</Profile>`

	gt := scanner.GroundTruth{
		"Account.Revenue__c": true,
	}

	prof, err := Decode(strings.NewReader(xml), gt)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	entries := prof.Sections["fieldPermissions"]
	if len(entries) != 2 {
		t.Fatalf("expected 2 raw entries (graph handles dedup), got %d", len(entries))
	}
}

func TestDecodeMultipleSectionTypes(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <userLicense>Salesforce</userLicense>
    <classAccesses>
        <apexClass>MyController</apexClass>
        <enabled>true</enabled>
    </classAccesses>
    <fieldPermissions>
        <field>Account.Revenue__c</field>
        <readable>true</readable>
        <editable>true</editable>
    </fieldPermissions>
    <objectPermissions>
        <object>Account</object>
        <allowCreate>true</allowCreate>
        <allowRead>true</allowRead>
        <allowEdit>false</allowEdit>
        <allowDelete>false</allowDelete>
    </objectPermissions>
    <userPermissions>
        <name>APIEnabled</name>
        <enabled>true</enabled>
    </userPermissions>
    <tabVisibilities>
        <tab>standard-Account</tab>
        <visibility>true</visibility>
    </tabVisibilities>
    <applicationVisibilities>
        <application>Sales</application>
        <default>false</default>
        <visible>true</visible>
    </applicationVisibilities>
</Profile>`

	gt := scanner.GroundTruth{
		"MyController":       true,
		"Account.Revenue__c": true,
	}

	prof, err := Decode(strings.NewReader(xml), gt)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Verify all expected section types present.
	sections := []string{"classAccesses", "fieldPermissions", "objectPermissions", "userPermissions", "tabVisibilities", "applicationVisibilities"}
	for _, s := range sections {
		if _, ok := prof.Sections[s]; !ok {
			t.Errorf("expected section %s to be present", s)
		}
	}

	// Verify specific values.
	if ca := prof.Sections["classAccesses"]; len(ca) == 1 && ca[0].Name != "MyController" {
		t.Errorf("expected MyController, got %q", ca[0].Name)
	}
	if fp := prof.Sections["fieldPermissions"]; len(fp) == 1 && *fp[0].Readable != true {
		t.Errorf("expected Readable=true, got %+v", fp[0])
	}
	if op := prof.Sections["objectPermissions"]; len(op) == 1 {
		if *op[0].AllowCreate != true || *op[0].AllowRead != true {
			t.Errorf("expected AllowCreate+AllowRead, got %+v", op[0])
		}
	}
	if up := prof.Sections["userPermissions"]; len(up) == 1 && up[0].Name != "APIEnabled" {
		t.Errorf("expected APIEnabled, got %q", up[0].Name)
	}
	if tv := prof.Sections["tabVisibilities"]; len(tv) == 1 && (tv[0].Visibility == nil || *tv[0].Visibility != "true") {
		t.Errorf("expected Visibility='true', got %+v", tv[0])
	}
	if av := prof.Sections["applicationVisibilities"]; len(av) == 1 {
		if *av[0].Visible != true || *av[0].Default != false {
			t.Errorf("expected Visible=true Default=false, got %+v", av[0])
		}
	}
}

func TestDecodeEmptyProfile(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <userLicense>Salesforce</userLicense>
</Profile>`

	gt := scanner.GroundTruth{}
	prof, err := Decode(strings.NewReader(xml), gt)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if prof.UserLicense != "Salesforce" {
		t.Errorf("expected Salesforce, got %q", prof.UserLicense)
	}
	if len(prof.Sections) != 0 {
		t.Errorf("expected no sections, got %d", len(prof.Sections))
	}
}

func TestDecodeRawSectionPreserved(t *testing.T) {
	// loginHours, loginIpRanges, profileActionOverrides should be skipped
	// (consumed but not added to Sections) since they don't produce edges.
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <loginHours>
        <startHour>8</startHour>
        <endHour>17</endHour>
    </loginHours>
    <loginIpRanges>
        <startAddress>192.168.0.0</startAddress>
        <endAddress>192.168.255.255</endAddress>
    </loginIpRanges>
</Profile>`

	gt := scanner.GroundTruth{}
	prof, err := Decode(strings.NewReader(xml), gt)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Raw sections should NOT appear in Sections map.
	if _, ok := prof.Sections["loginHours"]; ok {
		t.Error("loginHours should not produce edges in Sections")
	}
	if _, ok := prof.Sections["loginIpRanges"]; ok {
		t.Error("loginIpRanges should not produce edges in Sections")
	}
}

func TestDecodeTabVisibilityStringEnums(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <tabVisibilities>
        <tab>standard-Account</tab>
        <visibility>DefaultOn</visibility>
    </tabVisibilities>
    <tabVisibilities>
        <tab>standard-Opportunity</tab>
        <visibility>DefaultOff</visibility>
    </tabVisibilities>
    <tabVisibilities>
        <tab>standard-Case</tab>
        <visibility>Hidden</visibility>
    </tabVisibilities>
</Profile>`

	gt := scanner.GroundTruth{}
	prof, err := Decode(strings.NewReader(xml), gt)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	tv := prof.Sections["tabVisibilities"]
	if len(tv) != 3 {
		t.Fatalf("expected 3 tab vis entries, got %d", len(tv))
	}

	// Verify each entry has the correct string value stored in Visibility.
	expectations := map[string]string{
		"standard-Account":     "DefaultOn",
		"standard-Opportunity": "DefaultOff",
		"standard-Case":        "Hidden",
	}
	for _, entry := range tv {
		expected, ok := expectations[entry.Name]
		if !ok {
			t.Errorf("unexpected tab %q", entry.Name)
			continue
		}
		if entry.Visibility == nil || *entry.Visibility != expected {
			t.Errorf("tab %q: expected Visibility=%q, got %+v", entry.Name, expected, entry.Visibility)
		}
	}
}

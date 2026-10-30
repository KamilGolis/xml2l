package converter

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestYAMLToXML(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "mapping",
			yaml: `Profile:
  userLicense: "Salesforce"
  custom: "false"
`,
			want: `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <userLicense>Salesforce</userLicense>
    <custom>false</custom>
</Profile>
`,
		},
		{
			name: "sequence of mappings",
			yaml: `Profile:
  classAccesses:
    - apexClass: "MyController"
      enabled: "true"
    - apexClass: "MyService"
      enabled: "false"
`,
			want: `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <classAccesses>
        <apexClass>MyController</apexClass>
        <enabled>true</enabled>
    </classAccesses>
    <classAccesses>
        <apexClass>MyService</apexClass>
        <enabled>false</enabled>
    </classAccesses>
</Profile>
`,
		},
		{
			name: "sequence of scalars",
			yaml: `Profile:
  ipRanges:
    - "10.0.0.0"
    - "10.0.0.1"
`,
			want: `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <ipRanges>10.0.0.0</ipRanges>
    <ipRanges>10.0.0.1</ipRanges>
</Profile>
`,
		},
		{
			name: "empty value",
			yaml: `Profile:
  description: ""
`,
			want: `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <description></description>
</Profile>
`,
		},
		{
			name: "escaping",
			yaml: `Profile:
  description: "a & b <c>"
`,
			want: `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <description>a &amp; b &lt;c&gt;</description>
</Profile>
`,
		},
		{
			name: "nested mapping",
			yaml: `Profile:
  objectPermissions:
    object: "Account"
    allowRead: "true"
`,
			want: `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <objectPermissions>
        <allowRead>true</allowRead>
        <object>Account</object>
    </objectPermissions>
</Profile>
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := YAMLToXML([]byte(tc.yaml), "Profile")
			if err != nil {
				t.Fatalf("YAMLToXML: %v", err)
			}

			if string(got) != tc.want {
				t.Errorf("XML mismatch:\n---got---\n%s\n---want---\n%s", got, tc.want)
			}
		})
	}
}

func TestYAMLToXMLErrors(t *testing.T) {
	cases := []struct {
		name string
		yaml string
	}{
		{"wrong root", "PermissionSet:\n  label: \"x\"\n"},
		{"multiple documents", "Profile:\n  custom: \"false\"\n---\nProfile:\n  custom: \"true\"\n"},
		{"nested sequences", "Profile:\n  x:\n    - - \"a\"\n"},
		{"invalid element name", "Profile:\n  \"bad key\": \"x\"\n"},
		{"sequence root", "- Profile\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := YAMLToXML([]byte(tc.yaml), "Profile"); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// TestYAMLWriterHoistsIdentifyingChild pins the readability rule: the entry's
// identifying element opens the YAML item even though the XML sorts it elsewhere.
func TestYAMLWriterHoistsIdentifyingChild(t *testing.T) {
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <fieldPermissions>
        <editable>false</editable>
        <field>Account.Name</field>
        <readable>true</readable>
    </fieldPermissions>
    <fieldPermissions>
        <editable>true</editable>
        <field>Account.Other</field>
        <readable>true</readable>
    </fieldPermissions>
</Profile>
`

	dir := t.TempDir()
	path := filepath.Join(dir, "Admin.profile-meta.xml")

	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}

	_, yamlBytes, err := ConvertProfileFile(path)
	if err != nil {
		t.Fatalf("ConvertProfileFile: %v", err)
	}

	yamlText := string(yamlBytes)

	if !strings.Contains(yamlText, `- field: "Account.Name"`) {
		t.Errorf("identifying child should open the item:\n%s", yamlText)
	}

	if strings.Index(yamlText, `- field:`) > strings.Index(yamlText, `editable:`) {
		t.Errorf("identifying child should come before the other children:\n%s", yamlText)
	}
}

// TestYAMLToXMLSortsEntryChildren pins the write-back rule: an entry's children
// are emitted in Salesforce's order (alphabetical), whatever the YAML shows.
func TestYAMLToXMLSortsEntryChildren(t *testing.T) {
	yamlDoc := `Profile:
  fieldPermissions:
    - field: "Account.Name"
      readable: "true"
      editable: "false"
`

	got, err := YAMLToXML([]byte(yamlDoc), "Profile")
	if err != nil {
		t.Fatalf("YAMLToXML: %v", err)
	}

	want := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <fieldPermissions>
        <editable>false</editable>
        <field>Account.Name</field>
        <readable>true</readable>
    </fieldPermissions>
</Profile>
`

	if string(got) != want {
		t.Errorf("XML mismatch:\n---got---\n%s\n---want---\n%s", got, want)
	}
}

// TestRoundTripCanonicalisesChildOrder documents the one thing a round trip can
// change: an entry whose children are not alphabetical comes back in canonical
// order.
func TestRoundTripCanonicalisesChildOrder(t *testing.T) {
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <fieldPermissions>
        <field>Account.Name</field>
        <editable>false</editable>
        <readable>true</readable>
    </fieldPermissions>
</Profile>
`

	dir := t.TempDir()
	path := filepath.Join(dir, "Admin.profile-meta.xml")

	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}

	_, yamlBytes, err := ConvertProfileFile(path)
	if err != nil {
		t.Fatalf("ConvertProfileFile: %v", err)
	}

	got, err := YAMLToXML(yamlBytes, "Profile")
	if err != nil {
		t.Fatalf("YAMLToXML: %v", err)
	}

	want := strings.Replace(doc, `        <field>Account.Name</field>
        <editable>false</editable>`, `        <editable>false</editable>
        <field>Account.Name</field>`, 1)

	if string(got) != want {
		t.Errorf("children were not canonicalised:\n---got---\n%s\n---want---\n%s", got, want)
	}
}

// TestRoundTripPreservesMultiLineText guards the YAML quoting: an unescaped
// newline inside a double-quoted scalar is folded to a space by YAML parsers.
func TestRoundTripPreservesMultiLineText(t *testing.T) {
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <description>line one
line two</description>
</Profile>
`

	dir := t.TempDir()
	path := filepath.Join(dir, "ML.profile-meta.xml")

	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}

	_, yamlBytes, err := ConvertProfileFile(path)
	if err != nil {
		t.Fatalf("ConvertProfileFile: %v", err)
	}

	if !strings.Contains(string(yamlBytes), `description: "line one\nline two"`) {
		t.Errorf("newline should be escaped on one line:\n%s", yamlBytes)
	}

	xmlBytes, err := YAMLToXML(yamlBytes, "Profile")
	if err != nil {
		t.Fatalf("YAMLToXML: %v", err)
	}

	if string(xmlBytes) != doc {
		t.Errorf("multi-line text changed:\n---got---\n%s\n---want---\n%s", xmlBytes, doc)
	}
}

// TestRoundTripPreservesChildOrder covers a normalized profile: the YAML hoists
// the identifying child, and the writer restores the alphabetical XML order, so
// the round trip is exact.
func TestRoundTripPreservesChildOrder(t *testing.T) {
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <fieldPermissions>
        <editable>false</editable>
        <field>Account.Name</field>
        <readable>true</readable>
    </fieldPermissions>
</Profile>
`

	dir := t.TempDir()
	path := filepath.Join(dir, "Admin.profile-meta.xml")

	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}

	_, yamlBytes, err := ConvertProfileFile(path)
	if err != nil {
		t.Fatalf("ConvertProfileFile: %v", err)
	}

	xmlBytes, err := YAMLToXML(yamlBytes, "Profile")
	if err != nil {
		t.Fatalf("YAMLToXML: %v", err)
	}

	if string(xmlBytes) != doc {
		t.Errorf("child order changed:\n---got---\n%s\n---want---\n%s", xmlBytes, doc)
	}
}

// TestRoundTripProfile converts a fixture to YAML and back, then compares the
// element trees: formatting is canonical, structure must be identical.
func TestRoundTripProfile(t *testing.T) {
	input := filepath.Join("testdata", "Admin.profile-meta.xml")

	_, yamlBytes, err := ConvertProfileFile(input)
	if err != nil {
		t.Fatalf("ConvertProfileFile: %v", err)
	}

	xmlBytes, err := YAMLToXML(yamlBytes, "Profile")
	if err != nil {
		t.Fatalf("YAMLToXML: %v", err)
	}

	want := parseFixture(t, input)
	got := parseBytes(t, xmlBytes)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip changed the element tree:\n---got---\n%+v\n---want---\n%+v", got, want)
	}
}

func TestRoundTripPermSet(t *testing.T) {
	input := filepath.Join("testdata", "Sample.permissionset-meta.xml")

	_, yamlBytes, err := ConvertPermSetFile(input)
	if err != nil {
		t.Fatalf("ConvertPermSetFile: %v", err)
	}

	xmlBytes, err := YAMLToXML(yamlBytes, "PermissionSet")
	if err != nil {
		t.Fatalf("YAMLToXML: %v", err)
	}

	want := parseFixture(t, input)
	got := parseBytes(t, xmlBytes)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip changed the element tree:\n---got---\n%+v\n---want---\n%+v", got, want)
	}
}

func TestConvertProfilesYAMLToXML(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "Admin.profile-meta.yaml"), []byte(wantProfileYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ConvertProfiles(dir, "", "", "yaml"); err != nil {
		t.Fatalf("ConvertProfiles: %v", err)
	}

	got := parseFixture(t, filepath.Join(dir, "Admin.profile-meta.xml"))
	want := parseFixture(t, filepath.Join("testdata", "Admin.profile-meta.xml"))

	if !reflect.DeepEqual(got, want) {
		t.Error("written XML does not match the original element tree")
	}
}

func TestConvertProfilesNameAcceptsFileSuffix(t *testing.T) {
	dir := t.TempDir()
	copyFixture(t, "Admin.profile-meta.xml", dir)

	for _, name := range []string{"Admin", "Admin.xml", "Admin.profile-meta.xml"} {
		if err := ConvertProfiles(dir, name, "", ""); err != nil {
			t.Errorf("name %q: %v", name, err)
		}
	}
}

func TestParseFromType(t *testing.T) {
	for _, value := range []string{"", "xml", "XML"} {
		if got, err := ParseFromType(value); err != nil || got != FromTypeXML {
			t.Errorf("ParseFromType(%q) = %q, %v; want xml", value, got, err)
		}
	}

	if got, err := ParseFromType("YAML"); err != nil || got != FromTypeYAML {
		t.Errorf("ParseFromType(YAML) = %q, %v; want yaml", got, err)
	}

	if _, err := ParseFromType("json"); err == nil || !strings.Contains(err.Error(), "supported: xml, yaml") {
		t.Errorf("expected an unsupported-type error, got %v", err)
	}
}

// parseFixture parses an XML file into the converter's element tree.
func parseFixture(t *testing.T, path string) bufNode {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	node, err := parseXML(xml.NewDecoder(f))
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	return node
}

// parseBytes parses XML content into the converter's element tree.
func parseBytes(t *testing.T, data []byte) bufNode {
	t.Helper()

	node, err := parseXML(xml.NewDecoder(strings.NewReader(string(data))))
	if err != nil {
		t.Fatalf("parse converted XML: %v", err)
	}

	return node
}

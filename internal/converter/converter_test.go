package converter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Fixtures live under testdata/, which is tracked in version control (unlike
// the gitignored data/ directory), so the suite runs on a clean checkout.

const wantProfileYAML = `Profile:
  userLicense: "Salesforce"
  custom: "false"
  classAccesses:
    - apexClass: "MyController"
      enabled: "true"
    - apexClass: "MyService"
      enabled: "false"
  applicationVisibilities:
    application: "Sales"
    default: "false"
    visible: "true"
  fieldPermissions:
    field: "Account.Revenue__c"
    editable: "true"
    readable: "true"
  tabVisibilities:
    tab: "standard-Account"
    visibility: "DefaultOn"
`

const wantPermSetYAML = `PermissionSet:
  description: "Sample integration user access"
  hasActivationRequired: "true"
  label: "Sample Integration User"
  license: "Salesforce"
  objectPermissions:
    object: "Account"
    allowCreate: "false"
    allowDelete: "false"
    allowEdit: "false"
    allowRead: "true"
    modifyAllRecords: "false"
    viewAllRecords: "true"
`

func TestConvertProfileFile(t *testing.T) {
	out, yamlBytes, err := ConvertProfileFile(filepath.Join("testdata", "Admin.profile-meta.xml"))
	if err != nil {
		t.Fatalf("ConvertProfileFile: %v", err)
	}

	if want := filepath.Join("testdata", "Admin.profile-meta.yaml"); out != want {
		t.Errorf("output path = %q, want %q", out, want)
	}

	if got := string(yamlBytes); got != wantProfileYAML {
		t.Errorf("unexpected YAML:\n---got---\n%s\n---want---\n%s", got, wantProfileYAML)
	}

	if strings.Contains(string(yamlBytes), "xmlns") {
		t.Error("YAML output should not contain xmlns")
	}
}

func TestConvertPermSetFile(t *testing.T) {
	out, yamlBytes, err := ConvertPermSetFile(filepath.Join("testdata", "Sample.permissionset-meta.xml"))
	if err != nil {
		t.Fatalf("ConvertPermSetFile: %v", err)
	}

	if want := filepath.Join("testdata", "Sample.permissionset-meta.yaml"); out != want {
		t.Errorf("output path = %q, want %q", out, want)
	}

	if got := string(yamlBytes); got != wantPermSetYAML {
		t.Errorf("unexpected YAML:\n---got---\n%s\n---want---\n%s", got, wantPermSetYAML)
	}
}

func TestConvertProfilesWritesToDisk(t *testing.T) {
	dir := t.TempDir()
	copyFixture(t, "Admin.profile-meta.xml", dir)

	if err := ConvertProfiles(dir, "", "", ""); err != nil {
		t.Fatalf("ConvertProfiles: %v", err)
	}

	outPath := filepath.Join(dir, "Admin.profile-meta.yaml")

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read converted file: %v", err)
	}

	if string(got) != wantProfileYAML {
		t.Errorf("written YAML differs from expected:\n---got---\n%s", got)
	}

	// The source XML must be left untouched.
	if _, err := os.Stat(filepath.Join(dir, "Admin.profile-meta.xml")); err != nil {
		t.Errorf("source XML should still exist: %v", err)
	}
}

func TestConvertPermSetsWritesToDisk(t *testing.T) {
	dir := t.TempDir()
	copyFixture(t, "Sample.permissionset-meta.xml", dir)

	if err := ConvertPermSets(dir, "", "", ""); err != nil {
		t.Fatalf("ConvertPermSets: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "Sample.permissionset-meta.yaml"))
	if err != nil {
		t.Fatalf("read converted file: %v", err)
	}

	if string(got) != wantPermSetYAML {
		t.Errorf("written YAML differs from expected:\n---got---\n%s", got)
	}
}

func TestConvertProfilesFilterMissingName(t *testing.T) {
	dir := t.TempDir()
	copyFixture(t, "Admin.profile-meta.xml", dir)

	err := ConvertProfiles(dir, "DoesNotExist", "", "")
	if err == nil {
		t.Fatal("expected an error for an unknown profile name")
	}

	msg := err.Error()
	if !strings.Contains(msg, "DoesNotExist") || !strings.Contains(msg, "Admin") {
		t.Errorf("error should name the missing profile and the available ones, got: %s", msg)
	}
}

// copyFixture copies a testdata fixture into dir using its base name.
func copyFixture(t *testing.T, name, dir string) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}

	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
}

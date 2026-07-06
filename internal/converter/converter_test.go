package converter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConvertFile(t *testing.T) {
	// Locate a sample profile relative to the test file's project root.
	// We walk up until we find the go.mod.
	dir, _ := os.Getwd()

	for !fileExists(filepath.Join(dir, "go.mod")) && dir != "/" {
		dir = filepath.Dir(dir)
	}

	sample := filepath.Join(dir, "data", "src-dx", "main", "default", "profiles", "Admin.profile-meta.xml")

	out, yaml, err := ConvertProfileFile(sample)
	if err != nil {
		t.Fatalf("ConvertFile: %v", err)
	}

	if !strings.HasSuffix(out, ".profile-meta.yaml") {
		t.Errorf("output path should end with .profile-meta.yaml, got %s", out)
	}

	if len(yaml) == 0 {
		t.Fatal("YAML output is empty")
	}

	output := string(yaml)

	// Verify no namespace cruft
	if strings.Contains(output, "xmlns") {
		t.Error("YAML output should not contain xmlns")
	}

	// Verify some known content
	if !strings.Contains(output, "userLicense:") {
		t.Error("YAML output should contain userLicense")
	}

	if !strings.Contains(output, "applicationVisibilities:") {
		t.Error("YAML output should contain applicationVisibilities")
	}

	// Sequence items should start with "- "
	if !strings.Contains(output, "\n  - ") && !strings.Contains(output, "\n- ") {
		t.Log("Note: no sequence items found (may be ok for small profiles)")
	}

	t.Logf("Output path: %s", out)
	t.Logf("---YAML---\n%s---end---", output)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestConvertPermSetFile(t *testing.T) {
	dir, _ := os.Getwd()

	for !fileExists(filepath.Join(dir, "go.mod")) && dir != "/" {
		dir = filepath.Dir(dir)
	}

	sample := filepath.Join(dir, "data", "sfdcInternalInt__sfdc_scrt2.permissionset-meta.xml")

	out, yaml, err := ConvertPermSetFile(sample)
	if err != nil {
		t.Fatalf("ConvertPermSetFile: %v", err)
	}

	if !strings.HasSuffix(out, ".permissionset-meta.yaml") {
		t.Errorf("output path should end with .permissionset-meta.yaml, got %s", out)
	}

	if len(yaml) == 0 {
		t.Fatal("YAML output is empty")
	}

	output := string(yaml)

	// Verify no namespace cruft
	if strings.Contains(output, "xmlns") {
		t.Error("YAML output should not contain xmlns")
	}

	// Verify known content from the fixture
	if !strings.Contains(output, "hasActivationRequired:") {
		t.Error("YAML output should contain hasActivationRequired")
	}

	if !strings.Contains(output, "objectPermissions:") {
		t.Error("YAML output should contain objectPermissions")
	}

	if !strings.Contains(output, "license:") {
		t.Error("YAML output should contain license")
	}
	// Sequence items (objectPermissions is a single element with children,
	// rendered as a nested map, not a sequence — only repeated elements get "- ").
	if !strings.Contains(output, "\n  - ") && !strings.Contains(output, "\n- ") {
		t.Log("Note: no sequence items found (single objectPermissions renders as nested map)")
	}

	t.Logf("Output path: %s", out)
	t.Logf("---YAML---\n%s---end---", output)
}

func TestConvertPermSetFile_WriteRoundtrip(t *testing.T) {
	dir, _ := os.Getwd()

	for !fileExists(filepath.Join(dir, "go.mod")) && dir != "/" {
		dir = filepath.Dir(dir)
	}

	src := filepath.Join(dir, "data", "sfdcInternalInt__sfdc_scrt2.permissionset-meta.xml")

	// Copy fixture to temp dir
	tmpDir := t.TempDir()
	input := filepath.Join(tmpDir, "sfdcInternalInt__sfdc_scrt2.permissionset-meta.xml")

	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(input, data, 0o644); err != nil {
		t.Fatalf("write fixture copy: %v", err)
	}

	out, yaml, err := ConvertPermSetFile(input)
	if err != nil {
		t.Fatalf("ConvertPermSetFile: %v", err)
	}

	// Write the YAML to disk (ConvertPermSetFile returns content; writing is done by ConvertPermSets)
	if err := os.WriteFile(out, yaml, 0o644); err != nil {
		t.Fatalf("write yaml output: %v", err)
	}

	if _, err := os.Stat(out); err != nil {
		t.Errorf("output file should exist on disk: %v", err)
	}

	if len(yaml) == 0 {
		t.Fatal("YAML output is empty")
	}

	// Read back and verify content
	readback, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read back yaml: %v", err)
	}

	output := string(readback)
	if !strings.Contains(output, "hasActivationRequired:") {
		t.Error("YAML output should contain hasActivationRequired")
	}
}

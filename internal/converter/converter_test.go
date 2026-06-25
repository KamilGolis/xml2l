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

	out, yaml, err := ConvertFile(sample)
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

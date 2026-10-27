package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// execute runs the CLI with args and returns everything it printed.
func execute(t *testing.T, args ...string) (string, error) {
	t.Helper()

	root := &cobra.Command{Use: "xml2l"}
	root.AddCommand(newProfileCmd(), newPermSetCmd())

	var out bytes.Buffer

	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)

	err := root.Execute()

	return out.String(), err
}

// writeProject creates a metadata directory with one profile.
func writeProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	profiles := filepath.Join(dir, "profiles")

	if err := os.MkdirAll(profiles, 0o755); err != nil {
		t.Fatal(err)
	}

	// Deliberately not in Salesforce order, so a save has something to sort.
	profile := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <userLicense>Salesforce</userLicense>
    <custom>false</custom>
</Profile>`

	if err := os.WriteFile(filepath.Join(profiles, "Only.profile-meta.xml"), []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestSaveFailsOnUndecodableProfile(t *testing.T) {
	dir := writeProject(t)
	truncatedPath := filepath.Join(dir, "profiles", "Trunc.profile-meta.xml")
	truncated := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <custom>false</custom>
    <userLicense>Salesforce</userLicense>
    <loginIpRanges>
        <startAddress>10.0.0.0</startAddress>
</Profile>`

	if err := os.WriteFile(truncatedPath, []byte(truncated), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := execute(t, "profile", "save", "-p", dir)

	if err == nil {
		t.Fatalf("expected save to fail for an undecodable profile, output:\n%s", out)
	}

	if !strings.Contains(err.Error(), "Trunc.profile-meta.xml") {
		t.Errorf("error should name the file, got: %v", err)
	}

	if strings.Contains(out, "Done. Profiles saved.") {
		t.Errorf("save must not report success when a profile was skipped:\n%s", out)
	}

	// The malformed profile is left exactly as it was.
	after, readErr := os.ReadFile(truncatedPath)
	if readErr != nil {
		t.Fatal(readErr)
	}

	if string(after) != truncated {
		t.Errorf("undecodable profile was modified:\n%s", after)
	}

	// The healthy profile is still saved and ordered.
	good, readErr := os.ReadFile(filepath.Join(dir, "profiles", "Only.profile-meta.xml"))
	if readErr != nil {
		t.Fatal(readErr)
	}

	if strings.Index(string(good), "<userLicense>") < strings.Index(string(good), "<custom>") {
		t.Errorf("healthy profile was not normalized:\n%s", good)
	}
}

func TestDiffWarnsOnUndecodableProfile(t *testing.T) {
	dir := writeProject(t)

	truncated := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <custom>false</custom>
    <userLicense>Salesforce</userLicense>
    <loginIpRanges>
        <startAddress>10.0.0.0</startAddress>
</Profile>`

	if err := os.WriteFile(filepath.Join(dir, "profiles", "Trunc.profile-meta.xml"), []byte(truncated), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _ := execute(t, "profile", "diff", "-p", dir)

	if !strings.Contains(out, "warning:") || !strings.Contains(out, "Trunc.profile-meta.xml") {
		t.Errorf("diff should warn about the skipped profile, got:\n%s", out)
	}
}

func TestRuntimeErrorDoesNotPrintUsage(t *testing.T) {
	out, err := execute(t, "profile", "diff", "-p", writeProject(t))

	if err == nil {
		t.Fatal("expected an error for a single-profile diff")
	}

	if !strings.Contains(out, "at least 2 profiles") {
		t.Errorf("expected the error text, got:\n%s", out)
	}

	if strings.Contains(out, "Usage:") {
		t.Errorf("runtime error should not print usage:\n%s", out)
	}
}

func TestFlagErrorPrintsUsage(t *testing.T) {
	out, err := execute(t, "profile", "save", "--no-such-flag")

	if err == nil {
		t.Fatal("expected an error for an unknown flag")
	}

	if !strings.Contains(out, "Usage:") {
		t.Errorf("unknown flag should print usage, got:\n%s", out)
	}
}

func TestMissingRequiredFlagPrintsUsage(t *testing.T) {
	out, err := execute(t, "profile", "save")

	if err == nil {
		t.Fatal("expected an error for a missing --path")
	}

	if !strings.Contains(out, "required flag --path") {
		t.Errorf("expected the missing-path error, got:\n%s", out)
	}

	if !strings.Contains(out, "Usage:") {
		t.Errorf("missing required flag should print usage, got:\n%s", out)
	}
}

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

	profile := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <custom>false</custom>
    <userLicense>Salesforce</userLicense>
</Profile>`

	if err := os.WriteFile(filepath.Join(profiles, "Only.profile-meta.xml"), []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}

	return dir
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

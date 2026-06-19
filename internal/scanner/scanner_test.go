package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanFindsClsAndFieldMeta(t *testing.T) {
	dir := t.TempDir()

	// Create 3 .cls files
	for _, name := range []string{"MyController.cls", "MyService.cls", "MyTest.cls"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("// test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create 2 .field-meta.xml files inside objects/Account/fields/
	fieldDir := filepath.Join(dir, "objects", "Account", "fields")
	if err := os.MkdirAll(fieldDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Revenue__c.field-meta.xml", "Status__c.field-meta.xml"} {
		if err := os.WriteFile(filepath.Join(fieldDir, name), []byte("<?xml version=\"1.0\"?>"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	gt, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Expect 5 keys: 3 classes + 2 fields
	if len(gt) != 5 {
		t.Errorf("expected 5 keys, got %d: %v", len(gt), gt)
	}

	// Check specific entries
	for _, key := range []string{"MyController", "MyService", "MyTest", "Account.Revenue__c", "Account.Status__c"} {
		if !gt[key] {
			t.Errorf("expected key %q in ground truth", key)
		}
	}
}

func TestScanExtensionDedup(t *testing.T) {
	dir := t.TempDir()

	// Create both .cls and .cls-meta.xml for the same class
	if err := os.WriteFile(filepath.Join(dir, "MyController.cls"), []byte("// test"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "MyController.cls-meta.xml"), []byte("<?xml?>"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a standalone .cls-meta.xml without matching .cls
	if err := os.WriteFile(filepath.Join(dir, "Orphan.cls-meta.xml"), []byte("<?xml?>"), 0644); err != nil {
		t.Fatal(err)
	}

	gt, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Should have exactly 2 keys (deduplicated)
	if len(gt) != 2 {
		t.Errorf("expected 2 unique keys, got %d: %v", len(gt), gt)
	}

	if !gt["MyController"] {
		t.Error("expected MyController in ground truth")
	}
	if !gt["Orphan"] {
		t.Error("expected Orphan in ground truth")
	}
}

func TestScanEmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	gt, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(gt) != 0 {
		t.Errorf("expected empty ground truth, got %d keys", len(gt))
	}
}

func TestScanRootIsolation(t *testing.T) {
	dir := t.TempDir()

	// Create a .cls inside the scanned root.
	if err := os.WriteFile(filepath.Join(dir, "Valid.cls"), []byte("// test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a sibling directory outside the scanned root.
	outsideDir := filepath.Join(dir, "..", "outside")
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(outsideDir)
	if err := os.WriteFile(filepath.Join(outsideDir, "Outside.cls"), []byte("// test"), 0644); err != nil {
		t.Fatal(err)
	}

	gt, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if !gt["Valid"] {
		t.Error("expected Valid in ground truth")
	}
	if gt["Outside"] {
		t.Error("Outside should NOT be in ground truth (outside scan root)")
	}
}

func TestExtractFieldIdentifier(t *testing.T) {
	tests := []struct {
		path     string
		fileName string
		want     string
	}{
		{
			path:     "/some/project/objects/Account/fields/Revenue__c.field-meta.xml",
			fileName: "Revenue__c.field-meta.xml",
			want:     "Account.Revenue__c",
		},
		{
			path:     "/some/project/objects/Opportunity/fields/Amount__c.field-meta.xml",
			fileName: "Amount__c.field-meta.xml",
			want:     "Opportunity.Amount__c",
		},
		{
			// Flat directory (no objects/ parent) — return just the field name.
			path:     "/some/project/fields/Revenue__c.field-meta.xml",
			fileName: "Revenue__c.field-meta.xml",
			want:     "Revenue__c",
		},
	}

	for _, tc := range tests {
		got := extractFieldIdentifier(tc.path, tc.fileName)
		if got != tc.want {
			t.Errorf("extractFieldIdentifier(%q, %q) = %q, want %q", tc.path, tc.fileName, got, tc.want)
		}
	}
}

func TestScanLayouts(t *testing.T) {
	dir := t.TempDir()
	layoutDir := filepath.Join(dir, "layouts")
	if err := os.MkdirAll(layoutDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"Account-Account Layout.layout-meta.xml",
		"Contact-Patient Layout.layout-meta.xml",
		"Account-HCO Layout.layout-meta.xml",
	} {
		if err := os.WriteFile(filepath.Join(layoutDir, name), []byte("<xml/>"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	layouts, err := ScanLayouts(dir)
	if err != nil {
		t.Fatalf("ScanLayouts failed: %v", err)
	}
	if len(layouts) != 3 {
		t.Fatalf("expected 3 layouts, got %d: %v", len(layouts), layouts)
	}
	// Should be sorted alphabetically.
	if layouts[0] != "Account-Account Layout" {
		t.Errorf("expected first layout Account-Account Layout, got %q", layouts[0])
	}
	if layouts[1] != "Account-HCO Layout" {
		t.Errorf("expected second layout Account-HCO Layout, got %q", layouts[1])
	}
	if layouts[2] != "Contact-Patient Layout" {
		t.Errorf("expected third layout Contact-Patient Layout, got %q", layouts[2])
	}
}

func TestScanLayoutsMissingDirectory(t *testing.T) {
	dir := t.TempDir()
	layouts, err := ScanLayouts(dir)
	if err != nil {
		t.Fatalf("ScanLayouts should not error on missing directory: %v", err)
	}
	if layouts != nil && len(layouts) != 0 {
		t.Errorf("expected empty slice for missing layouts dir, got %v", layouts)
	}
}

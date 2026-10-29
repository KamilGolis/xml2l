package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeUsageFile writes one file under dir, creating parent directories.
func writeUsageFile(t *testing.T, dir, rel, content string) {
	t.Helper()

	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanApexUsage(t *testing.T) {
	dir := t.TempDir()

	writeUsageFile(t, dir, "classes/Rest.cls", "@RestResource(urlMapping='/x')\nglobal class Rest {}")
	writeUsageFile(t, dir, "classes/Soap.cls", "global class Soap { webservice static void ping() {} }")
	writeUsageFile(t, dir, "classes/VfController.cls", "public class VfController {}")
	writeUsageFile(t, dir, "classes/VfExtension.cls", "public class VfExtension {}")
	writeUsageFile(t, dir, "classes/LwcController.cls", "public class LwcController {}")
	writeUsageFile(t, dir, "classes/AuraController.cls", "public class AuraController {}")
	writeUsageFile(t, dir, "classes/FlowAction.cls", "public class FlowAction {}")
	writeUsageFile(t, dir, "classes/Unused.cls", "public class Unused { @AuraEnabled public static void get() {} }")
	writeUsageFile(t, dir, "pages/Site.page", `<apex:page controller="VfController" extensions="VfExtension" />`)
	writeUsageFile(t, dir, "aura/bundle/bundle.cmp", `<aura:component controller="AuraController" />`)
	writeUsageFile(t, dir, "lwc/thing/thing.js", `import get from '@salesforce/apex/LwcController.get';`)
	writeUsageFile(t, dir, "flows/Do.flow-meta.xml", `<Flow><actionCalls><apexClass>FlowAction</apexClass></actionCalls></Flow>`)

	usage, err := ScanApexUsage(dir)
	if err != nil {
		t.Fatalf("ScanApexUsage: %v", err)
	}

	want := []string{
		"AuraController", "FlowAction", "LwcController", "Rest", "Soap",
		"VfController", "VfExtension",
	}

	if len(usage.Required) != len(want) {
		t.Fatalf("required = %v, want %v", keysOf(usage.Required), want)
	}

	for _, class := range want {
		if !usage.Required[class] {
			t.Errorf("expected %s to be required", class)
		}
	}

	if usage.Required["Unused"] {
		t.Error("@AuraEnabled without a call site must not be required")
	}

	if len(usage.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", usage.Warnings)
	}
}

func TestScanApexUsageIgnoresStandardController(t *testing.T) {
	dir := t.TempDir()

	writeUsageFile(t, dir, "classes/Known.cls", "public class Known {}")
	writeUsageFile(t, dir, "pages/Standard.page", `<apex:page standardController="Account" />`)

	usage, err := ScanApexUsage(dir)
	if err != nil {
		t.Fatalf("ScanApexUsage: %v", err)
	}

	if len(usage.Required) != 0 {
		t.Errorf("required = %v, want none: standardController is not an Apex class", keysOf(usage.Required))
	}

	if len(usage.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", usage.Warnings)
	}
}

func TestScanApexUsageNamespacedLWCImport(t *testing.T) {
	dir := t.TempDir()

	writeUsageFile(t, dir, "classes/NsController.cls", "public class NsController {}")
	writeUsageFile(t, dir, "lwc/thing/thing.js", `import get from '@salesforce/apex/ns.NsController.get';`)

	usage, err := ScanApexUsage(dir)
	if err != nil {
		t.Fatalf("ScanApexUsage: %v", err)
	}

	if !usage.Required["NsController"] {
		t.Errorf("required = %v, want NsController", keysOf(usage.Required))
	}
}

func TestScanApexUsageWarnsAboutMissingClass(t *testing.T) {
	dir := t.TempDir()

	writeUsageFile(t, dir, "classes/Known.cls", "public class Known {}")
	writeUsageFile(t, dir, "pages/Broken.page", `<apex:page controller="Missing" />`)
	writeUsageFile(t, dir, "pages/Other.page", `<apex:page extensions="Missing" />`)

	usage, err := ScanApexUsage(dir)
	if err != nil {
		t.Fatalf("ScanApexUsage: %v", err)
	}

	if len(usage.Required) != 0 {
		t.Errorf("required = %v, want none", keysOf(usage.Required))
	}

	if len(usage.Warnings) != 1 {
		t.Fatalf("warnings = %v, want one per class", usage.Warnings)
	}

	if !strings.Contains(usage.Warnings[0], "Missing") || !strings.Contains(usage.Warnings[0], "pages/Broken.page") {
		t.Errorf("warning should name the class and the first referencing file, got %q", usage.Warnings[0])
	}
}

func TestScanApexUsageRefusesProjectWithoutClasses(t *testing.T) {
	dir := t.TempDir()
	writeUsageFile(t, dir, "profiles/Admin.profile-meta.xml", "<Profile/>")

	if _, err := ScanApexUsage(dir); err == nil {
		t.Fatal("expected an error for a project without Apex classes")
	} else if !strings.Contains(err.Error(), "refusing to prune class access") {
		t.Errorf("unexpected error: %v", err)
	}
}

func keysOf(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}

	return out
}

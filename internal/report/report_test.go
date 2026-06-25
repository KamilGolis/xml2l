package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"xml2l/internal/graph"
)

func newTestGraph() *graph.SalesforceGraph {
	g := graph.NewGraph()
	_ = g.AddProfile("Admin", "")
	_ = g.AddProfile("Customer Service", "")

	return g
}

func addEdge(g *graph.SalesforceGraph, profileName, metaType, name string, props graph.EdgeProperties) {
	var pn *graph.ProfileNode

	for _, p := range g.ProfileNodes {
		if p.Name == profileName {
			pn = p
			break
		}
	}

	if pn == nil {
		return
	}

	mn := g.GetOrCreateMetadataNode(graph.MetadataType(metaType), name)
	g.AddEdge(pn, mn, props)
}

func TestComputeDiff_BasicMissing(t *testing.T) {
	g := newTestGraph()
	addEdge(g, "Admin", "ApexClass", "ClassA", graph.EdgeProperties{Enabled: boolPtr(true)})
	addEdge(g, "Customer Service", "ApexClass", "ClassB", graph.EdgeProperties{Enabled: boolPtr(true)})

	r := ComputeDiff(g, false, "")

	if len(r.Profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(r.Profiles))
	}

	// Admin should be missing ClassB.
	admin := findProfile(r, "Admin")

	if admin == nil {
		t.Fatal("Admin profile not found in report")
	}

	adminMissing := admin.Missing["ApexClass"]

	if !contains(adminMissing, "ClassB") {
		t.Errorf("Admin should be missing ClassB, got %v", adminMissing)
	}

	if contains(adminMissing, "ClassA") {
		t.Errorf("Admin should NOT be missing ClassA (it has it)")
	}

	// Customer Service should be missing ClassA.
	cs := findProfile(r, "Customer Service")

	if cs == nil {
		t.Fatal("Customer Service profile not found in report")
	}

	csMissing := cs.Missing["ApexClass"]

	if !contains(csMissing, "ClassA") {
		t.Errorf("Customer Service should be missing ClassA, got %v", csMissing)
	}

	if contains(csMissing, "ClassB") {
		t.Errorf("Customer Service should NOT be missing ClassB (it has it)")
	}
}

func TestComputeDiff_SharedTagSuppressed(t *testing.T) {
	g := newTestGraph()
	addEdge(g, "Admin", "ApexClass", "Logger", graph.EdgeProperties{Enabled: boolPtr(true)})
	addEdge(g, "Customer Service", "ApexClass", "Logger", graph.EdgeProperties{Enabled: boolPtr(true)})

	r := ComputeDiff(g, false, "")
	// Both profiles have Logger — it's shared, so no missing entries.
	for _, p := range r.Profiles {
		if p.Missing["ApexClass"] != nil {
			t.Errorf("profile %s should not have any missing ApexClass entries (Logger is shared)", p.ProfileName)
		}
	}
	// ValueDifferences should be empty without --details.
	if len(r.ValueDifferences) > 0 {
		t.Errorf("expected no value differences without --details, got %d", len(r.ValueDifferences))
	}
}

func TestComputeDiff_ValueDifferences(t *testing.T) {
	g := newTestGraph()
	addEdge(g, "Admin", "ApexClass", "ClassA", graph.EdgeProperties{Enabled: boolPtr(true)})
	addEdge(g, "Customer Service", "ApexClass", "ClassA", graph.EdgeProperties{Enabled: boolPtr(false)})

	r := ComputeDiff(g, true, "")

	if len(r.ValueDifferences) == 0 {
		t.Fatal("expected value differences with --details, got none")
	}

	found := false

	for _, vd := range r.ValueDifferences {
		if vd.MetaType == "ApexClass" && vd.Name == "ClassA" && vd.Field == "enabled" {
			found = true

			if vd.ValueA != "true" || vd.ValueB != "false" {
				t.Errorf("expected enabled=true vs false, got %s vs %s", vd.ValueA, vd.ValueB)
			}

			break
		}
	}

	if !found {
		t.Errorf("expected value diff for ClassA/enabled, got: %+v", r.ValueDifferences)
	}
}

func TestComputeDiff_IdenticalProfiles(t *testing.T) {
	g := graph.NewGraph()
	_ = g.AddProfile("Admin", "")
	_ = g.AddProfile("Clone", "")
	addEdge(g, "Admin", "ApexClass", "ClassA", graph.EdgeProperties{Enabled: boolPtr(true)})
	addEdge(g, "Admin", "ApexClass", "ClassB", graph.EdgeProperties{Enabled: boolPtr(true)})
	addEdge(g, "Clone", "ApexClass", "ClassA", graph.EdgeProperties{Enabled: boolPtr(true)})
	addEdge(g, "Clone", "ApexClass", "ClassB", graph.EdgeProperties{Enabled: boolPtr(true)})

	r := ComputeDiff(g, false, "")

	// Identical profiles — nothing missing, all tags shared.
	if len(r.Profiles) != 0 {
		t.Errorf("expected 0 profiles with missing entries for identical profiles, got %d", len(r.Profiles))
	}
}

func TestComputeDiff_DisjointProfiles(t *testing.T) {
	g := graph.NewGraph()
	_ = g.AddProfile("A", "")
	_ = g.AddProfile("B", "")
	addEdge(g, "A", "ApexClass", "ClassA", graph.EdgeProperties{Enabled: boolPtr(true)})
	addEdge(g, "B", "ApexClass", "ClassB", graph.EdgeProperties{Enabled: boolPtr(true)})

	r := ComputeDiff(g, false, "")

	if len(r.Profiles) != 2 {
		t.Fatalf("expected 2 profiles with missing entries, got %d", len(r.Profiles))
	}

	a := findProfile(r, "A")

	if a == nil || !contains(a.Missing["ApexClass"], "ClassB") {
		t.Errorf("profile A should be missing ClassB")
	}

	b := findProfile(r, "B")

	if b == nil || !contains(b.Missing["ApexClass"], "ClassA") {
		t.Errorf("profile B should be missing ClassA")
	}
}

func TestComputeDiff_SingleProfile(t *testing.T) {
	g := graph.NewGraph()
	_ = g.AddProfile("Only", "")
	addEdge(g, "Only", "ApexClass", "ClassA", graph.EdgeProperties{Enabled: boolPtr(true)})

	r := ComputeDiff(g, false, "")

	// Single profile: union == its own edges → no missing entries.
	if len(r.Profiles) != 0 {
		t.Errorf("expected 0 profiles with missing entries for single profile, got %d", len(r.Profiles))
	}
}

func TestComputeDiff_NilGraph(t *testing.T) {
	r := ComputeDiff(nil, false, "")

	if r == nil {
		t.Fatal("expected non-nil report for nil graph")
	}
}

func TestComputeDiff_EmptyGraph(t *testing.T) {
	g := graph.NewGraph()
	r := ComputeDiff(g, false, "")

	if r == nil {
		t.Fatal("expected non-nil report for empty graph")
	}

	if len(r.Profiles) != 0 {
		t.Errorf("expected 0 profiles for empty graph, got %d", len(r.Profiles))
	}
}

func TestFormatDiffText_Empty(t *testing.T) {
	r := &DiffReport{}
	text := FormatDiffText(r)

	if !strings.Contains(text, "No differences found") {
		t.Errorf("expected 'No differences found', got: %s", text)
	}
}

func TestFormatDiffText_Basic(t *testing.T) {
	r := &DiffReport{
		Profiles: []ProfileMissing{
			{
				ProfileName: "Admin",
				Missing:     map[string][]string{"ApexClass": {"ClassB", "ClassC"}},
			},
		},
	}

	text := FormatDiffText(r)

	if !strings.Contains(text, "[Admin]") {
		t.Errorf("expected [Admin] section, got: %s", text)
	}

	if !strings.Contains(text, "ClassB") || !strings.Contains(text, "ClassC") {
		t.Errorf("expected ClassB and ClassC in output, got: %s", text)
	}
}

func TestFormatDiffText_ValueDifferences(t *testing.T) {
	r := &DiffReport{
		ValueDifferences: []ValueDiff{
			{
				MetaKey:  MetaKey{MetaType: "ApexClass", Name: "Logger"},
				ProfileA: "Admin",
				ProfileB: "Customer Service",
				Field:    "enabled",
				ValueA:   "true",
				ValueB:   "false",
			},
		},
	}

	text := FormatDiffText(r)

	if !strings.Contains(text, "Value Differences") {
		t.Errorf("expected 'Value Differences' section, got: %s", text)
	}

	if !strings.Contains(text, "Admin") || !strings.Contains(text, "Customer Service") {
		t.Errorf("expected profile names in value diff output")
	}
}

func TestComputeDiff_MultipleMetaTypes(t *testing.T) {
	g := newTestGraph()
	addEdge(g, "Admin", "ApexClass", "Logger", graph.EdgeProperties{Enabled: boolPtr(true)})
	addEdge(g, "Admin", "CustomField", "Account.Industry", graph.EdgeProperties{Readable: boolPtr(true), Editable: boolPtr(true)})
	addEdge(g, "Customer Service", "ApexClass", "Logger", graph.EdgeProperties{Enabled: boolPtr(true)})

	r := ComputeDiff(g, false, "")

	// Admin has Logger and Account.Industry; Customer Service has only Logger.
	cs := findProfile(r, "Customer Service")

	if cs == nil {
		t.Fatal("Customer Service should appear in report")
	}

	if !contains(cs.Missing["CustomField"], "Account.Industry") {
		t.Errorf("Customer Service should be missing Account.Industry field")
	}

	// Logger is shared — Admin should not appear in report.
	admin := findProfile(r, "Admin")

	if admin != nil {
		t.Errorf("Admin should have no missing entries (only shared and its own tags), got %v", admin.Missing)
	}
}

func TestScanRepo_Basic(t *testing.T) {
	dir := t.TempDir()
	mkFile(t, dir, "classes/MyClass.cls")
	mkFile(t, dir, "classes/subdir/NestedClass.cls")
	mkFile(t, dir, "apps/MyApp.app-meta.xml")
	mkFile(t, dir, "tabs/MyTab.tab-meta.xml")
	mkFile(t, dir, "layouts/MyLayout.layout")
	mkFile(t, dir, "objects/Account.object-meta.xml")
	mkFile(t, dir, "objects/MyCustom__c.object-meta.xml")
	mkFile(t, dir, "objects/MyMetadata__mdt.object-meta.xml")
	mkFile(t, dir, "objects/Account/fields/Industry.field-meta.xml")
	mkFile(t, dir, "objects/Account/recordTypes/Customer.recordType-meta.xml")
	mkFile(t, dir, "objects/Account/validationRules/SomeRule.validationRule-meta.xml")
	mkFile(t, dir, "objects/Account/listViews/All.listView-meta.xml")

	repo, err := ScanRepo(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Check ApexClass (flat + nested).
	if !repo["ApexClass"]["MyClass"] {
		t.Error("expected MyClass in ApexClass results")
	}

	if !repo["ApexClass"]["NestedClass"] {
		t.Error("expected NestedClass (from subdir) in ApexClass results")
	}

	// Check flat types.
	if !repo["CustomApplication"]["MyApp"] {
		t.Error("expected MyApp in App results")
	}

	if !repo["CustomTab"]["MyTab"] {
		t.Error("expected MyTab in Tab results")
	}

	if !repo["Layout"]["MyLayout"] {
		t.Error("expected MyLayout in Layout results")
	}

	// Check Object types.
	if !repo["CustomObject"]["Account"] {
		t.Error("expected Account in Object results")
	}

	if !repo["CustomSetting"]["MyCustom__c"] {
		t.Error("expected MyCustom__c in CustomSetting results")
	}

	if !repo["CustomMetadata"]["MyMetadata__mdt"] {
		t.Error("expected MyMetadata__mdt in CustomMetadataType results")
	}

	// Check Field and RecordType (object subdirs).
	if !repo["CustomField"]["Account.Industry"] {
		t.Error("expected Account.Industry in Field results")
	}

	if !repo["RecordType"]["Account.Customer"] {
		t.Error("expected Account.Customer in RecordType results")
	}

	// Verify validationRules and listViews NOT scanned.
	if repo["CustomField"] != nil && repo["CustomField"]["Account.SomeRule"] {
		t.Error("validationRules should NOT be scanned")
	}

	if repo["CustomObject"] != nil && repo["CustomObject"]["Account.All"] {
		t.Error("listViews should NOT be scanned")
	}
}

func TestScanRepo_EmptyDirectories(t *testing.T) {
	dir := t.TempDir()
	mkFile(t, dir, "classes/.gitkeep")
	mkFile(t, dir, "apps/.gitkeep")

	repo, err := ScanRepo(dir)
	if err != nil {
		t.Fatal(err)
	}

	if repo["ApexClass"] == nil {
		t.Error("expected non-nil ApexClass map")
	}
}

func TestScanRepo_MissingDirectories(t *testing.T) {
	dir := t.TempDir()
	repo, err := ScanRepo(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(repo["ApexClass"]) != 0 {
		t.Error("expected empty ApexClass results")
	}
}

func TestCrossReference_MissingAndUnreferenced(t *testing.T) {
	g := graph.NewGraph()
	_ = g.AddProfile("Admin", "")

	g.GetOrCreateMetadataNode(graph.MetaTypeApexClass, "DeletedClass")

	repoFiles := map[string]map[string]bool{
		"ApexClass":         {"ExistingClass": true},
		"CustomApplication": {"MyApp": true},
	}

	xr := CrossReference(g, repoFiles)

	// Direction 1: graph→filesystem — DeletedClass has no file.
	foundMissing := false

	for _, entry := range xr.MissingFiles {
		if entry.MetaType == "ApexClass" && entry.Name == "DeletedClass" {
			foundMissing = true
			break
		}
	}

	if !foundMissing {
		t.Error("expected DeletedClass in missing files")
	}

	// Direction 2: filesystem→graph — ExistingClass and MyApp have no graph node.
	foundUnrefApex := false
	foundUnrefApp := false

	for _, entry := range xr.Unreferenced {
		if entry.MetaType == "ApexClass" && entry.Name == "ExistingClass" {
			foundUnrefApex = true
		}

		if entry.MetaType == "CustomApplication" && entry.Name == "MyApp" {
			foundUnrefApp = true
		}
	}

	if !foundUnrefApex {
		t.Error("expected ExistingClass in unreferenced")
	}

	if !foundUnrefApp {
		t.Error("expected MyApp in unreferenced")
	}
}

func TestCrossReference_ExcludedTypes(t *testing.T) {
	g := graph.NewGraph()
	_ = g.AddProfile("Admin", "")
	g.GetOrCreateMetadataNode(graph.MetaTypeUserPerm, "ModifyAllData")

	repoFiles := map[string]map[string]bool{}
	xr := CrossReference(g, repoFiles)

	for _, entry := range xr.MissingFiles {
		if entry.MetaType == "UserPermission" {
			t.Errorf("UserPermission should be excluded, found: %s/%s", entry.MetaType, entry.Name)
		}
	}

	if xr.SkippedTypesNote == "" {
		t.Error("expected skipped types note")
	}
}

func TestCrossReference_NilGraph(t *testing.T) {
	xr := CrossReference(nil, nil)

	if xr == nil {
		t.Fatal("expected non-nil result for nil graph")
	}
}

func TestCrossReference_EmptyGraph(t *testing.T) {
	g := graph.NewGraph()
	xr := CrossReference(g, map[string]map[string]bool{})

	if len(xr.MissingFiles) != 0 {
		t.Errorf("expected no missing files for empty graph, got %d", len(xr.MissingFiles))
	}
}

func TestFormatDiffText_CrossRef(t *testing.T) {
	r := &DiffReport{
		CrossRef: &RepoCrossRef{
			MissingFiles: []RepoEntry{
				{MetaType: "ApexClass", Name: "DeletedClass"},
			},
			Unreferenced: []RepoEntry{
				{MetaType: "CustomApplication", Name: "MyApp"},
			},
		},
	}

	text := FormatDiffText(r)

	if !strings.Contains(text, "Missing Files") {
		t.Errorf("expected 'Missing Files' section, got: %s", text)
	}

	if !strings.Contains(text, "Unreferenced Metadata") {
		t.Errorf("expected 'Unreferenced Metadata' section, got: %s", text)
	}

	if !strings.Contains(text, "DeletedClass") {
		t.Errorf("expected DeletedClass in output")
	}

	if !strings.Contains(text, "MyApp") {
		t.Errorf("expected MyApp in output")
	}
}

func TestComputeDiff_WithRepoPath(t *testing.T) {
	g := graph.NewGraph()
	_ = g.AddProfile("Admin", "")
	mn := g.GetOrCreateMetadataNode(graph.MetaTypeApexClass, "TestClass")
	admin := g.ProfileNodes["Admin"]
	g.AddEdge(admin, mn, graph.EdgeProperties{Enabled: boolPtr(true)})

	dir := t.TempDir()
	mkFile(t, dir, "classes/OtherClass.cls")

	r := ComputeDiff(g, true, dir)

	if r.CrossRef == nil {
		t.Fatal("expected crossRef when repo path is provided")
	}

	found := false

	for _, entry := range r.CrossRef.MissingFiles {
		if entry.MetaType == "ApexClass" && entry.Name == "TestClass" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected TestClass in missing files")
	}

	foundUnref := false

	for _, entry := range r.CrossRef.Unreferenced {
		if entry.MetaType == "ApexClass" && entry.Name == "OtherClass" {
			foundUnref = true
			break
		}
	}

	if !foundUnref {
		t.Error("expected OtherClass in unreferenced")
	}
}

// Helpers.
func findProfile(r *DiffReport, name string) *ProfileMissing {
	for _, p := range r.Profiles {
		if p.ProfileName == name {
			return &p
		}
	}

	return nil
}

func contains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}

	return false
}

func mkFile(t *testing.T, dir, path string) {
	t.Helper()
	full := filepath.Join(dir, path)

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(full, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
}

func boolPtr(b bool) *bool { return &b }

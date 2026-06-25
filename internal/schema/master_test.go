package schema

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"xml2l/internal/profile"
	"xml2l/internal/scanner"
)

func TestMasterSchemaMerge(t *testing.T) {
	ms := NewMasterSchema()

	prof1 := &profile.Profile{
		Sections: map[string][]profile.MetadataEntry{
			"classAccesses":    {{Name: "ControllerA"}, {Name: "ControllerB"}},
			"fieldPermissions": {{Name: "Account.Revenue__c"}},
		},
	}

	prof2 := &profile.Profile{
		Sections: map[string][]profile.MetadataEntry{
			"classAccesses":     {{Name: "ControllerB"}, {Name: "ControllerC"}},
			"objectPermissions": {{Name: "Account"}},
		},
	}

	ms.merge(prof1)
	ms.merge(prof2)

	// Classes: union of {A, B} + {B, C} = {A, B, C}
	classes := ms.AllNames("classAccesses")

	if len(classes) != 3 {
		t.Errorf("expected 3 classes, got %d: %v", len(classes), classes)
	}

	for _, c := range []string{"ControllerA", "ControllerB", "ControllerC"} {
		found := false

		for _, name := range classes {
			if name == c {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("missing class %q", c)
		}
	}

	// Fields: {Account.Revenue__c}
	fields := ms.AllNames("fieldPermissions")

	if len(fields) != 1 {
		t.Errorf("expected 1 field, got %d", len(fields))
	}

	if fields[0] != "Account.Revenue__c" {
		t.Errorf("missing field Account.Revenue__c, got %v", fields)
	}

	// Objects: {Account}
	objects := ms.AllNames("objectPermissions")

	if len(objects) != 1 {
		t.Errorf("expected 1 object, got %d", len(objects))
	}

	if objects[0] != "Account" {
		t.Errorf("missing object Account, got %v", objects)
	}
}

func TestMasterSchemaConcurrentMergeNoRace(t *testing.T) {
	ms := NewMasterSchema()
	var wg sync.WaitGroup

	// Spawn 10 goroutines all merging into the same MasterSchema.
	for i := 0; i < 10; i++ {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			prof := &profile.Profile{
				Sections: map[string][]profile.MetadataEntry{
					"classAccesses": {
						{Name: "SharedClass"},
						{Name: strings.Join([]string{"UniqueClass", string(rune('A' + idx))}, "")},
					},
				},
			}

			ms.merge(prof)
		}(i)
	}

	wg.Wait()

	// All 10 unique classes + 1 shared = 11 classes.
	classes := ms.AllNames("classAccesses")

	if len(classes) != 11 {
		t.Errorf("expected 11 classes (10 unique + 1 shared), got %d", len(classes))
	}
}

func TestPreAllocatedSliceCapacity(t *testing.T) {
	ms := NewMasterSchema()
	ms.entries["classAccesses"]["A"] = true
	ms.entries["classAccesses"]["B"] = true
	ms.entries["classAccesses"]["C"] = true

	all := ms.AllNames("classAccesses")

	if len(all) != 3 {
		t.Errorf("expected 3 classes, got %d", len(all))
	}

	if cap(all) != 3 {
		t.Errorf("expected capacity 3 (pre-allocated), got %d", cap(all))
	}
}

func TestRunConcurrentUnion(t *testing.T) {
	dir := t.TempDir()

	// Create 3 profile files with some overlapping and some unique entries.
	profile1 := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <classAccesses><apexClass>ClassA</apexClass><enabled>true</enabled></classAccesses>
    <classAccesses><apexClass>ClassB</apexClass><enabled>true</enabled></classAccesses>
    <fieldPermissions><field>Account.F1__c</field><readable>true</readable><editable>false</editable></fieldPermissions>
</Profile>`
	profile2 := `<?xml version="1.0" encoding="UTF-8"?>
<Profile xmlns="http://soap.sforce.com/2006/04/metadata">
    <classAccesses><apexClass>ClassB</apexClass><enabled>true</enabled></classAccesses>
    <classAccesses><apexClass>ClassC</apexClass><enabled>true</enabled></classAccesses>
    <fieldPermissions><field>Account.F2__c</field><readable>true</readable><editable>false</editable></fieldPermissions>
</Profile>`

	writeProfile(t, dir, "ProfileA.profile-meta.xml", profile1)
	writeProfile(t, dir, "ProfileB.profile-meta.xml", profile2)

	gt := scanner.GroundTruth{
		"ClassA": true, "ClassB": true, "ClassC": true,
		"Account.F1__c": true, "Account.F2__c": true,
	}

	paths := []string{
		filepath.Join(dir, "ProfileA.profile-meta.xml"),
		filepath.Join(dir, "ProfileB.profile-meta.xml"),
	}

	results, ms, errs := RunConcurrent(paths, gt)

	if len(errs) > 0 {
		t.Fatalf("RunConcurrent failed: %v", errs[0])
	}

	// Verify all profiles decoded.
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// Verify Master Schema union.
	classes := ms.AllNames("classAccesses")

	if len(classes) != 3 {
		t.Errorf("expected 3 unique classes (A+B+C), got %d", len(classes))
	}

	fields := ms.AllNames("fieldPermissions")

	if len(fields) != 2 {
		t.Errorf("expected 2 unique fields (F1+F2), got %d", len(fields))
	}
}

func TestRunConcurrentEmptyProfiles(t *testing.T) {
	results, ms, errs := RunConcurrent(nil, scanner.GroundTruth{})

	if len(errs) > 0 {
		t.Fatalf("RunConcurrent failed: %v", errs[0])
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}

	if ms == nil {
		t.Fatal("expected non-nil MasterSchema")
	}
}

func TestProfilePreAllocationScenarios(t *testing.T) {
	t.Run("classes slice pre-allocated", func(t *testing.T) {
		ms := NewMasterSchema()

		for i := 0; i < 100; i++ {
			name := string(rune('A' + i))
			ms.entries["classAccesses"][name] = true
		}

		all := ms.AllNames("classAccesses")

		if cap(all) != 100 {
			t.Errorf("expected cap 100, got %d", cap(all))
		}
	})

	t.Run("fields slice pre-allocated", func(t *testing.T) {
		ms := NewMasterSchema()
		ms.entries["fieldPermissions"]["a"] = true
		ms.entries["fieldPermissions"]["b"] = true
		all := ms.AllNames("fieldPermissions")

		if cap(all) != 2 {
			t.Errorf("expected cap 2, got %d", cap(all))
		}
	})
}

func writeProfile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

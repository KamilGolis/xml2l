package orgschema

import (
	"testing"
)

func TestNew(t *testing.T) {
	o := New()

	if o == nil {
		t.Fatal("New() returned nil")
	}
}

func TestHasExistingName(t *testing.T) {
	o := New()
	o.entries["ApexClass"] = map[string]bool{"MyClass": true}

	if !o.Has("ApexClass", "MyClass") {
		t.Error("expected Has to return true for existing name")
	}
}

func TestHasMissingName(t *testing.T) {
	o := New()
	o.entries["ApexClass"] = map[string]bool{"MyClass": true}

	if o.Has("ApexClass", "OtherClass") {
		t.Error("expected Has to return false for missing name")
	}
}

func TestHasMissingType(t *testing.T) {
	o := New()
	o.entries["ApexClass"] = map[string]bool{"MyClass": true}

	if o.Has("CustomField", "Account.Field__c") {
		t.Error("expected Has to return false for missing type")
	}
}

func TestHasEmptySchema(t *testing.T) {
	o := New()

	if o.Has("Anything", "anything") {
		t.Error("expected Has to return false for empty schema")
	}
}

func TestHasNilReceiver(t *testing.T) {
	var o *OrgSchema

	if o.Has("ApexClass", "MyClass") {
		t.Error("expected Has to return false for nil receiver")
	}
}

func TestHasTypeExistingType(t *testing.T) {
	o := New()
	o.entries["ApexClass"] = map[string]bool{"MyClass": true}

	if !o.HasType("ApexClass") {
		t.Error("expected HasType to return true for existing type")
	}
}

func TestHasTypeMissingType(t *testing.T) {
	o := New()

	if o.HasType("CustomApplication") {
		t.Error("expected HasType to return false for missing type")
	}
}

func TestHasStandardField(t *testing.T) {
	o := New()
	// CustomField was queried (empty map), so type is known.
	// Standard fields (no __c suffix) should always survive.
	o.entries["CustomField"] = map[string]bool{}

	if !o.Has("CustomField", "Account.AccountNumber") {
		t.Error("expected Has to return true for standard field (no __c suffix)")
	}
}

func TestHasCustomFieldInSchema(t *testing.T) {
	o := New()
	o.entries["CustomField"] = map[string]bool{"Account.MyField__c": true}

	if !o.Has("CustomField", "Account.MyField__c") {
		t.Error("expected Has to return true for existing custom field")
	}
}

func TestHasCustomFieldNotInSchema(t *testing.T) {
	o := New()
	o.entries["CustomField"] = map[string]bool{"Account.Other__c": true}

	if o.Has("CustomField", "Account.Missing__c") {
		t.Error("expected Has to return false for missing custom field")
	}
}

// Package orgschema provides tools to query Salesforce org metadata schema via
// the `sf org list metadata` CLI command and check metadata existence.
package orgschema

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// OrgSchema holds a map of metadata type (xmlName) -> set of fullNames
// that exist in the Salesforce org.
type OrgSchema struct {
	entries map[string]map[string]bool
}

// metadataTypesResult maps to the output of `sf org list metadata-types --json`.
type metadataTypesResult struct {
	Status int `json:"status"`
	Result struct {
		MetadataObjects []struct {
			XMLName string `json:"xmlName"`
		} `json:"metadataObjects"`
	} `json:"result"`
}

// metadataListResult maps to the output of `sf org list metadata -m <type> --json`.
type metadataListResult struct {
	Status int `json:"status"`
	Result []struct {
		FullName string `json:"fullName"`
		Type     string `json:"type"`
	} `json:"result"`
}

// sfErrorResult maps to an error response from `sf org` commands.
type sfErrorResult struct {
	Status   int    `json:"status"`
	Name     string `json:"name"`
	Message  string `json:"message"`
	ExitCode int    `json:"exitCode"`
}

// priorityTypes are the metadata types we query during org schema fetch.
var priorityTypes = []string{
	"CustomApplication",
	"ApexClass",
	"CustomField",
	"CustomObject",
	"ApexPage",
	"RecordType",
	"Layout",
	"CustomPermission",
	"Flow",
}

// Has returns true if the given fullName exists in the schema for the given
// metadata type (xmlName). Returns false if the type is unknown or the name
// is not present.
func (o *OrgSchema) Has(xmlName, fullName string) bool {
	if o == nil {
		return false
	}

	m, ok := o.entries[xmlName]
	if !ok {
		return false
	}

	// CustomField metadata API only returns custom fields (ending in __c).
	// Standard fields are not listed but always exist in the org.
	if xmlName == "CustomField" && !strings.HasSuffix(fullName, "__c") {
		return true
	}

	return m[fullName]
}

// HasType returns true if the given metadata type was queried and has entries
// in this schema (even if the set is empty). Returns false if the type was
// never queried.
func (o *OrgSchema) HasType(xmlName string) bool {
	if o == nil {
		return false
	}

	_, ok := o.entries[xmlName]

	return ok
}

// New creates an empty OrgSchema.
func New() *OrgSchema {
	return &OrgSchema{entries: make(map[string]map[string]bool)}
}

// Fetch queries the Salesforce org for available metadata types and their
// members, building an in-memory OrgSchema. When orgName is non-empty, the
// -o flag is appended to every sf command to target the specified org.
// Priority types are always queried; if a type is not supported by the org,
// it is treated as empty (SKIP logged to stderr). Only the initial
// metadata-types query is treated as a hard failure.
func Fetch(orgName string) (*OrgSchema, error) {
	// Step 1: Get available metadata types.
	if _, err := fetchMetadataTypes(orgName); err != nil {
		return nil, err
	}

	// Step 2: Fetch members for each priority type. Child types like
	// CustomField and RecordType are not listed in metadata-types but
	// are still queryable — always attempt the query.
	o := New()
	for _, mt := range priorityTypes {
		fmt.Fprintf(os.Stderr, "Getting %s... ", mt)

		names, err := fetchMetadataList(mt, orgName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "SKIP (%v)\n", err)
			o.entries[mt] = make(map[string]bool)
			continue
		}

		m := make(map[string]bool, len(names))

		for _, n := range names {
			m[n] = true
		}

		o.entries[mt] = m
		fmt.Fprintf(os.Stderr, "OK\n")

		// Custom metadata types are custom objects ending in __mdt.
		// Profile customMetadataTypeAccesses references these type names,
		// not individual records. Extract them from CustomObject results.
		if mt == "CustomObject" {
			cmdtNames := make(map[string]bool)

			for name := range m {
				if strings.HasSuffix(name, "__mdt") {
					cmdtNames[name] = true
				}
			}

			o.entries["CustomMetadata"] = cmdtNames
		}
	}

	return o, nil
}

// fetchMetadataTypes returns the list of xmlName values available in the org.
func fetchMetadataTypes(orgName string) ([]string, error) {
	fmt.Fprintf(os.Stderr, "Getting Org Metadata Types... ")

	args := []string{"org", "list", "metadata-types", "--json"}
	if orgName != "" {
		args = append(args, "-o", orgName)
	}

	output, err := exec.Command("sf", args...).Output()
	if err != nil {
		// Try to parse error JSON.
		if exitErr, ok := err.(*exec.ExitError); ok {
			if msg := parseSFError(exitErr.Stderr); msg != "" {
				fmt.Fprintf(os.Stderr, "ERROR\n")
				return nil, fmt.Errorf("sf org list metadata-types: %s", msg)
			}
		}

		fmt.Fprintf(os.Stderr, "ERROR\n")

		return nil, fmt.Errorf("sf org list metadata-types: %w", err)
	}

	var result metadataTypesResult

	if err := json.Unmarshal(output, &result); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR\n")
		return nil, fmt.Errorf("parse metadata-types response: %w", err)
	}

	if result.Status != 0 {
		fmt.Fprintf(os.Stderr, "ERROR\n")
		return nil, fmt.Errorf("sf org list metadata-types returned status %d", result.Status)
	}

	var types []string

	for _, mo := range result.Result.MetadataObjects {
		types = append(types, mo.XMLName)
	}

	fmt.Fprintf(os.Stderr, "OK\n")
	return types, nil
}

// fetchMetadataList returns the list of fullName values for the given metadata
// type from the org.
func fetchMetadataList(metaType, orgName string) ([]string, error) {
	args := []string{"org", "list", "metadata", "-m", metaType, "--json"}

	if orgName != "" {
		args = append(args, "-o", orgName)
	}

	output, err := exec.Command("sf", args...).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if msg := parseSFError(exitErr.Stderr); msg != "" {
				return nil, fmt.Errorf("%s", msg)
			}
		}

		return nil, fmt.Errorf("%w", err)
	}

	var result metadataListResult

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parse %s response: %w", metaType, err)
	}

	if result.Status != 0 {
		return nil, fmt.Errorf("sf org list metadata -m %s returned status %d", metaType, result.Status)
	}

	var names []string

	for _, item := range result.Result {
		names = append(names, item.FullName)
	}

	return names, nil
}

// parseSFError attempts to parse an error message from sf CLI stderr output.
func parseSFError(stderr []byte) string {
	var errResult sfErrorResult

	if err := json.Unmarshal(stderr, &errResult); err == nil && errResult.Message != "" {
		return errResult.Message
	}

	// Fallback: return the raw stderr trimmed.
	msg := strings.TrimSpace(string(stderr))
	if msg != "" {
		return msg
	}

	return "unknown error"
}

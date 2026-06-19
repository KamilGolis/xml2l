package graph

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONExporter(t *testing.T) {
	g := newTestGraph()

	var buf bytes.Buffer
	err := JSONExporter{}.Export(g, &buf)
	if err != nil {
		t.Fatalf("JSONExporter.Export failed: %v", err)
	}

	var result struct {
		Nodes []CytoscapeNode `json:"nodes"`
		Edges []CytoscapeEdge `json:"edges"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("JSON output not valid JSON: %v\n%s", err, buf.String())
	}

	if len(result.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(result.Edges))
	}

	// Verify all nodes present
	nodeIDs := make(map[string]bool)
	for _, n := range result.Nodes {
		nodeIDs[n.Data.ID] = true
	}
	if !nodeIDs["Profile:Admin"] {
		t.Error("missing Profile:Admin node")
	}
	if !nodeIDs["ApexClass:MyClass"] {
		t.Error("missing ApexClass:MyClass node")
	}
	if !nodeIDs["Field:Account.Name"] {
		t.Error("missing Field:Account.Name node")
	}

	// Verify edges have properties preserved
	for _, e := range result.Edges {
		switch e.Data.Source {
		case "Profile:Admin":
			if e.Data.Target == "ApexClass:MyClass" {
				if e.Data.Enabled == nil || !*e.Data.Enabled {
					t.Error("expected enabled=true on class edge")
				}
			} else if e.Data.Target == "Field:Account.Name" {
				if e.Data.Readable == nil || !*e.Data.Readable {
					t.Error("expected readable=true on field edge")
				}
				if e.Data.Editable == nil || *e.Data.Editable {
					t.Error("expected editable=false on field edge")
				}
			}
		}
	}
}

func TestDOTExporter(t *testing.T) {
	g := newTestGraph()

	var buf bytes.Buffer
	err := DOTExporter{}.Export(g, &buf)
	if err != nil {
		t.Fatalf("DOTExporter.Export failed: %v", err)
	}

	output := buf.String()

	// Must start with digraph
	if !strings.HasPrefix(output, "digraph ProfileGraph {") {
		t.Errorf("DOT output should start with 'digraph ProfileGraph {', got:\n%s", output[:50])
	}

	// Must end with }
	output = strings.TrimSpace(output)
	if !strings.HasSuffix(output, "}") {
		t.Error("DOT output should end with }")
	}

	// Must contain node statements
	if !strings.Contains(output, `"Profile:Admin"`) {
		t.Error("DOT output missing Profile:Admin node")
	}
	if !strings.Contains(output, `"ApexClass:MyClass"`) {
		t.Error("DOT output missing ApexClass:MyClass node")
	}

	// Must contain edge with property label
	if !strings.Contains(output, "->") {
		t.Error("DOT output missing edges")
	}
	if !strings.Contains(output, "enabled: true") {
		t.Error("DOT output missing edge property label")
	}

	// Quoting: IDs with colons or spaces should be quoted
	if !strings.Contains(output, `"Profile:Admin"`) {
		t.Error("Profile:Admin ID should be quoted (contains colon)")
	}
}

func TestJSONExporterEmptyGraph(t *testing.T) {
	g := NewGraph()

	var buf bytes.Buffer
	err := JSONExporter{}.Export(g, &buf)
	if err != nil {
		t.Fatalf("JSONExporter.Export on empty graph failed: %v", err)
	}

	var result struct {
		Nodes []CytoscapeNode `json:"nodes"`
		Edges []CytoscapeEdge `json:"edges"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}

	if len(result.Nodes) != 0 {
		t.Errorf("expected 0 nodes for empty graph, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 0 {
		t.Errorf("expected 0 edges for empty graph, got %d", len(result.Edges))
	}
}

func TestDOTExporterEmptyGraph(t *testing.T) {
	g := NewGraph()

	var buf bytes.Buffer
	err := DOTExporter{}.Export(g, &buf)
	if err != nil {
		t.Fatalf("DOTExporter.Export on empty graph failed: %v", err)
	}

	output := strings.TrimSpace(buf.String())
	if output != "digraph ProfileGraph {}" && output != "digraph ProfileGraph {\n}" {
		t.Errorf("unexpected DOT output for empty graph: %s", output)
	}
}

// newTestGraph creates a graph with 1 profile, 2 metadata nodes, and 2 edges
// with known properties for testing exporters.
func newTestGraph() *SalesforceGraph {
	g := NewGraph()
	p := g.AddProfile("Admin", "Admin.profile-meta.xml")

	cls := g.GetOrCreateMetadataNode(MetaTypeApexClass, "MyClass")
	enabled := true
	g.AddEdge(p, cls, EdgeProperties{Enabled: &enabled})

	fld := g.GetOrCreateMetadataNode(MetaTypeField, "Account.Name")
	readable := true
	editable := false
	g.AddEdge(p, fld, EdgeProperties{Readable: &readable, Editable: &editable})

	return g
}

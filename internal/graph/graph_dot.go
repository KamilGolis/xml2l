package graph

import (
	"fmt"
	"io"
	"strings"
)

// DOTExporter serializes the graph as a Graphviz DOT directed graph.
type DOTExporter struct{}

func (e DOTExporter) Export(g *SalesforceGraph, w io.Writer) error {
	if _, err := fmt.Fprintln(w, "digraph ProfileGraph {"); err != nil {
		return err
	}

	// Profile nodes
	for _, p := range g.ProfileNodes {
		id := dotID("Profile:" + p.Name)
		if _, err := fmt.Fprintf(w, "  %s [label=%s];\n", id, dotID(p.Name)); err != nil {
			return err
		}
	}

	// Metadata nodes
	for _, m := range g.MetadataNodes {
		id := dotID(string(m.MetaType) + ":" + m.Name)
		if _, err := fmt.Fprintf(w, "  %s [label=%s];\n", id, dotID(m.Name)); err != nil {
			return err
		}
	}

	// Edges
	for _, e := range g.Edges {
		src := dotID("Profile:" + e.ProfileNode.Name)
		tgt := dotID(string(e.MetadataNode.MetaType) + ":" + e.MetadataNode.Name)
		label := edgePropertiesLabel(e.Properties)
		if label != "" {
			if _, err := fmt.Fprintf(w, "  %s -> %s [label=%s];\n", src, tgt, dotID(label)); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(w, "  %s -> %s;\n", src, tgt); err != nil {
				return err
			}
		}
	}

	if _, err := fmt.Fprintln(w, "}"); err != nil {
		return err
	}
	return nil
}

// dotID wraps a string in double quotes for safe DOT identifier usage.
func dotID(s string) string {
	if strings.ContainsAny(s, " :.-") {
		return `"` + s + `"`
	}
	return s
}

// edgePropertiesLabel builds a comma-separated "key: value" label string
// from all non-nil EdgeProperties fields.
func edgePropertiesLabel(props EdgeProperties) string {
	var parts []string
	if props.Enabled != nil {
		parts = append(parts, fmt.Sprintf("enabled: %t", *props.Enabled))
	}
	if props.Readable != nil {
		parts = append(parts, fmt.Sprintf("readable: %t", *props.Readable))
	}
	if props.Editable != nil {
		parts = append(parts, fmt.Sprintf("editable: %t", *props.Editable))
	}
	if props.AllowCreate != nil {
		parts = append(parts, fmt.Sprintf("allowCreate: %t", *props.AllowCreate))
	}
	if props.AllowDelete != nil {
		parts = append(parts, fmt.Sprintf("allowDelete: %t", *props.AllowDelete))
	}
	if props.AllowRead != nil {
		parts = append(parts, fmt.Sprintf("allowRead: %t", *props.AllowRead))
	}
	if props.AllowEdit != nil {
		parts = append(parts, fmt.Sprintf("allowEdit: %t", *props.AllowEdit))
	}
	if props.ModifyAll != nil {
		parts = append(parts, fmt.Sprintf("modifyAllRecords: %t", *props.ModifyAll))
	}
	if props.ViewAll != nil {
		parts = append(parts, fmt.Sprintf("viewAllRecords: %t", *props.ViewAll))
	}
	if props.Default != nil {
		parts = append(parts, fmt.Sprintf("default: %t", *props.Default))
	}
	if props.Visible != nil {
		parts = append(parts, fmt.Sprintf("visible: %t", *props.Visible))
	}
	if props.Hidden != nil {
		parts = append(parts, fmt.Sprintf("hidden: %t", *props.Hidden))
	}
	if props.Visibility != nil {
		parts = append(parts, fmt.Sprintf("visibility: %s", *props.Visibility))
	}
	if props.RecordType != nil {
		parts = append(parts, fmt.Sprintf("recordType: %s", *props.RecordType))
	}
	return strings.Join(parts, ", ")
}

package graph

import (
	"encoding/json"
	"fmt"
	"io"
)

// GraphExporter is the interface for serializing a SalesforceGraph to a writer.
type GraphExporter interface {
	Export(g *SalesforceGraph, w io.Writer) error
}

// JSONExporter serializes the graph as indented JSON with all nodes and edges.
type JSONExporter struct{}

func (e JSONExporter) Export(g *SalesforceGraph, w io.Writer) error {
	nodes := make([]CytoscapeNode, 0, len(g.ProfileNodes)+len(g.MetadataNodes))

	for _, p := range g.ProfileNodes {
		nodes = append(nodes, CytoscapeNode{
			Data: CytoscapeNodeData{
				ID:       "Profile:" + p.Name,
				Label:    p.Name,
				MetaType: "Profile",
			},
		})
	}

	for _, m := range g.MetadataNodes {
		nodes = append(nodes, CytoscapeNode{
			Data: CytoscapeNodeData{
				ID:       string(m.MetaType) + ":" + m.Name,
				Label:    m.Name,
				MetaType: string(m.MetaType),
			},
		})
	}

	edges := make([]CytoscapeEdge, 0, len(g.Edges))

	for i, e := range g.Edges {
		edges = append(edges, newCytoscapeEdge(fmt.Sprintf("e%d", i), e))
	}

	out := struct {
		Nodes []CytoscapeNode `json:"nodes"`
		Edges []CytoscapeEdge `json:"edges"`
	}{Nodes: nodes, Edges: edges}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(out)
}

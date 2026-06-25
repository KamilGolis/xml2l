package graph

import (
	"fmt"
	"sort"
	"strconv"
)

// CytoscapeGraph is the top-level structure for serializing the graph to
// the Cytoscape.js elements format.
type CytoscapeGraph struct {
	Nodes []CytoscapeNode `json:"nodes"`
	Edges []CytoscapeEdge `json:"edges"`
}

// CytoscapeNode represents a single node in the Cytoscape.js graph.
type CytoscapeNode struct {
	Data CytoscapeNodeData `json:"data"`
}

// CytoscapeNodeData holds the identifying fields for a node.
type CytoscapeNodeData struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	MetaType string `json:"metaType"`
}

// CytoscapeEdge represents a single edge in the Cytoscape.js graph.
type CytoscapeEdge struct {
	Data CytoscapeEdgeData `json:"data"`
}

// CytoscapeEdgeData holds the source, target, and permission properties for an edge.
type CytoscapeEdgeData struct {
	ID          string  `json:"id"`
	Source      string  `json:"source"`
	Target      string  `json:"target"`
	MetaType    string  `json:"metaType,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
	Readable    *bool   `json:"readable,omitempty"`
	Editable    *bool   `json:"editable,omitempty"`
	AllowCreate *bool   `json:"allowCreate,omitempty"`
	AllowDelete *bool   `json:"allowDelete,omitempty"`
	AllowRead   *bool   `json:"allowRead,omitempty"`
	AllowEdit   *bool   `json:"allowEdit,omitempty"`
	ModifyAll   *bool   `json:"modifyAll,omitempty"`
	ViewAll     *bool   `json:"viewAll,omitempty"`
	Default     *bool   `json:"default,omitempty"`
	Visible     *bool   `json:"visible,omitempty"`
	Hidden      *bool   `json:"hidden,omitempty"`
	Visibility  *string `json:"visibility,omitempty"`
	RecordType  *string `json:"recordType,omitempty"`
}

// ToCytoscapeJSON serializes the graph to the Cytoscape.js elements format.
// It caps metadata nodes at maxPerType per MetaType, aggregating excess nodes
// into summary entries. All permission properties are serialized on edges.
func (g *SalesforceGraph) ToCytoscapeJSON(maxPerType int) *CytoscapeGraph {
	cg := &CytoscapeGraph{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}

	// Profile nodes — always show all, with "Profile:" prefix
	for _, p := range g.ProfileNodes {
		cg.Nodes = append(cg.Nodes, CytoscapeNode{
			Data: CytoscapeNodeData{
				ID:       "Profile:" + p.Name,
				Label:    p.Name,
				MetaType: "Profile",
			},
		})
	}

	// Group metadata nodes by MetaType and count edges per node
	typeMetaNodes := make(map[MetadataType][]*MetadataNode)
	edgeCounts := make(map[*MetadataNode]int)

	for _, e := range g.Edges {
		edgeCounts[e.MetadataNode]++
	}

	for _, m := range g.MetadataNodes {
		typeMetaNodes[m.MetaType] = append(typeMetaNodes[m.MetaType], m)
	}

	// Sort each type's nodes by edge count descending, cap at maxPerType
	for metaType, nodes := range typeMetaNodes {
		sort.Slice(nodes, func(i, j int) bool {
			return edgeCounts[nodes[i]] > edgeCounts[nodes[j]]
		})

		limit := maxPerType
		if limit <= 0 || len(nodes) < limit {
			limit = len(nodes)
		}

		for i := 0; i < limit; i++ {
			m := nodes[i]

			cg.Nodes = append(cg.Nodes, CytoscapeNode{
				Data: CytoscapeNodeData{
					ID:       string(metaType) + ":" + m.Name,
					Label:    m.Name,
					MetaType: string(metaType),
				},
			})
		}

		// Aggregate excess nodes into a summary entry
		excess := len(nodes) - limit

		if excess > 0 {
			summaryName := string(metaType) + " +" + strconv.Itoa(excess) + " more"

			cg.Nodes = append(cg.Nodes, CytoscapeNode{
				Data: CytoscapeNodeData{
					ID:       summaryName,
					Label:    "[" + string(metaType) + "] +" + strconv.Itoa(excess) + " more",
					MetaType: string(metaType),
				},
			})
		}
	}

	// Build a set of included node IDs for edge filtering
	included := make(map[string]bool)

	for _, n := range cg.Nodes {
		included[n.Data.ID] = true
	}

	// Edges — only include edges whose source and target are both in the node set
	edgeIndex := 0

	for _, e := range g.Edges {
		srcID := "Profile:" + e.ProfileNode.Name
		tgtID := string(e.MetadataNode.MetaType) + ":" + e.MetadataNode.Name

		if !included[srcID] || !included[tgtID] {
			continue
		}

		edge := CytoscapeEdge{
			Data: CytoscapeEdgeData{
				ID:          fmt.Sprintf("e%d", edgeIndex),
				Source:      srcID,
				Target:      tgtID,
				MetaType:    string(e.MetadataNode.MetaType),
				Enabled:     e.Properties.Enabled,
				Readable:    e.Properties.Readable,
				Editable:    e.Properties.Editable,
				AllowCreate: e.Properties.AllowCreate,
				AllowDelete: e.Properties.AllowDelete,
				AllowRead:   e.Properties.AllowRead,
				AllowEdit:   e.Properties.AllowEdit,
				ModifyAll:   e.Properties.ModifyAll,
				ViewAll:     e.Properties.ViewAll,
				Default:     e.Properties.Default,
				Visible:     e.Properties.Visible,
				Hidden:      e.Properties.Hidden,
				Visibility:  e.Properties.Visibility,
				RecordType:  e.Properties.RecordType,
			},
		}

		cg.Edges = append(cg.Edges, edge)
		edgeIndex++
	}

	return cg
}

package converter

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"xml2l/internal/graph"
)

// FromTypeXML and FromTypeYAML are the accepted conversion directions.
const (
	FromTypeXML  = "xml"
	FromTypeYAML = "yaml"
)

// metadataNamespace is the XML namespace every profile and permission set uses.
const metadataNamespace = "http://soap.sforce.com/2006/04/metadata"

// ParseFromType returns the canonical conversion direction, defaulting to XML
// when the flag is omitted.
func ParseFromType(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", FromTypeXML:
		return FromTypeXML, nil
	case FromTypeYAML:
		return FromTypeYAML, nil
	default:
		return "", fmt.Errorf("unsupported --from-type %q (supported: xml, yaml)", value)
	}
}

// xmlNameRe matches the element names Salesforce metadata uses.
var xmlNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

// YAMLToXML converts the YAML this package emits back to metadata XML.
//
// A mapping key becomes an element, a sequence under a key becomes repeated
// elements, and a scalar becomes the element's text. The YAML carries no XML
// attributes, comments or formatting, so those cannot come back; everything
// else round-trips.
func YAMLToXML(data []byte, rootName string) ([]byte, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))

	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}

	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("multiple YAML documents are not supported")
	}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, fmt.Errorf("expected a single top-level mapping")
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode || len(root.Content) != 2 {
		return nil, fmt.Errorf("expected a single top-level key, got %s", describeNode(root))
	}

	name := root.Content[0].Value
	if name != rootName {
		return nil, fmt.Errorf("expected a %s document, got %s", rootName, name)
	}

	value := resolveAlias(root.Content[1])
	if value.Kind == yaml.SequenceNode {
		return nil, fmt.Errorf("%s must be a mapping, got a sequence", rootName)
	}

	var buf bytes.Buffer

	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&buf, `<%s xmlns="%s">`+"\n", name, metadataNamespace)

	if value.Kind == yaml.MappingNode {
		if err := writeMapping(&buf, pairsOf(value), 4); err != nil {
			return nil, err
		}
	}

	fmt.Fprintf(&buf, "</%s>\n", name)

	return buf.Bytes(), nil
}

// yamlPair is one key/value pair of a mapping, in the order it appears.
type yamlPair struct {
	name  string
	value *yaml.Node
}

// pairsOf returns the pairs of a mapping node with aliases followed.
func pairsOf(node *yaml.Node) []yamlPair {
	pairs := make([]yamlPair, 0, len(node.Content)/2)

	for i := 0; i+1 < len(node.Content); i += 2 {
		pairs = append(pairs, yamlPair{
			name:  node.Content[i].Value,
			value: resolveAlias(node.Content[i+1]),
		})
	}

	return pairs
}

// writeMapping writes the pairs as child elements, in the given order.
func writeMapping(buf *bytes.Buffer, pairs []yamlPair, indent int) error {
	for _, pair := range pairs {
		if !xmlNameRe.MatchString(pair.name) {
			return fmt.Errorf("invalid XML element name %q", pair.name)
		}

		if err := writeElement(buf, pair.name, pair.value, indent); err != nil {
			return err
		}
	}

	return nil
}

// writeElement writes one element, repeating it for a sequence and recursing
// for a mapping.
func writeElement(buf *bytes.Buffer, name string, node *yaml.Node, indent int) error {
	pfx := strings.Repeat(" ", indent)

	switch node.Kind {
	case yaml.SequenceNode:
		for _, item := range node.Content {
			item = resolveAlias(item)
			if item.Kind == yaml.SequenceNode {
				return fmt.Errorf("nested sequences have no XML equivalent (element <%s>)", name)
			}

			if err := writeElement(buf, name, item, indent); err != nil {
				return err
			}
		}

		return nil

	case yaml.MappingNode:
		pairs := pairsOf(node)

		// An entry's children are written in Salesforce's order (alphabetical),
		// not the order the YAML happens to show: the YAML puts the identifying
		// child first for readability.
		if graph.SectionNameField(name) != "" {
			sort.Slice(pairs, func(i, j int) bool { return pairs[i].name < pairs[j].name })
		}

		fmt.Fprintf(buf, "%s<%s>\n", pfx, name)

		if err := writeMapping(buf, pairs, indent+4); err != nil {
			return err
		}

		fmt.Fprintf(buf, "%s</%s>\n", pfx, name)
		return nil

	case yaml.ScalarNode:
		fmt.Fprintf(buf, "%s<%s>%s</%s>\n", pfx, name, xmlEscapeText(node.Value), name)
		return nil

	default:
		return fmt.Errorf("unsupported YAML value for <%s>: %s", name, describeNode(node))
	}
}

// resolveAlias follows a YAML alias to the node it names.
func resolveAlias(node *yaml.Node) *yaml.Node {
	for node.Kind == yaml.AliasNode && node.Alias != nil {
		node = node.Alias
	}

	return node
}

// xmlEscapeText escapes the characters that are special in XML text.
func xmlEscapeText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// describeNode names a YAML node kind for error messages.
func describeNode(node *yaml.Node) string {
	switch node.Kind {
	case yaml.DocumentNode:
		return "document"
	case yaml.MappingNode:
		return "mapping"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	default:
		return "unknown node"
	}
}

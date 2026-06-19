// Package profile parses a single Salesforce .profile-meta.xml file using
// streaming encoding/xml.NewDecoder and produces a structured Profile with
// deduplicated, ghost-filtered metadata entries. Phase 1 of the 3-phase
// profile normalization pipeline.
package profile

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"xml2l/internal/graph"
	"xml2l/internal/scanner"
)

// Profile holds the parsed contents of a single .profile-meta.xml file.
type Profile struct {
	Name           string
	SourcePath     string
	RawXML         string
	UserLicense    string
	Description    string
	IsCustom       bool
	HasCustomField bool
	Sections       map[string][]MetadataEntry
}

// MetadataEntry represents a single permission entry within a profile section.
type MetadataEntry struct {
	Name        string
	Enabled     *bool
	Readable    *bool
	Editable    *bool
	Visible     *bool
	Hidden      *bool
	Default     *bool
	AllowCreate *bool
	AllowDelete *bool
	AllowRead   *bool
	AllowEdit   *bool
	ModifyAll   *bool
	ViewAll     *bool
	Visibility  *string
	RecordType  *string
}

// ToEdgeProperties converts this metadata entry to graph edge properties.
func (e MetadataEntry) ToEdgeProperties() graph.EdgeProperties {
	return graph.EdgeProperties{
		Enabled:     e.Enabled,
		Readable:    e.Readable,
		Editable:    e.Editable,
		Visible:     e.Visible,
		Hidden:      e.Hidden,
		Default:     e.Default,
		AllowCreate: e.AllowCreate,
		AllowDelete: e.AllowDelete,
		AllowRead:   e.AllowRead,
		AllowEdit:   e.AllowEdit,
		ModifyAll:   e.ModifyAll,
		ViewAll:     e.ViewAll,
		Visibility:  e.Visibility,
		RecordType:  e.RecordType,
	}
}

func BoolPtr(b bool) *bool    { return &b }
func StrPtr(s string) *string { return &s }

// sectionDecoder decodes ONE entry element of a known section type.
// se is the entry's StartElement (e.g. <classAccesses>).
// The decoder is positioned right after the StartElement.
type sectionDecoder func(*xml.Decoder, *xml.StartElement, scanner.GroundTruth, *[]MetadataEntry) error

var sectionFuncs = map[string]sectionDecoder{}

func init() {
	sectionFuncs["classAccesses"] = decodeClassAccesses
	sectionFuncs["agentAccesses"] = decodeAgentAccesses
	sectionFuncs["applicationVisibilities"] = decodeAppVisibilities
	sectionFuncs["categoryGroupVisibilities"] = decodeCategoryGroupVisibilities
	sectionFuncs["customMetadataTypeAccesses"] = decodeNameEnabled
	sectionFuncs["customPermissions"] = decodeNameEnabled
	sectionFuncs["customSettingAccesses"] = decodeNameEnabled
	sectionFuncs["externalDataSourceAccesses"] = decodeExtDataSource
	sectionFuncs["fieldLevelSecurities"] = decodeFieldLevelSecurities
	sectionFuncs["fieldPermissions"] = decodeFieldPermissions
	sectionFuncs["flowAccesses"] = decodeFlowAccesses
	sectionFuncs["genComputingSummaryDefAccess"] = decodeGenComputingDef
	sectionFuncs["layoutAssignments"] = decodeLayoutAssignments
	sectionFuncs["loginFlows"] = decodeLoginFlows
	sectionFuncs["loginHours"] = decodeRawEntry
	sectionFuncs["loginIpRanges"] = decodeRawEntry
	sectionFuncs["objectPermissions"] = decodeObjectPermissions
	sectionFuncs["profileActionOverrides"] = decodeRawEntry
	sectionFuncs["recordTypeVisibilities"] = decodeRecordTypeVisibilities
	sectionFuncs["servicePresenceStatusAccesses"] = decodeServicePresence
	sectionFuncs["tabVisibilities"] = decodeTabVisibilities
	sectionFuncs["userPermissions"] = decodeUserPermissions
}

const profileNS = "http://soap.sforce.com/2006/04/metadata"

// Decode parses a single .profile-meta.xml file from reader r, using gt to
// filter out ghost references. Returns a populated Profile or an error.
func Decode(r io.Reader, gt scanner.GroundTruth) (*Profile, error) {
	prof := &Profile{
		Sections: make(map[string][]MetadataEntry),
	}

	decoder := xml.NewDecoder(r)

	// Skip leading non-element tokens (ProcInst, Comment, Directive, CharData).
	var rootTok xml.Token
	for {
		var err error
		rootTok, err = decoder.Token()
		if err == io.EOF {
			return nil, fmt.Errorf("empty or non-XML input")
		}
		if err != nil {
			return nil, fmt.Errorf("read root: %w", err)
		}
		switch rootTok.(type) {
		case xml.ProcInst, xml.Comment, xml.Directive, xml.CharData:
			continue
		}
		break
	}

	startElem, ok := rootTok.(xml.StartElement)
	if !ok {
		return nil, fmt.Errorf("expected root StartElement, got %T", rootTok)
	}
	if startElem.Name.Local != "Profile" {
		return nil, fmt.Errorf("expected root <Profile>, got <%s>", startElem.Name.Local)
	}

	// Iterate through top-level children of <Profile>.
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			tag := t.Name.Local
			if t.Name.Space != "" && t.Name.Space != profileNS {
				if err := skipElement(decoder); err != nil {
					return nil, err
				}
				continue
			}

			switch tag {
			case "userLicense":
				if err := decoder.DecodeElement(&prof.UserLicense, &t); err != nil {
					return nil, fmt.Errorf("userLicense: %w", err)
				}
			case "custom":
				var val string
				if err := decoder.DecodeElement(&val, &t); err != nil {
					return nil, fmt.Errorf("custom: %w", err)
				}
				prof.IsCustom = strings.TrimSpace(val) == "true"
				prof.HasCustomField = true
			case "description":
				if err := decoder.DecodeElement(&prof.Description, &t); err != nil {
					return nil, fmt.Errorf("description: %w", err)
				}
			default:
				if fn, ok := sectionFuncs[tag]; ok {
					var entries []MetadataEntry
					if err := fn(decoder, &t, gt, &entries); err != nil {
						return nil, fmt.Errorf("%s: %w", tag, err)
					}
					if len(entries) > 0 {
						prof.Sections[tag] = append(prof.Sections[tag], entries...)
					}
				} else {
					if err := skipElement(decoder); err != nil {
						return nil, err
					}
				}
			}

		case xml.EndElement:
			if t.Name.Local == "Profile" {
				return prof, nil
			}

		case xml.CharData:
		}
	}

	return prof, nil
}

// skipElement skips over a complete XML element (including all children).
func skipElement(decoder *xml.Decoder) error {
	depth := 1
	for {
		tok, err := decoder.Token()
		if err != nil {
			return err
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
			if depth == 0 {
				return nil
			}
		}
	}
}

// decodeRawEntry consumes one element without producing edges.
func decodeRawEntry(decoder *xml.Decoder, se *xml.StartElement, _ scanner.GroundTruth, _ *[]MetadataEntry) error {
	// Decode into a discard target to advance the decoder past the element.
	var discard interface{}
	return decoder.DecodeElement(&discard, se)
}

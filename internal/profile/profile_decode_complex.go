package profile

import (
	"encoding/xml"

	"xml2l/internal/scanner"
)

// Entry structs for complex decoders (multi-property types).

type appVisibilityEntry struct {
	Application string `xml:"application"`
	Default     *bool  `xml:"default"`
	Visible     *bool  `xml:"visible"`
}

type categoryGroupEntry struct {
	DataCategoryGroup string `xml:"dataCategoryGroup"`
	Visibility        string `xml:"visibility"`
}

type layoutEntry struct {
	Layout     string `xml:"layout"`
	RecordType string `xml:"recordType"`
}

type loginFlowEntry struct {
	FlowType    string `xml:"flowType"`
	UserLicense string `xml:"userLicense"`
	Flow        string `xml:"flow"`
}

type objectPermEntry struct {
	Object         string `xml:"object"`
	AllowCreate    *bool  `xml:"allowCreate"`
	AllowDelete    *bool  `xml:"allowDelete"`
	AllowRead      *bool  `xml:"allowRead"`
	AllowEdit      *bool  `xml:"allowEdit"`
	AllowModifyAll *bool  `xml:"allowModifyAll"`
	ViewAll        *bool  `xml:"viewAll"`
}

type recordTypeEntry struct {
	RecordType string `xml:"recordType"`
	Visible    *bool  `xml:"visible"`
	Default    *bool  `xml:"default"`
}

type tabVisibilityEntry struct {
	Tab        string `xml:"tab"`
	Visibility string `xml:"visibility"`
}

// -- Complex section decoders

func decodeAppVisibilities(decoder *xml.Decoder, se *xml.StartElement, gt scanner.GroundTruth, entries *[]MetadataEntry) error {
	var entry appVisibilityEntry
	if err := decoder.DecodeElement(&entry, se); err != nil {
		return err
	}
	if entry.Application == "" {
		return nil
	}
	*entries = append(*entries, MetadataEntry{Name: entry.Application, Visible: entry.Visible, Default: entry.Default})
	return nil
}

func decodeCategoryGroupVisibilities(decoder *xml.Decoder, se *xml.StartElement, gt scanner.GroundTruth, entries *[]MetadataEntry) error {
	var entry categoryGroupEntry
	if err := decoder.DecodeElement(&entry, se); err != nil {
		return err
	}
	if entry.DataCategoryGroup == "" {
		return nil
	}
	*entries = append(*entries, MetadataEntry{Name: entry.DataCategoryGroup, Visibility: &entry.Visibility})
	return nil
}

func decodeTabVisibilities(decoder *xml.Decoder, se *xml.StartElement, gt scanner.GroundTruth, entries *[]MetadataEntry) error {
	var entry tabVisibilityEntry
	if err := decoder.DecodeElement(&entry, se); err != nil {
		return err
	}
	if entry.Tab == "" || entry.Visibility == "" {
		return nil
	}
	// Tab visibility uses string enum values: DefaultOn, DefaultOff, Hidden.
	// Pass through the raw string value.
	*entries = append(*entries, MetadataEntry{Name: entry.Tab, Visibility: &entry.Visibility})
	return nil
}

func decodeLayoutAssignments(decoder *xml.Decoder, se *xml.StartElement, gt scanner.GroundTruth, entries *[]MetadataEntry) error {
	var entry layoutEntry
	if err := decoder.DecodeElement(&entry, se); err != nil {
		return err
	}
	if entry.Layout == "" {
		return nil
	}
	*entries = append(*entries, MetadataEntry{Name: entry.Layout, RecordType: &entry.RecordType})
	return nil
}

func decodeLoginFlows(decoder *xml.Decoder, se *xml.StartElement, gt scanner.GroundTruth, entries *[]MetadataEntry) error {
	var entry loginFlowEntry
	if err := decoder.DecodeElement(&entry, se); err != nil {
		return err
	}
	if entry.Flow == "" {
		return nil
	}
	*entries = append(*entries, MetadataEntry{Name: entry.Flow})
	return nil
}

func decodeObjectPermissions(decoder *xml.Decoder, se *xml.StartElement, gt scanner.GroundTruth, entries *[]MetadataEntry) error {
	var entry objectPermEntry
	if err := decoder.DecodeElement(&entry, se); err != nil {
		return err
	}
	if entry.Object == "" {
		return nil
	}
	*entries = append(*entries, MetadataEntry{
		Name: entry.Object, AllowCreate: entry.AllowCreate, AllowDelete: entry.AllowDelete,
		AllowRead: entry.AllowRead, AllowEdit: entry.AllowEdit,
		ModifyAll: entry.AllowModifyAll, ViewAll: entry.ViewAll,
	})
	return nil
}

func decodeRecordTypeVisibilities(decoder *xml.Decoder, se *xml.StartElement, gt scanner.GroundTruth, entries *[]MetadataEntry) error {
	var entry recordTypeEntry
	if err := decoder.DecodeElement(&entry, se); err != nil {
		return err
	}
	if entry.RecordType == "" {
		return nil
	}
	*entries = append(*entries, MetadataEntry{Name: entry.RecordType, Visible: entry.Visible, Default: entry.Default})
	return nil
}

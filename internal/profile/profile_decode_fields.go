package profile

import (
	"encoding/xml"

	"xml2l/internal/scanner"
)

// Entry structs for field-level decoders.

type fieldPermEntry struct {
	Field    string `xml:"field"`
	Readable *bool  `xml:"readable"`
	Editable *bool  `xml:"editable"`
}

type fieldLevelSecEntry struct {
	Field  string `xml:"field"`
	Hidden *bool  `xml:"hidden"`
}

// -- Field permission decoders

func decodeFieldLevelSecurities(decoder *xml.Decoder, se *xml.StartElement, gt scanner.GroundTruth, entries *[]MetadataEntry) error {
	var entry fieldLevelSecEntry
	if err := decoder.DecodeElement(&entry, se); err != nil {
		return err
	}
	if entry.Field == "" {
		return nil
	}
	if !gt[entry.Field] {
		return nil
	}
	*entries = append(*entries, MetadataEntry{Name: entry.Field, Hidden: entry.Hidden})
	return nil
}

func decodeFieldPermissions(decoder *xml.Decoder, se *xml.StartElement, gt scanner.GroundTruth, entries *[]MetadataEntry) error {
	var entry fieldPermEntry
	if err := decoder.DecodeElement(&entry, se); err != nil {
		return err
	}
	if entry.Field == "" {
		return nil
	}
	if !gt[entry.Field] {
		return nil
	}
	*entries = append(*entries, MetadataEntry{Name: entry.Field, Readable: entry.Readable, Editable: entry.Editable})
	return nil
}

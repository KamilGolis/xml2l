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

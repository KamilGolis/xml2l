package profile

import (
	"encoding/xml"

	"xml2l/internal/scanner"
)

// Entry structs for "name + enabled" pattern decoders.

type classAccessEntry struct {
	ApexClass string `xml:"apexClass"`
	Enabled   bool   `xml:"enabled"`
}

type agentAccessEntry struct {
	Agent   string `xml:"agent"`
	Enabled bool   `xml:"enabled"`
}

type nameEnabledEntry struct {
	Name    string `xml:"name"`
	Enabled bool   `xml:"enabled"`
}

type flowEnabledEntry struct {
	Flow    string `xml:"flow"`
	Enabled bool   `xml:"enabled"`
}

type extDataSourceEntry struct {
	ExternalDataSource string `xml:"externalDataSource"`
	Enabled            bool   `xml:"enabled"`
}

type enhancedSummaryEntry struct {
	EnhancedSummaryDef string `xml:"enhancedSummaryDef"`
	Enabled            bool   `xml:"enabled"`
}

type servicePresenceEntry struct {
	ServicePresenceStatus string `xml:"servicePresenceStatus"`
	Enabled               bool   `xml:"enabled"`
}

type userPermEntry struct {
	Name    string `xml:"name"`
	Enabled bool   `xml:"enabled"`
}

// -- Section decoders for name+enabled types

func decodeClassAccesses(decoder *xml.Decoder, se *xml.StartElement, gt scanner.GroundTruth, entries *[]MetadataEntry) error {
	var entry classAccessEntry
	if err := decoder.DecodeElement(&entry, se); err != nil {
		return err
	}
	if entry.ApexClass == "" {
		return nil
	}
	if !gt[entry.ApexClass] {
		return nil
	}
	*entries = append(*entries, MetadataEntry{Name: entry.ApexClass, Enabled: BoolPtr(entry.Enabled)})
	return nil
}

func decodeAgentAccesses(decoder *xml.Decoder, se *xml.StartElement, gt scanner.GroundTruth, entries *[]MetadataEntry) error {
	var entry agentAccessEntry
	if err := decoder.DecodeElement(&entry, se); err != nil {
		return err
	}
	if entry.Agent == "" {
		return nil
	}
	if !gt[entry.Agent] {
		return nil
	}
	*entries = append(*entries, MetadataEntry{Name: entry.Agent, Enabled: BoolPtr(entry.Enabled)})
	return nil
}

func decodeNameEnabled(decoder *xml.Decoder, se *xml.StartElement, gt scanner.GroundTruth, entries *[]MetadataEntry) error {
	var entry nameEnabledEntry
	if err := decoder.DecodeElement(&entry, se); err != nil {
		return err
	}
	if entry.Name == "" {
		return nil
	}
	*entries = append(*entries, MetadataEntry{Name: entry.Name, Enabled: BoolPtr(entry.Enabled)})
	return nil
}

func decodeFlowAccesses(decoder *xml.Decoder, se *xml.StartElement, gt scanner.GroundTruth, entries *[]MetadataEntry) error {
	var entry flowEnabledEntry
	if err := decoder.DecodeElement(&entry, se); err != nil {
		return err
	}
	if entry.Flow == "" {
		return nil
	}
	*entries = append(*entries, MetadataEntry{Name: entry.Flow, Enabled: BoolPtr(entry.Enabled)})
	return nil
}

func decodeExtDataSource(decoder *xml.Decoder, se *xml.StartElement, gt scanner.GroundTruth, entries *[]MetadataEntry) error {
	var entry extDataSourceEntry
	if err := decoder.DecodeElement(&entry, se); err != nil {
		return err
	}
	if entry.ExternalDataSource == "" {
		return nil
	}
	*entries = append(*entries, MetadataEntry{Name: entry.ExternalDataSource, Enabled: BoolPtr(entry.Enabled)})
	return nil
}

func decodeGenComputingDef(decoder *xml.Decoder, se *xml.StartElement, gt scanner.GroundTruth, entries *[]MetadataEntry) error {
	var entry enhancedSummaryEntry
	if err := decoder.DecodeElement(&entry, se); err != nil {
		return err
	}
	if entry.EnhancedSummaryDef == "" {
		return nil
	}
	*entries = append(*entries, MetadataEntry{Name: entry.EnhancedSummaryDef, Enabled: BoolPtr(entry.Enabled)})
	return nil
}

func decodeServicePresence(decoder *xml.Decoder, se *xml.StartElement, gt scanner.GroundTruth, entries *[]MetadataEntry) error {
	var entry servicePresenceEntry
	if err := decoder.DecodeElement(&entry, se); err != nil {
		return err
	}
	if entry.ServicePresenceStatus == "" {
		return nil
	}
	*entries = append(*entries, MetadataEntry{Name: entry.ServicePresenceStatus, Enabled: BoolPtr(entry.Enabled)})
	return nil
}

func decodeUserPermissions(decoder *xml.Decoder, se *xml.StartElement, gt scanner.GroundTruth, entries *[]MetadataEntry) error {
	var entry userPermEntry
	if err := decoder.DecodeElement(&entry, se); err != nil {
		return err
	}
	if entry.Name == "" {
		return nil
	}
	*entries = append(*entries, MetadataEntry{Name: entry.Name, Enabled: BoolPtr(entry.Enabled)})
	return nil
}

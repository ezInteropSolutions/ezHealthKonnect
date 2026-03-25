package models

import (
	"encoding/xml"
	"time"
)

// ============================================================================
// Mirth Connect XML Parsing Structs
// ============================================================================

// MirthChannelXML is the top-level XML structure for a Mirth Connect channel export.
type MirthChannelXML struct {
	XMLName             xml.Name            `xml:"channel"`
	ID                  string              `xml:"id"`
	Name                string              `xml:"name"`
	Description         string              `xml:"description"`
	Enabled             bool                `xml:"enabled"`
	Version             string              `xml:"version"`
	SourceConnector     MirthConnectorXML   `xml:"sourceConnector"`
	DestConnectors      []MirthConnectorXML `xml:"destinationConnectors>connector"`
}

// MirthConnectorXML represents one source or destination connector.
type MirthConnectorXML struct {
	Name          string              `xml:"name"`
	TransportName string              `xml:"transportName"`
	Properties    MirthPropertiesXML  `xml:"properties"`
	Transformer   MirthTransformerXML `xml:"transformer"`
	Filter        MirthFilterXML      `xml:"filter"`
	Enabled       bool                `xml:"enabled"`
}

// MirthPropertiesXML captures the class attribute and raw inner XML so each
// transport type can extract its own configuration fields.
type MirthPropertiesXML struct {
	Class string `xml:"class,attr"`
	Inner []byte `xml:",innerxml"`
}

// MirthTransformerXML holds the raw inner XML of the <transformer> element.
// Mirth Connect 3.x places steps inside <elements> using Java class names as
// element tags. Earlier versions used <steps><step type="...">. Both formats
// are parsed in the service layer from the raw bytes.
type MirthTransformerXML struct {
	Inner []byte `xml:",innerxml"`
}

// MirthFilterXML holds the raw inner XML of the <filter> element.
type MirthFilterXML struct {
	Inner []byte `xml:",innerxml"`
}

// MirthStepXML is a single transformer step.
type MirthStepXML struct {
	SequenceNumber int    `xml:"sequenceNumber"`
	Name           string `xml:"name"`
	Type           string `xml:"type"`
	Inner          []byte `xml:",innerxml"` // contains <data class="map"> script entries
}

// MirthRuleXML is a single filter rule.
type MirthRuleXML struct {
	SequenceNumber int    `xml:"sequenceNumber"`
	Name           string `xml:"name"`
	Type           string `xml:"type"`
	Inner          []byte `xml:",innerxml"`
}

// ============================================================================
// Migration Preview / Result Structs
// ============================================================================

// MirthMappingStatus describes how well a Mirth element mapped to ezHealthKonnect.
type MirthMappingStatus string

const (
	MirthMappingAuto        MirthMappingStatus = "auto"        // fully auto-mapped, no action needed
	MirthMappingReview      MirthMappingStatus = "review"      // mapped but script/config needs user review
	MirthMappingUnsupported MirthMappingStatus = "unsupported" // no equivalent exists
)

// MappedConnector is the result of mapping a Mirth connector to an ezHK connector.
type MappedConnector struct {
	MirthTransport    string                 `json:"mirthTransport"`
	MirthClass        string                 `json:"mirthClass"`
	EzHKConnectorType string                 `json:"ezHKConnectorType"`
	Config            map[string]interface{} `json:"config"`
	Status            MirthMappingStatus     `json:"status"`
	Note              string                 `json:"note,omitempty"`
}

// MirthFieldMapping is one auto-converted Mirth Mapper row.
type MirthFieldMapping struct {
	Source       string `json:"source"`       // ezHK path — e.g. "PID.3.1"
	Target       string `json:"target"`       // variable name in the Mirth script
	MirthPath    string `json:"mirthPath"`    // original Mirth expression — e.g. msg['PID']['PID.3']['PID.3.1']
	DefaultValue string `json:"defaultValue,omitempty"`
	Scope        string `json:"scope,omitempty"` // LOCAL | GLOBAL | CHANNEL
}

// TransformerStepConversionType describes how a step was (or will be) converted.
type TransformerStepConversionType string

const (
	ConversionAuto   TransformerStepConversionType = "auto"    // deterministically converted, no LLM
	ConversionAI     TransformerStepConversionType = "ai"      // LLM rewrote the script
	ConversionManual TransformerStepConversionType = "manual"  // user must review/rewrite
)

// TransformerStepAnalysis is the analysis of a single Mirth transformer step or
// filter rule — what it does and how it can be represented in ezHK.
type TransformerStepAnalysis struct {
	Sequence       int                           `json:"sequence"`
	Name           string                        `json:"name"`
	MirthType      string                        `json:"mirthType"`
	Script         string                        `json:"script"`          // original or AI-rewritten script
	OriginalScript string                        `json:"originalScript,omitempty"` // only set after AI rewrite
	SuggestedType  string                        `json:"suggestedType"`   // e.g. "enrichment.script"
	Conversion     TransformerStepConversionType `json:"conversion"`      // auto | ai | manual
	AutoConverted  bool                          `json:"autoConverted"`   // kept for backward compat
	IsFilterRule   bool                          `json:"isFilterRule"`
	FieldMappings  []MirthFieldMapping            `json:"fieldMappings,omitempty"` // populated for Mapper steps
	Note           string                        `json:"note,omitempty"`
}

// MigrationSummary is a quick count breakdown for the UI.
type MigrationSummary struct {
	AutoMapped  int `json:"autoMapped"`
	NeedsReview int `json:"needsReview"`
	Unsupported int `json:"unsupported"`
	TotalSteps  int `json:"totalSteps"`
}

// MigrationPreview is the full analysis returned by the preview endpoint before
// any database writes occur.
type MigrationPreview struct {
	ChannelID    string                    `json:"channelId"`
	ChannelName  string                    `json:"channelName"`
	Description  string                    `json:"description"`
	MirthVersion string                    `json:"mirthVersion"`
	Enabled      bool                      `json:"enabled"`
	Source       MappedConnector           `json:"source"`
	Destinations []MappedConnector         `json:"destinations"`
	Steps        []TransformerStepAnalysis `json:"steps"`
	Summary      MigrationSummary          `json:"summary"`
}

// MigrationImportRequest is the payload sent by the UI to kick off an actual import.
type MigrationImportRequest struct {
	ChannelXML      string `json:"channelXml"`      // base64-encoded XML bytes
	InterfaceName   string `json:"interfaceName"`
	Description     string `json:"description"`
	SkipStepIndices []int  `json:"skipStepIndices"` // zero-based indices of steps to omit
	UserID          string `json:"userId"`
}

// MigrationImportResult is returned after a successful import.
type MigrationImportResult struct {
	InterfaceID   string   `json:"interfaceId"`
	PipelineID    string   `json:"pipelineId"`
	InterfaceName string   `json:"interfaceName"`
	StepsCreated  int      `json:"stepsCreated"`
	StepsSkipped  int      `json:"stepsSkipped"`
	Warnings      []string `json:"warnings"`
}

// MirthMigrationHistoryEntry is one row from mirth_migration_history.
type MirthMigrationHistoryEntry struct {
	ID            string    `json:"id"`
	InterfaceID   string    `json:"interfaceId"`
	InterfaceName string    `json:"interfaceName"`
	ChannelID     string    `json:"channelId"`
	ChannelName   string    `json:"channelName"`
	MirthVersion  string    `json:"mirthVersion"`
	StepsCreated  int       `json:"stepsCreated"`
	StepsSkipped  int       `json:"stepsSkipped"`
	MigratedBy    string    `json:"migratedBy"`
	CreatedAt     time.Time `json:"createdAt"`
}


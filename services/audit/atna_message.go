// services/audit/atna_message.go
//
// Builds the DICOM PS3.15 Appendix A.5 / RFC 3881 "AuditMessage" XML body
// that IHE ATNA's own Audit Trail transaction (ITI-20, "Record Audit Event")
// carries over syslog to an external Audit Record Repository (ARR).
//
// SCOPE/HONESTY NOTE: this is a pragmatic, well-structured subset built from
// what AuditEvent + its resolved taxonomy fields actually carry — the
// REQUIRED elements (EventIdentification/EventID/EventActionCode/
// EventOutcomeIndicator/EventDateTime, ActiveParticipant/UserID,
// AuditSourceIdentification/AuditSourceID) are populated with high
// confidence in their meaning. The OPTIONAL, more granular coded attributes
// (ParticipantObjectTypeCodeRole, the finer ParticipantObjectIDTypeCode
// sub-codes) are intentionally left at safe, well-documented top-level
// values rather than guessed at fine granularity — this codebase has no
// real external ARR to validate full interop against (per this project's
// own "verify against the real spec, don't guess" discipline), so precision
// claims stop where actual confidence does.
package audit

import (
	"encoding/xml"
	"time"
)

type auditMessageXML struct {
	XMLName                          xml.Name                             `xml:"AuditMessage"`
	EventIdentification               eventIdentificationXML               `xml:"EventIdentification"`
	ActiveParticipant                 activeParticipantXML                 `xml:"ActiveParticipant"`
	AuditSourceIdentification         auditSourceIdentificationXML         `xml:"AuditSourceIdentification"`
	ParticipantObjectIdentification   *participantObjectIdentificationXML  `xml:"ParticipantObjectIdentification,omitempty"`
}

type eventIdentificationXML struct {
	EventActionCode       string        `xml:"EventActionCode,attr,omitempty"`
	EventDateTime          string        `xml:"EventDateTime,attr"`
	EventOutcomeIndicator string        `xml:"EventOutcomeIndicator,attr"`
	EventID                 codedValueXML `xml:"EventID"`
}

type codedValueXML struct {
	Code           string `xml:"code,attr"`
	CodeSystemName string `xml:"codeSystemName,attr"`
	DisplayName    string `xml:"displayName,attr,omitempty"`
}

type activeParticipantXML struct {
	UserID                     string `xml:"UserID,attr"`
	UserIsRequestor            string `xml:"UserIsRequestor,attr"`
	NetworkAccessPointID       string `xml:"NetworkAccessPointID,attr,omitempty"`
	NetworkAccessPointTypeCode string `xml:"NetworkAccessPointTypeCode,attr,omitempty"`
}

type auditSourceIdentificationXML struct {
	AuditSourceID         string `xml:"AuditSourceID,attr"`
	AuditEnterpriseSiteID string `xml:"AuditEnterpriseSiteID,attr,omitempty"`
	AuditSourceTypeCode   string `xml:"AuditSourceTypeCode,omitempty"`
}

type participantObjectIdentificationXML struct {
	ParticipantObjectID         string        `xml:"ParticipantObjectID,attr"`
	ParticipantObjectTypeCode   string        `xml:"ParticipantObjectTypeCode,attr,omitempty"`
	ParticipantObjectIDTypeCode codedValueXML `xml:"ParticipantObjectIDTypeCode"`
}

// buildAuditMessageXML converts a resolved audit event into the AuditMessage
// XML body. sourceID/enterpriseSiteID identify THIS application instance as
// the audit source (DICOM AuditSourceIdentification), configured once for
// the whole exporter, not per-event.
func buildAuditMessageXML(event AuditEvent, resolved resolvedEvent, sourceID, enterpriseSiteID string) ([]byte, error) {
	tax := lookupTaxonomy(event.Action)

	eventID := tax.ATNAEventID
	eventIDDisplay := tax.ATNAEventIDDisplay
	if eventID == "" {
		// No taxonomy entry (or an unregistered action) — EventID is
		// mandatory per the schema, so fall back to a generic
		// "Application Activity" event rather than omitting it.
		eventID = "110100"
		eventIDDisplay = "Application Activity"
	}

	userID := event.UserID
	if userID == "" {
		userID = "system"
	}

	msg := auditMessageXML{
		EventIdentification: eventIdentificationXML{
			EventActionCode:        tax.EventActionCode,
			EventDateTime:          time.Now().UTC().Format(time.RFC3339),
			EventOutcomeIndicator:  atnaOutcomeIndicator(resolved.Result),
			EventID: codedValueXML{
				Code:           eventID,
				CodeSystemName: "DCM",
				DisplayName:    eventIDDisplay,
			},
		},
		ActiveParticipant: activeParticipantXML{
			UserID:                     userID,
			UserIsRequestor:            "true",
			NetworkAccessPointID:       event.IPAddress,
			NetworkAccessPointTypeCode: networkAccessPointTypeCode(event.IPAddress),
		},
		AuditSourceIdentification: auditSourceIdentificationXML{
			AuditSourceID:         sourceID,
			AuditEnterpriseSiteID: enterpriseSiteID,
			// "4" = Application Server process, per RFC 3881's
			// AuditSourceTypeCode enumeration — a safe, high-confidence
			// top-level value for what this application is.
			AuditSourceTypeCode: "4",
		},
	}

	if event.EntityID != "" {
		// ParticipantObjectTypeCode: RFC 3881's top-level enum is small and
		// well-documented (1=Person, 2=System/Object, 3=Organization,
		// 4=Other) — confident distinguishing a "User" entity (a person)
		// from everything else this codebase audits (interfaces, pipelines,
		// messages, DLQ rows — all system objects), without guessing at the
		// more granular, less-certain ParticipantObjectTypeCodeRole
		// sub-codes (deliberately omitted — see this file's own header).
		typeCode := "2"
		if event.EntityType == "User" {
			typeCode = "1"
		}
		msg.ParticipantObjectIdentification = &participantObjectIdentificationXML{
			ParticipantObjectID:       event.EntityID,
			ParticipantObjectTypeCode: typeCode,
			ParticipantObjectIDTypeCode: codedValueXML{
				Code:           "1",
				CodeSystemName: "RFC-3881",
				DisplayName:    entityTypeDisplay(event.EntityType),
			},
		}
	}

	return xml.Marshal(msg)
}

func networkAccessPointTypeCode(ip string) string {
	if ip == "" {
		return ""
	}
	// "2" = IP Address, per RFC 3881's NetworkAccessPointTypeCode
	// enumeration — the only shape event.IPAddress ever carries in this
	// codebase (never a machine/DNS name).
	return "2"
}

func entityTypeDisplay(entityType string) string {
	if entityType == "" {
		return "Entity"
	}
	return entityType
}

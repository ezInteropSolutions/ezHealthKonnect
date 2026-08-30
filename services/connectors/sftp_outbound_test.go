// services/connectors/sftp_outbound_test.go
// Coverage for resolveFilename/resolvePatternFilename -- pure logic that
// doesn't need a live SFTP server. There is no live-server verification for
// this connector in this session (same "cannot test live infra" limitation
// as the cloud warehouse connectors) -- see file header comment on
// sftp_outbound.go.
package connectors

import (
	"strings"
	"testing"

	"ezhealthkonnect/models"
)

func newTestSFTPOutbound() *SFTPOutboundConnector {
	return &SFTPOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(ConnectorMetadata{TypeName: "sftp_outbound"}, true),
	}
}

func TestSFTPResolveFilename_UsesPatternWhenSet(t *testing.T) {
	c := newTestSFTPOutbound()
	c.filenamePattern = "{message_id}.hl7"
	name := c.resolveFilename(&models.OutboundMessage{MessageID: "msg-123", ContentType: "application/hl7-v2"})
	if name != "msg-123.hl7" {
		t.Errorf("resolveFilename() = %q, want %q", name, "msg-123.hl7")
	}
}

func TestSFTPResolveFilename_PatternSupportsNestedSubdirectories(t *testing.T) {
	c := newTestSFTPOutbound()
	c.filenamePattern = "{interface_id}/{message_id}.hl7"
	name := c.resolveFilename(&models.OutboundMessage{MessageID: "msg-1", InterfaceID: "if-42"})
	if name != "if-42/msg-1.hl7" {
		t.Errorf("resolveFilename() = %q, want %q", name, "if-42/msg-1.hl7")
	}
}

func TestSFTPResolvePatternFilename_SanitizesSlashesInMessageID(t *testing.T) {
	c := newTestSFTPOutbound()
	c.filenamePattern = "{message_id}.hl7"
	name := c.resolveFilename(&models.OutboundMessage{MessageID: "msg/123"})
	if strings.Contains(name, "msg/123") {
		t.Errorf("expected slashes in message_id to be sanitized, got: %s", name)
	}
}

func TestSFTPResolvePatternFilename_AppendsExtensionWhenPatternHasNone(t *testing.T) {
	c := newTestSFTPOutbound()
	c.filenamePattern = "{message_id}"
	name := c.resolveFilename(&models.OutboundMessage{MessageID: "msg-1", ContentType: "application/json"})
	if !strings.HasSuffix(name, ".json") {
		t.Errorf("expected an inferred .json extension appended, got: %s", name)
	}
}

func TestSFTPResolveFilename_FallsBackToLegacyFieldsWhenPatternUnset(t *testing.T) {
	c := newTestSFTPOutbound()
	c.filenamePrefix = "msg_"
	c.fileExtension = ".dat"
	name := c.resolveFilename(&models.OutboundMessage{MessageID: "abcdefgh12345"})
	if !strings.HasPrefix(name, "msg_") || !strings.HasSuffix(name, ".dat") {
		t.Errorf("expected legacy prefix/extension behavior when filename_pattern unset, got: %s", name)
	}
}

func TestSFTPResolveFilename_LegacyFilenameFieldTakesPriorityOverDefaults(t *testing.T) {
	c := newTestSFTPOutbound()
	c.filenameField = "custom_name"
	name := c.resolveFilename(&models.OutboundMessage{
		MessageID: "msg-1",
		Metadata:  map[string]string{"custom_name": "exact-file.txt"},
	})
	if name != "exact-file.txt" {
		t.Errorf("expected legacy filename_field metadata lookup to win, got: %s", name)
	}
}

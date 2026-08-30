// services/connectors/azure_blob_outbound_test.go
// Coverage for buildBlobName/accessTier/Validate -- pure logic that doesn't
// need a live Azure Storage account. Unlike Databricks/Snowflake, this
// connector's real connectivity IS separately verified against a throwaway
// Azurite (official Azure Storage emulator) container -- see the live test
// notes in azure_blob_outbound.go's file header for what that proved and
// what it didn't.
package connectors

import (
	"strings"
	"testing"

	"ezhealthkonnect/models"
)

func newTestAzureBlobOutbound(cfg AzureBlobOutboundConfig) *AzureBlobOutboundConnector {
	return &AzureBlobOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(ConnectorMetadata{TypeName: "azure_blob_outbound"}, true),
		config:                cfg,
	}
}

func TestAzureBlobBuildBlobName_DefaultsToMessageIDJSON(t *testing.T) {
	c := newTestAzureBlobOutbound(AzureBlobOutboundConfig{KeyPattern: "{message_id}.json"})
	name := c.buildBlobName(&models.OutboundMessage{MessageID: "msg-123"})
	if name != "msg-123.json" {
		t.Errorf("buildBlobName() = %q, want %q", name, "msg-123.json")
	}
}

func TestAzureBlobBuildBlobName_SanitizesSlashesInMessageID(t *testing.T) {
	c := newTestAzureBlobOutbound(AzureBlobOutboundConfig{KeyPattern: "{interface_id}/{message_id}.json"})
	name := c.buildBlobName(&models.OutboundMessage{MessageID: "msg/123", InterfaceID: "if:1"})
	if strings.Contains(name, "msg/123") {
		t.Errorf("expected slashes in message_id to be sanitized, got: %s", name)
	}
}

func TestAzureBlobBuildBlobName_AppendsExtensionWhenPatternHasNone(t *testing.T) {
	c := newTestAzureBlobOutbound(AzureBlobOutboundConfig{KeyPattern: "{message_id}"})
	name := c.buildBlobName(&models.OutboundMessage{MessageID: "msg-1", ContentType: "application/json"})
	if !strings.HasSuffix(name, ".json") {
		t.Errorf("expected an inferred .json extension appended, got: %s", name)
	}
}

func TestAzureBlobBuildBlobName_KeepsExtensionAlreadyInPattern(t *testing.T) {
	c := newTestAzureBlobOutbound(AzureBlobOutboundConfig{KeyPattern: "{message_id}.hl7"})
	name := c.buildBlobName(&models.OutboundMessage{MessageID: "msg-1", ContentType: "application/json"})
	if name != "msg-1.hl7" {
		t.Errorf("expected explicit .hl7 extension to be preserved, got: %s", name)
	}
}

func TestAzureBlobAccessTier_ValidValues(t *testing.T) {
	cases := map[string]bool{"hot": true, "Cool": true, "ARCHIVE": true, "": false, "bogus": false}
	for tier, wantNonNil := range cases {
		c := newTestAzureBlobOutbound(AzureBlobOutboundConfig{AccessTier: tier})
		got := c.accessTier()
		if (got != nil) != wantNonNil {
			t.Errorf("accessTier() for %q: got non-nil=%v, want non-nil=%v", tier, got != nil, wantNonNil)
		}
	}
}

func TestAzureBlobInitialize_RejectsInvalidAccessTier(t *testing.T) {
	c := newTestAzureBlobOutbound(AzureBlobOutboundConfig{})
	cfg := []byte(`{"container":"c","connection_string":"UseDevelopmentStorage=true","access_tier":"lukewarm"}`)
	err := c.Initialize(cfg)
	if err == nil {
		t.Fatal("expected Initialize to reject an unrecognized access_tier")
	}
	if !strings.Contains(err.Error(), "access_tier") {
		t.Errorf("expected error to mention access_tier, got: %v", err)
	}
}

func TestAzureBlobInitialize_RequiresContainer(t *testing.T) {
	c := newTestAzureBlobOutbound(AzureBlobOutboundConfig{})
	cfg := []byte(`{"connection_string":"UseDevelopmentStorage=true"}`)
	err := c.Initialize(cfg)
	if err == nil || !strings.Contains(err.Error(), "container is required") {
		t.Errorf("expected a container-is-required error, got: %v", err)
	}
}

func TestAzureBlobInitialize_RequiresAuthMode(t *testing.T) {
	c := newTestAzureBlobOutbound(AzureBlobOutboundConfig{})
	cfg := []byte(`{"container":"c"}`)
	err := c.Initialize(cfg)
	if err == nil || !strings.Contains(err.Error(), "connection_string") {
		t.Errorf("expected an auth-mode-required error, got: %v", err)
	}
}

func TestAzureBlobInitialize_AccountNameWithoutKey_Rejected(t *testing.T) {
	c := newTestAzureBlobOutbound(AzureBlobOutboundConfig{})
	cfg := []byte(`{"container":"c","account_name":"acct"}`)
	err := c.Initialize(cfg)
	if err == nil {
		t.Fatal("expected Initialize to reject account_name without account_key")
	}
}

func TestAzureBlobValidate_RequiresConnectionFields(t *testing.T) {
	cases := []struct {
		name string
		cfg  AzureBlobOutboundConfig
	}{
		{"missing container", AzureBlobOutboundConfig{ConnectionString: "UseDevelopmentStorage=true"}},
		{"missing auth", AzureBlobOutboundConfig{Container: "c"}},
		{"account_name without key", AzureBlobOutboundConfig{Container: "c", AccountName: "a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestAzureBlobOutbound(tc.cfg)
			c.BaseOutboundConnector.BaseConnector.initialized = true
			if err := c.Validate(); err == nil {
				t.Errorf("expected Validate to reject config: %+v", tc.cfg)
			}
		})
	}
}

func TestAzureBlobValidate_PassesWithConnectionString(t *testing.T) {
	c := newTestAzureBlobOutbound(AzureBlobOutboundConfig{Container: "c", ConnectionString: "UseDevelopmentStorage=true"})
	c.BaseOutboundConnector.BaseConnector.initialized = true
	if err := c.Validate(); err != nil {
		t.Errorf("expected Validate to pass, got: %v", err)
	}
}

func TestAzureBlobValidate_PassesWithAccountNameAndKey(t *testing.T) {
	c := newTestAzureBlobOutbound(AzureBlobOutboundConfig{Container: "c", AccountName: "a", AccountKey: "k"})
	c.BaseOutboundConnector.BaseConnector.initialized = true
	if err := c.Validate(); err != nil {
		t.Errorf("expected Validate to pass, got: %v", err)
	}
}

func TestAzureBlobSendBatch_AggregatesPerMessageResults(t *testing.T) {
	// SendBatch requires an initialized client to actually Send(); this just
	// confirms the uninitialized-connector guard fires before any upload runs.
	c := newTestAzureBlobOutbound(AzureBlobOutboundConfig{})
	results, _ := c.SendBatch(nil, []*models.OutboundMessage{{MessageID: "m1"}})
	if len(results) != 1 || results[0].Success {
		t.Errorf("expected a single failed result when the connector isn't initialized, got: %+v", results)
	}
}

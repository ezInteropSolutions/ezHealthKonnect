// services/connectors/azure_blob_inbound_test.go
// Coverage for pure config/validation logic. The SDK-dependent parts (listing,
// downloading, after-processing) are verified live against a throwaway
// Azurite (official Azure Storage emulator) container — see the file header
// on azure_blob_inbound.go and this session's Azure Blob outbound precedent
// for the same approach.
package connectors

import (
	"strings"
	"testing"
)

func newTestAzureBlobInbound(cfg AzureBlobInboundConfig) *AzureBlobInboundConnector {
	return &AzureBlobInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(ConnectorMetadata{TypeName: "azure_blob_inbound"}),
		config:                cfg,
	}
}

func TestAzureBlobInboundInitialize_RequiresContainer(t *testing.T) {
	c := newTestAzureBlobInbound(AzureBlobInboundConfig{})
	cfg := []byte(`{"connection_string":"UseDevelopmentStorage=true"}`)
	err := c.Initialize(cfg)
	if err == nil || !strings.Contains(err.Error(), "container is required") {
		t.Errorf("expected a container-is-required error, got: %v", err)
	}
}

func TestAzureBlobInboundInitialize_RequiresAuthMode(t *testing.T) {
	c := newTestAzureBlobInbound(AzureBlobInboundConfig{})
	cfg := []byte(`{"container":"c"}`)
	err := c.Initialize(cfg)
	if err == nil || !strings.Contains(err.Error(), "connection_string") {
		t.Errorf("expected an auth-mode-required error, got: %v", err)
	}
}

func TestAzureBlobInboundInitialize_AccountNameWithoutKey_Rejected(t *testing.T) {
	c := newTestAzureBlobInbound(AzureBlobInboundConfig{})
	cfg := []byte(`{"container":"c","account_name":"acct"}`)
	err := c.Initialize(cfg)
	if err == nil {
		t.Fatal("expected Initialize to reject account_name without account_key")
	}
}

func TestAzureBlobInboundValidate_RequiresConnectionFields(t *testing.T) {
	cases := []struct {
		name string
		cfg  AzureBlobInboundConfig
	}{
		{"missing container", AzureBlobInboundConfig{ConnectionString: "UseDevelopmentStorage=true"}},
		{"missing auth", AzureBlobInboundConfig{Container: "c"}},
		{"account_name without key", AzureBlobInboundConfig{Container: "c", AccountName: "a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestAzureBlobInbound(tc.cfg)
			c.BaseInboundConnector.BaseConnector.initialized = true
			if err := c.Validate(); err == nil {
				t.Errorf("expected Validate to reject config: %+v", tc.cfg)
			}
		})
	}
}

func TestAzureBlobInboundValidate_PassesWithConnectionString(t *testing.T) {
	c := newTestAzureBlobInbound(AzureBlobInboundConfig{Container: "c", ConnectionString: "UseDevelopmentStorage=true"})
	c.BaseInboundConnector.BaseConnector.initialized = true
	if err := c.Validate(); err != nil {
		t.Errorf("expected Validate to pass, got: %v", err)
	}
}

func TestAzureBlobInboundInitialize_DefaultsApplied(t *testing.T) {
	c := newTestAzureBlobInbound(AzureBlobInboundConfig{})
	// A syntactically well-formed (but fake) connection string -- azblob's
	// client constructor validates format eagerly (no AccountName= key fails
	// immediately) without making any network call, so this is enough to
	// prove Initialize()'s own default-application logic without needing a
	// real or emulated storage account.
	cfg := []byte(`{"container":"c","connection_string":"DefaultEndpointsProtocol=https;AccountName=fakeaccount;AccountKey=ZmFrZWtleQ==;EndpointSuffix=core.windows.net"}`)
	if err := c.Initialize(cfg); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if c.config.PollingInterval != 60 {
		t.Errorf("expected default polling_interval=60, got: %d", c.config.PollingInterval)
	}
	if c.config.MaxBlobs != 100 {
		t.Errorf("expected default max_blobs=100, got: %d", c.config.MaxBlobs)
	}
	if c.config.AfterProcessing != "nothing" {
		t.Errorf("expected default after_processing='nothing', got: %s", c.config.AfterProcessing)
	}
}

func TestAzureBlobInboundSupportsCron(t *testing.T) {
	c := newTestAzureBlobInbound(AzureBlobInboundConfig{})
	if !c.SupportsCron() {
		t.Error("expected SupportsCron to return true")
	}
}

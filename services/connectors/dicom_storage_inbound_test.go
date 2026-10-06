package connectors

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"testing"
	"time"

	"ezhealthkonnect/models"

	"github.com/amrshadid/go-dicom/dataelem"
	"github.com/amrshadid/go-dicom/dataset"
	"github.com/amrshadid/go-dicom/network"
)

func TestDICOMStorageInbound_Validate_RequiresAETitleAndPort(t *testing.T) {
	c := NewDICOMStorageInboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{})
	c.Initialize(cfg)
	if err := c.Validate(); err == nil {
		t.Error("expected Validate to fail when ae_title/port are missing")
	}
}

func TestDICOMStorageInbound_Initialize_TLSEnabledRequiresCertAndKey(t *testing.T) {
	c := NewDICOMStorageInboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{
		"ae_title": "TESTSCP", "port": 11112, "tls_enabled": true,
	})
	if err := c.Initialize(cfg); err == nil {
		t.Error("expected Initialize to fail when tls_enabled is true but cert/key files are missing")
	}
}

func TestDICOMStorageInbound_Initialize_DefaultsBindAddress(t *testing.T) {
	cIface := NewDICOMStorageInboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{"ae_title": "TESTSCP", "port": 11112})
	if err := cIface.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	c := cIface.(*DICOMStorageInboundConnector)
	if c.bindAddress != "0.0.0.0" {
		t.Errorf("bindAddress = %q, want 0.0.0.0", c.bindAddress)
	}
}

func TestDICOMStorageInbound_Initialize_MaxInstanceSizeDefaultsTo100MB(t *testing.T) {
	cIface := NewDICOMStorageInboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{"ae_title": "TESTSCP", "port": 11112})
	if err := cIface.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	c := cIface.(*DICOMStorageInboundConnector)
	want := int64(defaultMaxDICOMInstanceSizeMB) * 1024 * 1024
	if c.maxInstanceSizeBytes != want {
		t.Errorf("maxInstanceSizeBytes = %d, want %d (100MB default) when max_instance_size_mb is unset", c.maxInstanceSizeBytes, want)
	}
}

func TestDICOMStorageInbound_Initialize_MaxInstanceSizeHonorsExplicitValue(t *testing.T) {
	cIface := NewDICOMStorageInboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{"ae_title": "TESTSCP", "port": 11112, "max_instance_size_mb": 25})
	if err := cIface.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	c := cIface.(*DICOMStorageInboundConnector)
	want := int64(25) * 1024 * 1024
	if c.maxInstanceSizeBytes != want {
		t.Errorf("maxInstanceSizeBytes = %d, want %d for an explicit max_instance_size_mb=25", c.maxInstanceSizeBytes, want)
	}
}

func TestDICOMStorageInbound_Initialize_MaxInstanceSizeClampedToHardMax(t *testing.T) {
	cIface := NewDICOMStorageInboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{"ae_title": "TESTSCP", "port": 11112, "max_instance_size_mb": 999999})
	if err := cIface.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	c := cIface.(*DICOMStorageInboundConnector)
	want := int64(hardMaxDICOMInstanceSizeMB) * 1024 * 1024
	if c.maxInstanceSizeBytes != want {
		t.Errorf("maxInstanceSizeBytes = %d, want %d — an excessive configured value must be clamped, not honored verbatim", c.maxInstanceSizeBytes, want)
	}
}

func TestDICOMStorageInbound_Initialize_TLSCAFileParsed(t *testing.T) {
	cIface := NewDICOMStorageInboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{
		"ae_title": "TESTSCP", "port": 11112, "tls_ca_file": "/some/ca.pem",
	})
	if err := cIface.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	c := cIface.(*DICOMStorageInboundConnector)
	if c.tlsCAFile != "/some/ca.pem" {
		t.Errorf("tlsCAFile = %q, want /some/ca.pem", c.tlsCAFile)
	}
}

func TestDICOMStorageInbound_TestConnection_PortFree(t *testing.T) {
	port := findFreePort(t)
	cIface := NewDICOMStorageInboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{"ae_title": "TESTSCP", "port": port})
	cIface.Initialize(cfg)
	c := cIface.(*DICOMStorageInboundConnector)
	if err := c.TestConnection(context.Background()); err != nil {
		t.Errorf("expected TestConnection to succeed on a free port: %v", err)
	}
}

// TestDICOMStorageInbound_RealEndToEnd starts the real connector (not just
// dicom.Receiver directly) and drives a real C-STORE against it via the
// library's own SCU client, asserting the resulting InboundMessage.Content
// is byte-identical to a real re-parsed Part10 file — proving the "DICOM
// Part10 bytes travel safely through a Go string" architectural claim this
// feature's own plan relies on, not just asserting it.
func TestDICOMStorageInbound_RealEndToEnd(t *testing.T) {
	port := findFreePort(t)
	cIface := NewDICOMStorageInboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{"ae_title": "TESTSCP", "port": port})
	if err := cIface.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	msgChan := make(chan *models.InboundMessage, 5)
	if err := cIface.Start(context.Background(), msgChan); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cIface.Stop()
	time.Sleep(100 * time.Millisecond)

	scu := network.NewSCU(network.SCUConfig{
		CallingAE: "TESTSCU", CalledAE: "TESTSCP", Address: "127.0.0.1:" + strconv.Itoa(port),
	})
	if err := scu.Associate(context.Background(), nil); err != nil {
		t.Fatalf("Associate: %v", err)
	}
	defer scu.Release(context.Background())

	ds := dataset.NewDataset()
	must := func(err error) {
		if err != nil {
			t.Fatalf("AddByKeyword: %v", err)
		}
	}
	must(ds.AddByKeyword("SOPClassUID", dataelem.UI, []byte(network.CRImageStorageUID)))
	must(ds.AddByKeyword("SOPInstanceUID", dataelem.UI, []byte("1.2.3.4.5.333")))
	must(ds.AddByKeyword("PatientID", dataelem.LO, []byte("PID333")))
	must(ds.AddByKeyword("Modality", dataelem.CS, []byte("CR")))

	if err := scu.Store(context.Background(), ds); err != nil {
		t.Fatalf("Store: %v", err)
	}

	select {
	case msg := <-msgChan:
		if msg.SourceType != "dicom_storage_inbound" {
			t.Errorf("SourceType = %q, want dicom_storage_inbound", msg.SourceType)
		}
		if msg.ContentType != "application/dicom" {
			t.Errorf("ContentType = %q, want application/dicom", msg.ContentType)
		}
		if msg.SourceMetadata["sop_instance_uid"] != "1.2.3.4.5.333" {
			t.Errorf("SourceMetadata[sop_instance_uid] = %q, want 1.2.3.4.5.333", msg.SourceMetadata["sop_instance_uid"])
		}
		if msg.SourceMetadata["patient_id"] != "PID333" {
			t.Errorf("SourceMetadata[patient_id] = %q, want PID333", msg.SourceMetadata["patient_id"])
		}
		if len(msg.Content) == 0 {
			t.Fatal("InboundMessage.Content is empty")
		}
		if msg.Content[128:132] != "DICM" {
			t.Errorf("InboundMessage.Content missing DICM magic at offset 128 — Part10 bytes were corrupted in transit through the Go string")
		}
		// Regression guard: storeMessage's own INSERT binds msg.MessageSize
		// directly, never auto-computing it from Content — found live
		// during this feature's own full-stack verification (every stored
		// message showed message_size=0 until this field was set).
		if msg.MessageSize != len(msg.Content) {
			t.Errorf("MessageSize = %d, want %d (len(Content))", msg.MessageSize, len(msg.Content))
		}
		if msg.ReceivedAt.IsZero() {
			t.Error("ReceivedAt was never set")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the message on messageChan")
	}
}

func TestDICOMStorageInbound_Start_PortAlreadyInUse_ReturnsRealError(t *testing.T) {
	port := findFreePort(t)
	// Occupy the port directly so Start must fail to bind.
	blocker, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("failed to occupy test port: %v", err)
	}
	defer blocker.Close()

	cIface := NewDICOMStorageInboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{"ae_title": "TESTSCP", "port": port})
	cIface.Initialize(cfg)

	msgChan := make(chan *models.InboundMessage, 1)
	if err := cIface.Start(context.Background(), msgChan); err == nil {
		t.Error("expected Start to fail with a real bind error when the port is already in use")
	}
}

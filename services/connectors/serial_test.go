package connectors

import (
	"context"
	"encoding/json"
	"testing"

	"go.bug.st/serial"
)

func TestParseSerialParity_RecognizedValues(t *testing.T) {
	cases := map[string]serial.Parity{
		"":      serial.NoParity,
		"none":  serial.NoParity,
		"odd":   serial.OddParity,
		"even":  serial.EvenParity,
		"mark":  serial.MarkParity,
		"space": serial.SpaceParity,
		// Case-insensitivity, since a user-typed config value's casing
		// shouldn't matter.
		"ODD": serial.OddParity,
		"Even": serial.EvenParity,
	}
	for input, want := range cases {
		got, err := parseSerialParity(input)
		if err != nil {
			t.Errorf("parseSerialParity(%q): unexpected error: %v", input, err)
		}
		if got != want {
			t.Errorf("parseSerialParity(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestParseSerialParity_RejectsUnrecognizedValue(t *testing.T) {
	if _, err := parseSerialParity("bogus"); err == nil {
		t.Error("expected an error for an unrecognized parity value")
	}
}

func TestParseSerialStopBits_RecognizedValues(t *testing.T) {
	cases := map[string]serial.StopBits{
		"":    serial.OneStopBit,
		"1":   serial.OneStopBit,
		"1.5": serial.OnePointFiveStopBits,
		"2":   serial.TwoStopBits,
	}
	for input, want := range cases {
		got, err := parseSerialStopBits(input)
		if err != nil {
			t.Errorf("parseSerialStopBits(%q): unexpected error: %v", input, err)
		}
		if got != want {
			t.Errorf("parseSerialStopBits(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestParseSerialStopBits_RejectsUnrecognizedValue(t *testing.T) {
	if _, err := parseSerialStopBits("3"); err == nil {
		t.Error("expected an error for an unrecognized stop_bits value")
	}
}

func TestSerialInbound_Validate_PortNameRequired(t *testing.T) {
	c := NewSerialInboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{})
	if err := c.Initialize(cfg); err != nil {
		t.Fatalf("Initialize should not itself fail on missing port_name: %v", err)
	}
	if err := c.Validate(); err == nil {
		t.Error("expected Validate to fail when port_name is missing")
	}
}

func TestSerialInbound_Initialize_RejectsUnrecognizedParity(t *testing.T) {
	c := NewSerialInboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{"port_name": "COM3", "parity": "bogus"})
	if err := c.Initialize(cfg); err == nil {
		t.Error("expected Initialize to fail for an unrecognized parity value")
	}
}

func TestSerialInbound_Initialize_RejectsUnrecognizedStopBits(t *testing.T) {
	c := NewSerialInboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{"port_name": "COM3", "stop_bits": "3"})
	if err := c.Initialize(cfg); err == nil {
		t.Error("expected Initialize to fail for an unrecognized stop_bits value")
	}
}

func TestSerialInbound_Initialize_DefaultsBaudRateAndDataBits(t *testing.T) {
	c := NewSerialInboundConnector().(*SerialInboundConnector)
	cfg, _ := json.Marshal(map[string]interface{}{"port_name": "COM3"})
	if err := c.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if c.mode.BaudRate != 9600 {
		t.Errorf("default BaudRate = %d, want 9600", c.mode.BaudRate)
	}
	if c.mode.DataBits != 8 {
		t.Errorf("default DataBits = %d, want 8", c.mode.DataBits)
	}
}

func TestSerialInbound_TestConnection_NonExistentPort_ReturnsError(t *testing.T) {
	c := NewSerialInboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{"port_name": "COM_DEFINITELY_DOES_NOT_EXIST_999"})
	if err := c.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := c.TestConnection(context.Background()); err == nil {
		t.Error("expected TestConnection to fail for a non-existent COM port")
	}
}

func TestSerialOutbound_Validate_PortNameRequired(t *testing.T) {
	c := NewSerialOutboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{})
	if err := c.Initialize(cfg); err != nil {
		t.Fatalf("Initialize should not itself fail on missing port_name: %v", err)
	}
	if err := c.Validate(); err == nil {
		t.Error("expected Validate to fail when port_name is missing")
	}
}

func TestSerialOutbound_Initialize_DefaultsWriteTimeout(t *testing.T) {
	c := NewSerialOutboundConnector().(*SerialOutboundConnector)
	cfg, _ := json.Marshal(map[string]interface{}{"port_name": "COM3"})
	if err := c.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if c.writeTimeout.Seconds() != 30 {
		t.Errorf("default writeTimeout = %v, want 30s", c.writeTimeout)
	}
}

func TestSerialOutbound_TestConnection_NonExistentPort_ReturnsError(t *testing.T) {
	c := NewSerialOutboundConnector()
	cfg, _ := json.Marshal(map[string]interface{}{"port_name": "COM_DEFINITELY_DOES_NOT_EXIST_999"})
	if err := c.Initialize(cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := c.TestConnection(context.Background()); err == nil {
		t.Error("expected TestConnection to fail for a non-existent COM port")
	}
}

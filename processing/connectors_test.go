package processing

import "testing"

func TestExtractInterfaceContext_ReadsInjectedKeys(t *testing.T) {
	// Mirrors what engine.go actually sends: innerConfig["interface_id"] =
	// interfaceID (see ActivateInterface's pipeline-driven and legacy paths).
	configJSON := []byte(`{"interface_id":"iface-123","interface_name":"Epic ADT Listener","directory_path":"/data/inbox"}`)

	id, name := extractInterfaceContext(configJSON)
	if id != "iface-123" {
		t.Errorf("interfaceID = %q, want %q", id, "iface-123")
	}
	if name != "Epic ADT Listener" {
		t.Errorf("interfaceName = %q, want %q", name, "Epic ADT Listener")
	}
}

func TestExtractInterfaceContext_MissingKeysReturnEmpty(t *testing.T) {
	configJSON := []byte(`{"directory_path":"/data/inbox"}`)

	id, name := extractInterfaceContext(configJSON)
	if id != "" || name != "" {
		t.Errorf("expected empty id/name when keys are absent, got id=%q name=%q", id, name)
	}
}

func TestExtractInterfaceContext_InvalidJSONReturnsEmpty(t *testing.T) {
	id, name := extractInterfaceContext([]byte(`not json`))
	if id != "" || name != "" {
		t.Errorf("expected empty id/name on unmarshal failure, got id=%q name=%q", id, name)
	}
}

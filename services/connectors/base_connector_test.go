package connectors

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBaseConnector_Logger_FallsBackToGlobalWithoutInterfaceContext(t *testing.T) {
	bc := NewBaseConnector(ConnectorMetadata{TypeName: "test_connector"})

	l := bc.Logger()
	if l == nil {
		t.Fatal("Logger() must never return nil")
	}
}

func TestBaseConnector_SetInterfaceContext_RoutesToInterfaceLog(t *testing.T) {
	interfaceID := "test-base-connector-iface"
	t.Cleanup(func() { os.RemoveAll(filepath.Join("logs", "interfaces", interfaceID)) })

	bc := NewBaseConnector(ConnectorMetadata{TypeName: "test_connector"})
	bc.SetInterfaceContext(interfaceID, "Test Interface")

	bc.Logger().Info("connector activity", "detail", "polling started")

	found := false
	_ = filepath.Walk(filepath.Join("logs", "interfaces", interfaceID), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && info.Name() == "interface.log" {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatal("expected SetInterfaceContext to route Logger() output to a dated interface.log file")
	}
}

func TestBaseConnector_SetInterfaceContext_BlankIDIsNoOp(t *testing.T) {
	bc := NewBaseConnector(ConnectorMetadata{TypeName: "test_connector"})

	// A blank interfaceID (e.g. a connector-test/dry-run flow with no owning
	// interface) must not register a bogus log file or panic — Logger()
	// should keep falling back to the global logger.
	bc.SetInterfaceContext("", "")

	bc.statusMu.RLock()
	ilog := bc.ilog
	bc.statusMu.RUnlock()

	if ilog != nil {
		t.Fatal("expected a blank interfaceID to leave ilog unset")
	}
	if bc.Logger() == nil {
		t.Fatal("Logger() must never return nil even after a no-op SetInterfaceContext")
	}
}

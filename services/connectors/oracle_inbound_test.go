// services/connectors/oracle_inbound_test.go
// Coverage for buildDSN/buildWindowsAuthDSN -- see oracle_outbound_test.go's
// file header for the same "cannot verify against a real domain" caveat.
package connectors

import (
	"strings"
	"testing"
)

func newTestOracleInbound(cfg DatabaseInboundConfig) *OracleInboundConnector {
	return &OracleInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(ConnectorMetadata{TypeName: "oracle_inbound"}),
		config:               cfg,
	}
}

func TestOracleInboundBuildDSN_DefaultPasswordAuth(t *testing.T) {
	c := newTestOracleInbound(DatabaseInboundConfig{
		Host: "oracle.internal", Username: "app_user", Password: "secret", Database: "ORCLPDB1",
	})
	dsn := c.buildDSN()
	want := "oracle://app_user:secret@oracle.internal:1521/ORCLPDB1"
	if dsn != want {
		t.Errorf("buildDSN() = %q, want %q", dsn, want)
	}
}

func TestOracleInboundBuildDSN_RoutesToWindowsAuthWhenSet(t *testing.T) {
	c := newTestOracleInbound(DatabaseInboundConfig{
		Host: "oracle.internal", Database: "ORCLPDB1", AuthType: "windows", OSPassword: "winpass",
	})
	dsn := c.buildDSN()
	// go_ora.BuildUrl's output also starts with "oracle://" (same scheme) --
	// the real distinguishing signal is the AUTH TYPE=OS marker and the
	// absence of a plain username:password pair, not the scheme prefix.
	if !strings.Contains(dsn, "AUTH TYPE=OS") {
		t.Errorf("expected Windows-auth DSN to contain AUTH TYPE=OS, got: %s", dsn)
	}
	if strings.Contains(dsn, "@oracle.internal") && !strings.HasPrefix(dsn, "oracle://:@") {
		t.Errorf("expected empty username:password userinfo for Windows auth, got: %s", dsn)
	}
}

func TestOracleInboundBuildWindowsAuthDSN_IncludesRequiredOptions(t *testing.T) {
	c := newTestOracleInbound(DatabaseInboundConfig{
		Database: "ORCLPDB1", AuthType: "windows", OSUser: "winuser", OSPassword: "winpass", Domain: "CORP",
	})
	dsn := c.buildWindowsAuthDSN("oracle.internal", 1521)
	for _, want := range []string{"winuser", "winpass", "CORP", "NTS"} {
		if !strings.Contains(dsn, want) {
			t.Errorf("expected Windows-auth DSN to contain %q, got: %s", want, dsn)
		}
	}
}

func TestOracleInboundValidate_RequiresOSPasswordForWindowsAuth(t *testing.T) {
	c := newTestOracleInbound(DatabaseInboundConfig{
		TableName: "x", Database: "ORCLPDB1", AuthType: "windows",
	})
	c.BaseInboundConnector.BaseConnector.initialized = true
	if err := c.Validate(); err == nil {
		t.Error("expected Validate to reject windows auth_type without os_password")
	}
}

func TestOracleInboundValidate_PassesWithWindowsAuthAndOSPassword(t *testing.T) {
	c := newTestOracleInbound(DatabaseInboundConfig{
		TableName: "x", Database: "ORCLPDB1", AuthType: "windows", OSPassword: "winpass",
	})
	c.BaseInboundConnector.BaseConnector.initialized = true
	if err := c.Validate(); err != nil {
		t.Errorf("expected Validate to pass, got: %v", err)
	}
}

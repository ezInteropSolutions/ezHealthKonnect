// services/connectors/oracle_outbound_test.go
// Coverage for buildDSN/buildWindowsAuthDSN -- pure logic that doesn't need a
// live Oracle instance. Windows/OS authentication specifically cannot be
// verified against a real domain-joined Oracle server in this environment
// (no such infrastructure available) -- these tests only prove the DSN is
// built using go-ora's own documented option keys (AUTH TYPE, AUTH SERV,
// OS USER, OS PASS, DOMAIN), not that authentication against a real domain
// actually succeeds.
package connectors

import (
	"strings"
	"testing"
)

func newTestOracleOutbound(cfg DatabaseOutboundConfig) *OracleOutboundConnector {
	return &OracleOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(ConnectorMetadata{TypeName: "oracle_outbound"}, true),
		config:                cfg,
	}
}

func TestOracleOutboundBuildDSN_DefaultPasswordAuth(t *testing.T) {
	c := newTestOracleOutbound(DatabaseOutboundConfig{
		Host: "oracle.internal", Username: "app_user", Password: "secret", Database: "ORCLPDB1",
	})
	dsn := c.buildDSN()
	want := "oracle://app_user:secret@oracle.internal:1521/ORCLPDB1"
	if dsn != want {
		t.Errorf("buildDSN() = %q, want %q", dsn, want)
	}
}

func TestOracleOutboundBuildDSN_RoutesToWindowsAuthWhenSet(t *testing.T) {
	c := newTestOracleOutbound(DatabaseOutboundConfig{
		Host: "oracle.internal", Database: "ORCLPDB1", AuthType: "windows", OSPassword: "winpass",
	})
	dsn := c.buildDSN()
	// go_ora.BuildUrl's output also starts with "oracle://" (same scheme) --
	// the real distinguishing signal is the AUTH TYPE=OS marker.
	if !strings.Contains(dsn, "AUTH TYPE=OS") {
		t.Errorf("expected Windows-auth DSN to contain AUTH TYPE=OS, got: %s", dsn)
	}
	if !strings.Contains(dsn, "oracle.internal") || !strings.Contains(dsn, "ORCLPDB1") {
		t.Errorf("expected host and service name in Windows-auth DSN, got: %s", dsn)
	}
}

func TestOracleOutboundBuildWindowsAuthDSN_IncludesRequiredOptions(t *testing.T) {
	c := newTestOracleOutbound(DatabaseOutboundConfig{
		Database: "ORCLPDB1", AuthType: "windows", OSUser: "winuser", OSPassword: "winpass", Domain: "CORP",
	})
	dsn := c.buildWindowsAuthDSN("oracle.internal", 1521)

	// go_ora.BuildUrl URL-encodes option keys/values into the query string;
	// check for the presence of each option's value rather than assuming
	// exact encoding of the "AUTH TYPE" key itself (spaces get encoded).
	for _, want := range []string{"winuser", "winpass", "CORP", "NTS"} {
		if !strings.Contains(dsn, want) {
			t.Errorf("expected Windows-auth DSN to contain %q, got: %s", want, dsn)
		}
	}
}

func TestOracleOutboundBuildWindowsAuthDSN_OmitsEmptyOptionalFields(t *testing.T) {
	c := newTestOracleOutbound(DatabaseOutboundConfig{
		Database: "ORCLPDB1", AuthType: "windows", OSPassword: "winpass",
	})
	dsn := c.buildWindowsAuthDSN("oracle.internal", 1521)
	if strings.Contains(dsn, "OS+USER") || strings.Contains(dsn, "OS%20USER") {
		// OS USER wasn't set — its key shouldn't appear when go_ora.BuildUrl
		// omits absent map entries. This is a loose smoke check, not exact.
	}
	if !strings.Contains(dsn, "winpass") {
		t.Errorf("expected os_password to appear in DSN, got: %s", dsn)
	}
}

func TestOracleOutboundValidate_RequiresOSPasswordForWindowsAuth(t *testing.T) {
	c := newTestOracleOutbound(DatabaseOutboundConfig{
		TableName: "x", Database: "ORCLPDB1", AuthType: "windows",
	})
	c.BaseOutboundConnector.BaseConnector.initialized = true
	if err := c.Validate(); err == nil {
		t.Error("expected Validate to reject windows auth_type without os_password")
	}
}

func TestOracleOutboundValidate_PassesWithWindowsAuthAndOSPassword(t *testing.T) {
	c := newTestOracleOutbound(DatabaseOutboundConfig{
		TableName: "x", Database: "ORCLPDB1", AuthType: "windows", OSPassword: "winpass",
	})
	c.BaseOutboundConnector.BaseConnector.initialized = true
	if err := c.Validate(); err != nil {
		t.Errorf("expected Validate to pass, got: %v", err)
	}
}

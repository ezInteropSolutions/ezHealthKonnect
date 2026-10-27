package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsServiceACLPlan(t *testing.T) {
	dir := filepath.Join("C", "ezHealthKonnect")
	account := serviceAccountName("ezhealthkonnect")
	if account != `NT SERVICE\ezhealthkonnect` {
		t.Fatalf("account = %q", account)
	}
	secret := filepath.Join(dir, "tools", "pg-superuser.secret")
	plan := windowsServiceACLPlan(dir, account, secret)

	grants := map[string]string{} // path -> access granted to the service account
	adminOnly := map[string]bool{}
	for _, cmd := range plan {
		line := strings.Join(cmd, " ")
		if strings.Contains(line, account+":") {
			for _, a := range cmd {
				if strings.HasPrefix(a, account+":") {
					grants[cmd[0]] = strings.TrimPrefix(a, account+":")
				}
			}
		}
		if strings.Contains(line, "/inheritance:r") && !strings.Contains(line, account) {
			adminOnly[cmd[0]] = true
		}
	}

	if grants[dir] != "(OI)(CI)RX" {
		t.Errorf("application files must be read/execute only, got %q", grants[dir])
	}
	for _, d := range []string{"logs", "storage", "uploads", "schemas", "schema-dist", "git-repos", "cache"} {
		if grants[filepath.Join(dir, d)] != "(OI)(CI)M" {
			t.Errorf("%s must be writable by the service, got %q", d, grants[filepath.Join(dir, d)])
		}
	}
	for _, p := range []string{filepath.Join(dir, "backups"), filepath.Join(dir, "tools")} {
		if g, ok := grants[p]; ok {
			t.Errorf("%s must not be granted to the service, got %q", p, g)
		}
	}
	if !adminOnly[filepath.Join(dir, "backups")] || !adminOnly[secret] {
		t.Errorf("backups and the PostgreSQL password must be administrator-only: %v", adminOnly)
	}

	// Without installer-managed PostgreSQL there is no secret file to restrict.
	for _, cmd := range windowsServiceACLPlan(dir, account, "") {
		if strings.HasSuffix(cmd[0], "pg-superuser.secret") {
			t.Error("no secret file, but the plan restricts one")
		}
	}
}

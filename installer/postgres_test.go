package main

import (
	"strings"
	"testing"
)

// pg_dump rejects unknown options, so a psql-only flag (such as -X, which
// v26.10's first E2E run caught) makes every upgrade backup fail.
func TestDumpArgs_OnlyPgDumpOptions(t *testing.T) {
	c := PgConn{Host: "localhost", Port: "5432", User: "ezhealth_user", DB: "ezhealthkonnect"}
	args := dumpArgs(c, "/backups/db.dump")
	allowed := map[string]bool{"-h": true, "-p": true, "-U": true, "-d": true,
		"--no-owner": true, "--format=custom": true, "--file=/backups/db.dump": true}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !allowed[a] {
			t.Fatalf("unexpected pg_dump argument %q in %v", a, args)
		}
		if len(a) == 2 && strings.HasPrefix(a, "-") {
			i++ // skip the option's value
		}
	}
	if !strings.Contains(strings.Join(args, " "), "--format=custom") {
		t.Fatal("backup must use the custom format (pg_restore --list verifies it)")
	}
}

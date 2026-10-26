package main

// postgres_remote.go — administrator access to an EXISTING PostgreSQL server
// (another machine, a managed/cloud service, or a local server addressed by
// host), through the psql client. It lets setup create the application's
// database and user from an administrator login instead of requiring both to
// exist beforehand.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// psqlAdmin implements AdminSQL for any host.
type psqlAdmin struct {
	Psql     string // path to the psql program
	Host     string
	Port     string
	User     string
	Password string
	SSLMode  string // disable | require | verify-ca | verify-full
}

// psqlPath is the psql program inside a PostgreSQL bin directory.
func psqlPath(binDir string) string {
	name := "psql"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(binDir, name)
}

func (a psqlAdmin) args(database, sql string) []string {
	return []string{"-X", "-q", "-t", "-A", "-v", "ON_ERROR_STOP=1",
		"-h", a.Host, "-p", a.Port, "-U", a.User, "-d", database, "-c", sql}
}

// env carries the password and TLS mode; the password never appears on the
// command line.
func (a psqlAdmin) env() []string {
	e := []string{"PGPASSWORD=" + a.Password, "PGCONNECT_TIMEOUT=10"}
	if a.SSLMode != "" {
		e = append(e, "PGSSLMODE="+a.SSLMode)
	}
	return e
}

// Exec implements AdminSQL.
func (a psqlAdmin) Exec(database, sql string) (string, error) {
	cmd := exec.Command(a.Psql, a.args(database, sql)...)
	cmd.Env = append(os.Environ(), a.env()...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

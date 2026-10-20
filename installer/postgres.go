package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// MinPostgresMajor is the oldest PostgreSQL server the application supports.
const MinPostgresMajor = 15

// PgConn is how the application connects to its database.
type PgConn struct {
	Host, Port, DB, User, Password, SSLMode string
}

func (c PgConn) args() []string {
	return []string{"-h", c.Host, "-p", c.Port, "-U", c.User, "-d", c.DB}
}

func (c PgConn) env() []string {
	env := append(os.Environ(), "PGPASSWORD="+c.Password, "PGCONNECT_TIMEOUT=10")
	if c.SSLMode != "" {
		env = append(env, "PGSSLMODE="+c.SSLMode)
	}
	return env
}

// AdminSQL runs SQL as a PostgreSQL superuser on the local server.
type AdminSQL interface {
	Exec(database, sql string) (string, error)
}

// PgTools wraps the PostgreSQL client programs (psql, pg_dump, pg_restore).
type PgTools struct {
	BinDir string // directory holding psql/pg_dump/pg_restore
}

func (t PgTools) bin(name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if t.BinDir == "" {
		return name
	}
	return filepath.Join(t.BinDir, name)
}

func (t PgTools) run(c PgConn, name string, args ...string) (string, error) {
	cmd := exec.Command(t.bin(name), args...)
	cmd.Env = c.env()
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

// Query runs one SQL statement and returns its unaligned output.
func (t PgTools) Query(c PgConn, sql string) (string, error) {
	out, err := t.run(c, "psql", append(c.args(), "-X", "-q", "-t", "-A", "-v", "ON_ERROR_STOP=1", "-c", sql)...)
	if err != nil {
		return "", fmt.Errorf("%s", firstLine(out, err))
	}
	return out, nil
}

// Ping verifies the application's credentials work.
func (t PgTools) Ping(c PgConn) error {
	_, err := t.Query(c, "SELECT 1")
	return err
}

// ServerMajor returns the server's major version (e.g. 17).
func (t PgTools) ServerMajor(c PgConn) (int, error) {
	out, err := t.Query(c, "SHOW server_version_num")
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, fmt.Errorf("unexpected server_version_num %q", out)
	}
	return n / 10000, nil
}

var majorRe = regexp.MustCompile(`(\d+)(\.\d+)?`)

// ClientMajor returns pg_dump's major version (must be >= the server's).
func (t PgTools) ClientMajor() (int, error) {
	out, err := exec.Command(t.bin("pg_dump"), "--version").Output()
	if err != nil {
		return 0, fmt.Errorf("pg_dump not found at %s: %w", t.bin("pg_dump"), err)
	}
	m := majorRe.FindStringSubmatch(string(out))
	if m == nil {
		return 0, fmt.Errorf("cannot read pg_dump version from %q", strings.TrimSpace(string(out)))
	}
	return strconv.Atoi(m[1])
}

// HasTables reports whether the database already contains any table.
func (t PgTools) HasTables(c PgConn) (bool, error) {
	out, err := t.Query(c, "SELECT count(*) FROM pg_catalog.pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema')")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "0", nil
}

// Dump writes a custom-format backup of the database to file.
func (t PgTools) Dump(c PgConn, file string) error {
	out, err := t.run(c, "pg_dump", append(c.args(), "-X", "--format=custom", "--file="+file)...)
	if err != nil {
		return fmt.Errorf("pg_dump: %s", firstLine(out, err))
	}
	return nil
}

// VerifyDump checks a backup is readable and contains the schema history,
// before anything is changed.
func (t PgTools) VerifyDump(file string) error {
	cmd := exec.Command(t.bin("pg_restore"), "--list", file)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("backup %s is not readable: %s", file, firstLine(string(out), err))
	}
	if !strings.Contains(string(out), "flyway_schema_history") {
		return fmt.Errorf("backup %s does not contain the migration history — refusing to continue", file)
	}
	return nil
}

// Restore replaces the database's contents with a backup, all-or-nothing:
// every object owned by the application's role is dropped and the backup is
// replayed in ONE transaction — if anything fails, the database is left
// exactly as it was. Only the application's own objects are touched.
func (t PgTools) Restore(c PgConn, file string) error {
	script, err := os.CreateTemp(filepath.Dir(file), "restore-*.sql")
	if err != nil {
		return err
	}
	defer os.Remove(script.Name())
	if _, err := script.WriteString("DROP OWNED BY CURRENT_USER;\n"); err != nil {
		script.Close()
		return err
	}
	script.Close()

	body := script.Name() + ".body"
	defer os.Remove(body)
	if out, err := exec.Command(t.bin("pg_restore"), "--no-owner", "--file="+body, file).CombinedOutput(); err != nil {
		return fmt.Errorf("read backup: %s", firstLine(string(out), err))
	}
	if err := appendFile(script.Name(), body); err != nil {
		return err
	}
	out, err := t.run(c, "psql", append(c.args(), "-X", "-q", "--single-transaction", "-v", "ON_ERROR_STOP=1", "-f", script.Name())...)
	if err != nil {
		return fmt.Errorf("restore failed and was rolled back: %s", firstLine(out, err))
	}
	return nil
}

func appendFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// ── Administration (local server only) ────────────────────────────────────────

func sqlLiteral(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// EnsureRoleAndDatabase creates (or re-passwords) the login role and creates
// the database owned by it. Callers must have checked the database does not
// already hold data.
func EnsureRoleAndDatabase(admin AdminSQL, dbName, user, password string) error {
	if !identRe.MatchString(dbName) || !identRe.MatchString(user) {
		return fmt.Errorf("invalid database or user name")
	}
	role := fmt.Sprintf(`DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = %[1]s) THEN
    CREATE ROLE %[2]s LOGIN PASSWORD %[3]s;
  ELSE
    ALTER ROLE %[2]s WITH LOGIN PASSWORD %[3]s;
  END IF;
END $$;`, sqlLiteral(user), user, sqlLiteral(password))
	if out, err := admin.Exec("postgres", role); err != nil {
		return fmt.Errorf("create role %s: %s", user, firstLine(out, err))
	}
	exists, err := admin.Exec("postgres", fmt.Sprintf("SELECT 1 FROM pg_database WHERE datname = %s", sqlLiteral(dbName)))
	if err != nil {
		return fmt.Errorf("check database: %s", firstLine(exists, err))
	}
	if strings.TrimSpace(exists) != "1" {
		if out, err := admin.Exec("postgres", fmt.Sprintf("CREATE DATABASE %s OWNER %s TEMPLATE template0 ENCODING 'UTF8'", dbName, user)); err != nil {
			return fmt.Errorf("create database %s: %s", dbName, firstLine(out, err))
		}
	}
	// PostgreSQL 15+: the database owner owns schema public; be explicit for older layouts.
	if out, err := admin.Exec(dbName, fmt.Sprintf("ALTER SCHEMA public OWNER TO %s", user)); err != nil {
		return fmt.Errorf("grant schema: %s", firstLine(out, err))
	}
	return nil
}

// DropRoleAndDatabase removes the application's database and role (uninstall).
func DropRoleAndDatabase(admin AdminSQL, dbName, user string) error {
	if !identRe.MatchString(dbName) || !identRe.MatchString(user) {
		return fmt.Errorf("invalid database or user name")
	}
	stmts := []string{
		fmt.Sprintf("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = %s AND pid <> pg_backend_pid()", sqlLiteral(dbName)),
		fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName),
		fmt.Sprintf("DROP ROLE IF EXISTS %s", user),
	}
	for _, s := range stmts {
		if out, err := admin.Exec("postgres", s); err != nil {
			return fmt.Errorf("%s: %s", s, firstLine(out, err))
		}
	}
	return nil
}

func firstLine(out string, err error) string {
	out = strings.TrimSpace(out)
	if out == "" {
		if err != nil {
			return err.Error()
		}
		return ""
	}
	// psql/pg_* print the real problem on the first ERROR/FATAL line.
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "ERROR") || strings.Contains(l, "FATAL") || strings.Contains(l, "error") {
			return strings.TrimSpace(l)
		}
	}
	return strings.Split(out, "\n")[0]
}

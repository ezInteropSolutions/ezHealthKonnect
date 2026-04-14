package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// makeTestZip creates a zip at a temp path with the given name→content entries.
func makeTestZip(t *testing.T, entries map[string]string) string {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "test.zip")
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for name, content := range entries {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		fw.Write([]byte(content)) //nolint:errcheck
	}
	w.Close()
	f.Close()
	return tmp
}

// TestExtractZip_StripRoot verifies that the root wrapper dir is removed.
func TestExtractZip_StripRoot(t *testing.T) {
	zf := makeTestZip(t, map[string]string{
		"ezhealthkonnect-v1.0.0/app.js":    "// app",
		"ezhealthkonnect-v1.0.0/server.js": "// server",
	})
	dest := t.TempDir()
	if err := extractZip(zf, dest, true); err != nil {
		t.Fatalf("extractZip: %v", err)
	}
	// File should land at dest/app.js, NOT dest/ezhealthkonnect-v1.0.0/app.js
	if _, err := os.Stat(filepath.Join(dest, "app.js")); err != nil {
		t.Error("app.js missing after strip root:", err)
	}
	// The wrapper dir itself must not exist
	if _, err := os.Stat(filepath.Join(dest, "ezhealthkonnect-v1.0.0")); err == nil {
		t.Error("root wrapper dir should not exist after stripRoot=true")
	}
}

// TestExtractZip_NestedMigrations verifies deep dirs (database/migrations/) are created.
func TestExtractZip_NestedMigrations(t *testing.T) {
	zf := makeTestZip(t, map[string]string{
		"root/database/migrations/V1__init.sql":  "CREATE TABLE test;",
		"root/database/migrations/V10__more.sql": "ALTER TABLE test ADD COLUMN x INT;",
		"root/app.js":                            "// app",
	})
	dest := t.TempDir()
	if err := extractZip(zf, dest, true); err != nil {
		t.Fatalf("extractZip: %v", err)
	}
	for _, f := range []string{
		"database/migrations/V1__init.sql",
		"database/migrations/V10__more.sql",
		"app.js",
	} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(f))); err != nil {
			t.Errorf("expected %s: %v", f, err)
		}
	}
}

// TestExtractZip_BackslashNames verifies Compress-Archive backslash paths are normalised.
func TestExtractZip_BackslashNames(t *testing.T) {
	// Simulate a zip built by PowerShell Compress-Archive that uses backslashes
	tmp := filepath.Join(t.TempDir(), "bs.zip")
	f, _ := os.Create(tmp)
	w := zip.NewWriter(f)
	// Use raw backslash entry names (as PowerShell emits)
	fw, _ := w.Create(`root\database\migrations\V1__init.sql`)
	fw.Write([]byte("CREATE TABLE x;")) //nolint:errcheck
	w.Close()
	f.Close()

	dest := t.TempDir()
	if err := extractZip(tmp, dest, true); err != nil {
		t.Fatalf("extractZip: %v", err)
	}
	target := filepath.Join(dest, "database", "migrations", "V1__init.sql")
	if _, err := os.Stat(target); err != nil {
		t.Errorf("backslash path not normalised correctly, file missing: %v", err)
	}
}

// TestExtractZip_NoStripRoot verifies behaviour when stripRoot=false.
func TestExtractZip_NoStripRoot(t *testing.T) {
	zf := makeTestZip(t, map[string]string{
		"app.js": "// app",
	})
	dest := t.TempDir()
	if err := extractZip(zf, dest, false); err != nil {
		t.Fatalf("extractZip: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "app.js")); err != nil {
		t.Error("app.js missing:", err)
	}
}

// TestExtractZip_FileContents verifies file content is written correctly.
func TestExtractZip_FileContents(t *testing.T) {
	const want = "SELECT 1;"
	zf := makeTestZip(t, map[string]string{"root/check.sql": want})
	dest := t.TempDir()
	extractZip(zf, dest, true) //nolint:errcheck
	got, err := os.ReadFile(filepath.Join(dest, "check.sql"))
	if err != nil {
		t.Fatal("check.sql not found:", err)
	}
	if string(got) != want {
		t.Errorf("content mismatch: got %q want %q", got, want)
	}
}

// TestExtractZip_FileBlockingDir verifies that a regular file sitting where a
// directory component should be is automatically removed and extraction succeeds.
func TestExtractZip_FileBlockingDir(t *testing.T) {
	dest := t.TempDir()
	// Pre-create a FILE named "database" — blocks creation of database/migrations/
	if err := os.WriteFile(filepath.Join(dest, "database"), []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}
	zf := makeTestZip(t, map[string]string{
		"root/database/migrations/V1.sql": "new content",
	})
	if err := extractZip(zf, dest, true); err != nil {
		t.Fatalf("expected auto-recovery, got error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "database", "migrations", "V1.sql"))
	if err != nil {
		t.Fatal("V1.sql not extracted:", err)
	}
	if string(got) != "new content" {
		t.Errorf("content mismatch: %q", got)
	}
}

// TestExtractZip_PowerShellDirEntry simulates PowerShell Compress-Archive directory entries
// that lack the zip directory attribute (IsDir() == false) but have a trailing slash.
// On reinstall these would collide with an existing directory — must be skipped, not opened as files.
func TestExtractZip_PowerShellDirEntry(t *testing.T) {
	// Build a zip where "database/" entry has no directory attribute set (as PowerShell does).
	tmp := filepath.Join(t.TempDir(), "ps.zip")
	f, _ := os.Create(tmp)
	w := zip.NewWriter(f)
	// Directory entry with trailing slash but no directory attribute
	w.Create("root/database/")                                          //nolint:errcheck
	fw, _ := w.Create("root/database/migrations/V1__init.sql")
	fw.Write([]byte("CREATE TABLE x;")) //nolint:errcheck
	w.Close()
	f.Close()

	dest := t.TempDir()
	// Pre-existing directory from a previous install run
	os.MkdirAll(filepath.Join(dest, "database", "migrations"), 0755) //nolint:errcheck

	if err := extractZip(tmp, dest, true); err != nil {
		t.Fatalf("extractZip with PowerShell dir entry: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "database", "migrations", "V1__init.sql")); err != nil {
		t.Error("migration file not extracted:", err)
	}
}

// TestExtractZip_ReinstallOverPartialRun simulates re-running the installer over a previous
// failed extraction (files already exist at destination). Should overwrite cleanly.
func TestExtractZip_ReinstallOverPartialRun(t *testing.T) {
	dest := t.TempDir()
	// Simulate partial previous run: database dir exists but is incomplete
	os.MkdirAll(filepath.Join(dest, "database", "migrations"), 0755) //nolint:errcheck
	os.WriteFile(filepath.Join(dest, "database", "migrations", "V1__init.sql"), []byte("old"), 0644) //nolint:errcheck
	os.WriteFile(filepath.Join(dest, "app.js"), []byte("old"), 0644) //nolint:errcheck

	zf := makeTestZip(t, map[string]string{
		"root/app.js":                            "new",
		"root/database/migrations/V1__init.sql":  "new content",
		"root/database/migrations/V10__more.sql": "also new",
	})
	if err := extractZip(zf, dest, true); err != nil {
		t.Fatalf("extractZip over existing dir: %v", err)
	}
	// Files should be overwritten
	got, _ := os.ReadFile(filepath.Join(dest, "app.js"))
	if string(got) != "new" {
		t.Errorf("app.js not overwritten: got %q", got)
	}
	got, _ = os.ReadFile(filepath.Join(dest, "database", "migrations", "V1__init.sql"))
	if string(got) != "new content" {
		t.Errorf("V1.sql not overwritten: got %q", got)
	}
}

package migrator

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Version is a Flyway migration version ("21", "1.1"), compared numerically
// part by part.
type Version struct {
	text  string
	parts []int64
}

// ParseVersion parses a Flyway version; "_" and "." both separate parts.
func ParseVersion(s string) (Version, error) {
	norm := strings.ReplaceAll(s, "_", ".")
	if norm == "" {
		return Version{}, fmt.Errorf("empty version")
	}
	var parts []int64
	for _, p := range strings.Split(norm, ".") {
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil || n < 0 {
			return Version{}, fmt.Errorf("invalid version %q", s)
		}
		parts = append(parts, n)
	}
	return Version{text: norm, parts: parts}, nil
}

func (v Version) String() string { return v.text }

// Compare returns -1, 0 or 1. Trailing zero parts are insignificant (1.0 == 1).
func (v Version) Compare(o Version) int {
	n := len(v.parts)
	if len(o.parts) > n {
		n = len(o.parts)
	}
	for i := 0; i < n; i++ {
		var a, b int64
		if i < len(v.parts) {
			a = v.parts[i]
		}
		if i < len(o.parts) {
			b = o.parts[i]
		}
		if a != b {
			if a < b {
				return -1
			}
			return 1
		}
	}
	return 0
}

// key is a canonical form for map lookups (trailing zeros removed).
func (v Version) key() string {
	parts := v.parts
	for len(parts) > 1 && parts[len(parts)-1] == 0 {
		parts = parts[:len(parts)-1]
	}
	s := make([]string, len(parts))
	for i, p := range parts {
		s[i] = strconv.FormatInt(p, 10)
	}
	return strings.Join(s, ".")
}

// Migration is one versioned SQL migration file.
type Migration struct {
	Version     Version
	Description string
	Script      string // file name, e.g. V21__Fix_Output_Table_Schema.sql
	Checksum    int32
	SQL         string
	Analysis    Script
}

// Source provides the migrations to apply.
type Source interface {
	Load() ([]Migration, error)
}

// FSSource loads migrations from a directory in a file system. Only *.sql
// files are considered; every one must be a versioned migration
// (V<version>__<description>.sql) — anything else is an error rather than
// silently skipped, so a misnamed migration can never be missed.
type FSSource struct {
	FS  fs.FS
	Dir string
}

// Load reads, parses and sorts the migrations.
func (s FSSource) Load() ([]Migration, error) {
	entries, err := fs.ReadDir(s.FS, s.Dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations directory %q: %w", s.Dir, err)
	}
	var out []Migration
	seen := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(name), ".sql") {
			continue
		}
		version, desc, err := parseFileName(name)
		if err != nil {
			return nil, err
		}
		if other, dup := seen[version.key()]; dup {
			return nil, fmt.Errorf("duplicate migration version %s: %s and %s", version, other, name)
		}
		seen[version.key()] = name

		data, err := fs.ReadFile(s.FS, path.Join(s.Dir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		sum, err := Checksum(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("checksum %s: %w", name, err)
		}
		sql := strings.TrimPrefix(string(data), "\uFEFF")
		out = append(out, Migration{
			Version:     version,
			Description: desc,
			Script:      name,
			Checksum:    sum,
			SQL:         sql,
			Analysis:    AnalyzeScript(sql),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version.Compare(out[j].Version) < 0 })
	return out, nil
}

// parseFileName parses "V<version>__<description>.sql".
func parseFileName(name string) (Version, string, error) {
	base := name[:len(name)-len(".sql")]
	if !strings.HasPrefix(base, "V") {
		return Version{}, "", fmt.Errorf("unsupported migration file %q: only versioned V<version>__<description>.sql files are supported", name)
	}
	vPart, dPart, ok := strings.Cut(base[1:], "__")
	if !ok {
		return Version{}, "", fmt.Errorf("invalid migration file name %q: expected V<version>__<description>.sql", name)
	}
	v, err := ParseVersion(vPart)
	if err != nil {
		return Version{}, "", fmt.Errorf("invalid migration file name %q: %w", name, err)
	}
	return v, strings.ReplaceAll(dPart, "_", " "), nil
}

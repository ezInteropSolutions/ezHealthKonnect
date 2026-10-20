package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// alwaysProtected is customer data the installer never moves or deletes,
// regardless of what a bundle claims (defence in depth on top of the
// bundle's own "protected" list).
var alwaysProtected = []string{".env", stateFileName, "logs", "storage", "schemas", "schema-dist",
	"uploads", "backups", "git-repos", "cache", "tools"}

// BundleInfo is bundle-manifest.json written by cmd/bundle-builder.
type BundleInfo struct {
	Version     string   `json:"version"`
	OS          string   `json:"os"`
	Arch        string   `json:"arch"`
	NodeVersion string   `json:"nodeVersion"`
	Entries     []string `json:"entries"`
	Protected   []string `json:"protected"`
}

// BundleSource yields the application bundle zip.
type BundleSource interface {
	// Path returns a local zip path and a cleanup function.
	Path() (string, func(), error)
	Describe() string
}

// EmbeddedBundle is a bundle compiled into the installer (release builds).
type EmbeddedBundle struct{ Data []byte }

// Path implements BundleSource.
func (e EmbeddedBundle) Path() (string, func(), error) {
	f, err := os.CreateTemp("", "ezhk-bundle-*.zip")
	if err != nil {
		return "", nil, err
	}
	if _, err := f.Write(e.Data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", nil, err
	}
	f.Close()
	return f.Name(), func() { os.Remove(f.Name()) }, nil
}

// Describe implements BundleSource.
func (EmbeddedBundle) Describe() string { return "bundle included in this installer" }

// FileBundle is a bundle zip on disk (offline media, tests, CI).
type FileBundle struct{ File string }

// Path implements BundleSource.
func (f FileBundle) Path() (string, func(), error) {
	if _, err := os.Stat(f.File); err != nil {
		return "", nil, err
	}
	return f.File, func() {}, nil
}

// Describe implements BundleSource.
func (f FileBundle) Describe() string { return f.File }

// StagedBundle is a bundle extracted next to the installation, ready to swap in.
type StagedBundle struct {
	Dir  string
	Info BundleInfo
}

// StageBundle extracts the bundle into installDir/.staging-* and validates it.
func StageBundle(src BundleSource, installDir, wantOS string) (*StagedBundle, error) {
	zipPath, cleanup, err := src.Path()
	if err != nil {
		return nil, fmt.Errorf("open bundle: %w", err)
	}
	defer cleanup()
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(installDir, ".staging-")
	if err != nil {
		return nil, err
	}
	if err := extractZip(zipPath, dir, true); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("extract bundle: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "bundle-manifest.json"))
	if err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("bundle has no bundle-manifest.json — not an ezHealthKonnect bundle")
	}
	var info BundleInfo
	if err := json.Unmarshal(data, &info); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("bundle-manifest.json: %w", err)
	}
	if err := info.validate(dir, wantOS); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	return &StagedBundle{Dir: dir, Info: info}, nil
}

func (b BundleInfo) validate(dir, wantOS string) error {
	if b.Version == "" || len(b.Entries) == 0 {
		return fmt.Errorf("bundle manifest is incomplete")
	}
	if wantOS != "" && b.OS != wantOS {
		return fmt.Errorf("this bundle is for %s, not %s", b.OS, wantOS)
	}
	prot := protectedSet(b.Protected)
	for _, e := range b.Entries {
		if prot[e] || e == "" || strings.ContainsAny(e, `/\`) || e == "." || e == ".." {
			return fmt.Errorf("bundle entry %q is not allowed", e)
		}
		if _, err := os.Stat(filepath.Join(dir, e)); err != nil {
			return fmt.Errorf("bundle is missing %s", e)
		}
	}
	return nil
}

// Remove deletes the staging directory.
func (s *StagedBundle) Remove() { os.RemoveAll(s.Dir) }

func protectedSet(extra []string) map[string]bool {
	m := map[string]bool{}
	for _, p := range alwaysProtected {
		m[p] = true
	}
	for _, p := range extra {
		m[p] = true
	}
	return m
}

// moveEntries renames each top-level entry from src to dst. Both must be on
// the same volume (they are: staging and backups live inside the install
// directory), so each move is a fast, atomic rename. Protected entries are
// refused outright.
func moveEntries(src, dst string, entries []string) error {
	prot := protectedSet(nil)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if prot[e] {
			return fmt.Errorf("refusing to move protected entry %q", e)
		}
		from := filepath.Join(src, e)
		if _, err := os.Lstat(from); os.IsNotExist(err) {
			continue // e.g. an entry removed by hand; nothing to move
		}
		to := filepath.Join(dst, e)
		if _, err := os.Lstat(to); err == nil {
			return fmt.Errorf("cannot move %s: %s already exists", e, to)
		}
		if err := os.Rename(from, to); err != nil {
			return fmt.Errorf("move %s: %w", e, err)
		}
	}
	return nil
}

// ── zip extraction ────────────────────────────────────────────────────────────

// archivePath validates an archive member name: no absolute paths and no ".."
// segments (zip-slip). Backslashes from Windows-made archives count as separators.
func archivePath(name string) (string, error) {
	n := strings.ReplaceAll(name, "\\", "/")
	if n == "" || strings.HasPrefix(n, "/") || (len(n) > 1 && n[1] == ':') {
		return "", fmt.Errorf("unsafe path %q in archive", name)
	}
	for _, seg := range strings.Split(n, "/") {
		if seg == ".." {
			return "", fmt.Errorf("unsafe path %q in archive", name)
		}
	}
	return path.Clean(n), nil
}

// extractZip extracts src into dest, optionally stripping the single root folder.
func extractZip(src, dest string, stripRoot bool) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		isDir := f.FileInfo().IsDir() || strings.HasSuffix(name, "/")
		if stripRoot {
			idx := strings.Index(name, "/")
			if idx < 0 {
				continue
			}
			name = name[idx+1:]
		}
		name = strings.TrimSuffix(name, "/")
		if name == "" {
			continue
		}
		rel, err := archivePath(name)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.FromSlash(rel))

		if isDir {
			if err := mkdirAllClear(target); err != nil {
				return fmt.Errorf("create dir %s: %w", target, err)
			}
			continue
		}
		if err := mkdirAllClear(filepath.Dir(target)); err != nil {
			return fmt.Errorf("create parent dir for %s: %w", target, err)
		}
		mode := f.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode|0o600)
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// mkdirAllClear creates dirPath, removing a regular file that sits where a
// directory component must be (left over from an interrupted run).
func mkdirAllClear(dirPath string) error {
	if err := os.MkdirAll(dirPath, 0o755); err == nil {
		return nil
	}
	volume := filepath.VolumeName(dirPath)
	rest := filepath.ToSlash(dirPath[len(volume):])
	cur := volume + string(filepath.Separator)
	for _, part := range strings.Split(strings.Trim(rest, "/"), "/") {
		if part == "" {
			continue
		}
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err != nil {
			break
		}
		if !fi.IsDir() {
			if err := os.Remove(cur); err != nil {
				return fmt.Errorf("remove blocking file %s: %w", cur, err)
			}
			break
		}
	}
	return os.MkdirAll(dirPath, 0o755)
}

// peekBundleInfo reads bundle-manifest.json from the bundle zip without
// extracting anything else.
func peekBundleInfo(src BundleSource) (*BundleInfo, error) {
	zipPath, cleanup, err := src.Path()
	if err != nil {
		return nil, err
	}
	defer cleanup()
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	for _, f := range r.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if i := strings.Index(name, "/"); i >= 0 && name[i+1:] == "bundle-manifest.json" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			var info BundleInfo
			if err := json.NewDecoder(rc).Decode(&info); err != nil {
				return nil, err
			}
			return &info, nil
		}
	}
	return nil, fmt.Errorf("bundle has no bundle-manifest.json")
}

// sortedCopy returns a sorted copy of s.
func sortedCopy(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

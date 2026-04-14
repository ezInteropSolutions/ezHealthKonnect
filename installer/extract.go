package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// mkdirAllClear creates dirPath and all parents, removing any regular file that
// sits where a directory component should be (left over from a previous failed install).
func mkdirAllClear(dirPath string) error {
	if err := os.MkdirAll(dirPath, 0755); err == nil {
		return nil
	}
	// Walk each component from root to leaf.
	// On Windows: volume = "C:", rest = "\foo\bar". On Linux: volume = "", rest = "/foo/bar".
	volume := filepath.VolumeName(dirPath)
	rest := filepath.ToSlash(dirPath[len(volume):])
	parts := strings.Split(strings.Trim(rest, "/"), "/")

	cur := volume + string(filepath.Separator)
	for _, part := range parts {
		if part == "" {
			continue
		}
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err != nil {
			break // component doesn't exist yet — MkdirAll will create it
		}
		if !fi.IsDir() {
			// A regular file is sitting where a directory must be — remove it.
			if removeErr := os.Remove(cur); removeErr != nil {
				return fmt.Errorf("remove blocking file %s: %w", cur, removeErr)
			}
			break
		}
	}
	return os.MkdirAll(dirPath, 0755)
}

func extractZip(src, dest string, stripRoot bool) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		// Normalize backslashes (Compress-Archive on Windows uses \ instead of /)
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if stripRoot {
			// Remove first path component
			idx := strings.Index(name, "/")
			if idx < 0 {
				continue
			}
			name = name[idx+1:]
		}
		if name == "" {
			continue
		}

		target := filepath.Join(dest, filepath.FromSlash(name))

		// Treat as directory if the zip metadata says so OR if the normalised
		// name has a trailing slash — PowerShell Compress-Archive omits the
		// directory attribute flag, so IsDir() alone is not reliable.
		if f.FileInfo().IsDir() || strings.HasSuffix(name, "/") {
			if err := mkdirAllClear(target); err != nil {
				return fmt.Errorf("create dir %s: %w", target, err)
			}
			continue
		}

		if err := mkdirAllClear(filepath.Dir(target)); err != nil {
			return fmt.Errorf("create parent dir for %s: %w", target, err)
		}

		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
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
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

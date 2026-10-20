package main

import (
	"os"
	"path/filepath"
	"strings"
)

// keepInstallerCopy copies the running installer into <dir>/tools so that
// upgrade, rollback and uninstall keep working after the downloaded file is
// deleted.
func keepInstallerCopy(dir, name string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	dst := filepath.Join(dir, "tools", name)
	if sameFile(self, dst) {
		return nil
	}
	if err := copyFileTo(self, dst); err != nil {
		return err
	}
	return os.Chmod(dst, 0o700)
}

func sameFile(a, b string) bool {
	ai, err1 := os.Stat(a)
	bi, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(ai, bi)
}

// removeInstallDir deletes the installation. When the running installer is
// itself inside it (uninstall launched from Add/Remove Programs), everything
// else is removed now and the rest once this process exits.
func removeInstallDir(dir string) error {
	self, _ := os.Executable()
	inside := self != "" && strings.HasPrefix(strings.ToLower(filepath.Clean(self)), strings.ToLower(filepath.Clean(dir))+string(filepath.Separator))
	if !inside {
		return os.RemoveAll(dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == "tools" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return scheduleRemoval(dir)
}

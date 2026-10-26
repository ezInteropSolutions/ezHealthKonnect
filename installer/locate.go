package main

// locate.go — finding an existing installation.
//
// The wizard, Add/Remove Programs and the headless commands all need to know
// WHERE ezHealthKonnect is installed. Looking only in the default folder
// meant an install made into any other folder was invisible: no Manage or
// Uninstall screen, and the Add/Remove "Uninstall" entry opened a fresh
// install instead. The location is therefore taken from, in order:
//
//  1. the folder this very program lives in, when it is the copy kept in
//     <install>/tools (what Add/Remove Programs launches);
//  2. what the operating system recorded: the Add/Remove Programs entry and
//     the service definition (registeredInstallDirs, per platform);
//  3. the default folder.
//
// A location only counts when it holds install-state.json, so a stale
// registry value or an unrelated folder is never mistaken for an install.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// hasInstallState reports whether dir holds an ezHealthKonnect install record.
func hasInstallState(dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	_, found, err := LoadState(dir)
	return found && err == nil
}

var programRe = regexp.MustCompile(`(?i)^.*?\.(?:exe|bat|cmd|ps1|sh)\b`)

// programPath returns the program from a service ImagePath, which may be
// quoted and followed by arguments.
func programPath(imagePath string) string {
	s := strings.TrimSpace(imagePath)
	if strings.HasPrefix(s, `"`) {
		if end := strings.Index(s[1:], `"`); end >= 0 {
			return s[1 : 1+end]
		}
		return strings.Trim(s, `"`)
	}
	if m := programRe.FindString(s); m != "" {
		return m
	}
	return s
}

// installRootFor walks up from a file or directory inside an installation (a
// service binary, a unit's working directory) to the install directory.
func installRootFor(path string) string {
	p := filepath.Clean(strings.TrimSpace(path))
	for i := 0; i < 4 && p != "." && p != ""; i++ {
		if hasInstallState(p) {
			return p
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	return ""
}

// ownInstallDir is the installation this program belongs to when it is the
// copy kept in <install>/tools.
func ownInstallDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	tools := filepath.Dir(exe)
	if !strings.EqualFold(filepath.Base(tools), "tools") {
		return ""
	}
	if dir := filepath.Dir(tools); hasInstallState(dir) {
		return dir
	}
	return ""
}

// findInstall returns the first candidate that holds an installation, or
// fallback when none does.
func findInstall(candidates []string, fallback string) string {
	for _, c := range candidates {
		if hasInstallState(c) {
			return filepath.Clean(c)
		}
	}
	return fallback
}

// installCandidates lists where an existing installation may be, best first.
func installCandidates() []string {
	var out []string
	if d := ownInstallDir(); d != "" {
		out = append(out, d)
	}
	return append(out, registeredInstallDirs()...)
}

// parseRegValue extracts a string value from `reg query <key> /v <name>` output.
func parseRegValue(out, name string) string {
	re := regexp.MustCompile(`(?im)^\s*` + regexp.QuoteMeta(name) + `\s+REG_(?:EXPAND_)?SZ\s+(.*?)\s*$`)
	if m := re.FindStringSubmatch(out); m != nil {
		return m[1]
	}
	return ""
}

// parseUnitValue extracts KEY=value from a systemd unit file.
func parseUnitValue(unit, key string) string {
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(key) + `\s*=\s*(.*?)\s*$`)
	if m := re.FindStringSubmatch(unit); m != nil {
		return m[1]
	}
	return ""
}

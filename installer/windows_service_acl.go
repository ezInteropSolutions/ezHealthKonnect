package main

import "path/filepath"

// Windows service account and file access (v26.10).
//
// The service runs as the virtual account NT SERVICE\<service id>. Windows
// creates it together with the service; it has no password, cannot sign in
// interactively and holds only the access granted here. This mirrors the
// Linux installation (an unprivileged ezhealthkonnect user):
//
//   - application files: read and execute — the service cannot modify its code;
//   - data directories the application writes: modify;
//   - backups, and the stored PostgreSQL superuser password: no access at all.
//
// The plan is OS-independent data so it is unit-tested everywhere; the
// Windows platform runs it with icacls after the service exists (the virtual
// account only resolves once its service is registered).

const (
	sidSystem         = "*S-1-5-18"     // LocalSystem
	sidAdministrators = "*S-1-5-32-544" // BUILTIN\Administrators
)

// serviceAccountName is the virtual account a Windows service with this id runs as.
func serviceAccountName(serviceID string) string { return `NT SERVICE\` + serviceID }

// serviceWritableDirs are the data directories the running application writes.
// "backups" (written only by the installer) and "tools" (service wrapper and,
// for installer-managed PostgreSQL, the superuser password) are excluded.
func serviceWritableDirs() []string {
	var out []string
	for _, d := range dataDirs {
		if d != "backups" && d != "tools" {
			out = append(out, d)
		}
	}
	return out
}

// windowsServiceACLPlan returns the icacls invocations that give the service
// account exactly the access described above. secretFile, when non-empty, is
// a file under the install directory restricted to SYSTEM and Administrators.
func windowsServiceACLPlan(dir, account, secretFile string) [][]string {
	adminOnly := func(path, inherit string) []string {
		return []string{path, "/inheritance:r",
			"/grant:r", sidSystem + ":" + inherit + "F",
			"/grant:r", sidAdministrators + ":" + inherit + "F", "/Q"}
	}
	plan := [][]string{{dir, "/grant:r", account + ":(OI)(CI)RX", "/Q"}}
	for _, d := range serviceWritableDirs() {
		plan = append(plan, []string{filepath.Join(dir, d), "/grant:r", account + ":(OI)(CI)M", "/Q"})
	}
	plan = append(plan, adminOnly(filepath.Join(dir, "backups"), "(OI)(CI)"))
	if secretFile != "" {
		plan = append(plan, adminOnly(secretFile, ""))
	}
	return plan
}

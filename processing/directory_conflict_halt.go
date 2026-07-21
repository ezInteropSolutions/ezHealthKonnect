// processing/directory_conflict_halt.go
// PHI-safety hard-block for file_listener interfaces watching overlapping
// directory + pattern combinations — the file-based analog of the port-conflict
// halt in engine.go. Two listeners racing over the same files risk double
// processing (both claim the file) or a partial read (one deletes/moves it out
// from under the other mid-read), the same ambiguous-PHI-routing risk a port
// conflict poses, just via the filesystem instead of a socket.

package processing

import (
	"encoding/json"
	"fmt"
	"log"
	"path"
	"strings"

	. "ezhealthkonnect/services/connectors" // for ExtractFileListenerDirConfig — matches engine.go's import style
)

// normalizeDirPath returns a canonical form of a container POSIX directory path
// for equality/ancestor comparisons. Uses the OS-agnostic "path" package (not
// "path/filepath") since directory_path config values are always container-POSIX
// strings regardless of what OS the Go toolchain happens to run on — filepath.Clean
// would silently mangle them with backslashes if this ever runs natively on Windows.
func normalizeDirPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	return path.Clean(p)
}

// isAncestorDir reports whether child is parent itself or nested under it.
// Purely lexical (matches file_listener.go's existing isWithinDir helper) —
// symlinks to the same physical directory under a different path are not resolved.
// Both arguments must already be normalized.
func isAncestorDir(parent, child string) bool {
	if parent == "" || child == "" {
		return false
	}
	if parent == child {
		return true
	}
	return strings.HasPrefix(child, parent+"/")
}

// isCatchAllPattern reports whether a file_listener glob pattern matches
// effectively any file.
func isCatchAllPattern(pattern string) bool {
	return pattern == "*" || pattern == "*.*"
}

// dirListenersConflict evaluates whether two file_listener directory/pattern
// configurations conflict, per the two rules documented on
// findFileListenerConflictsLocked. dirA/dirB must already be normalized (see
// normalizeDirPath). Exposed as a standalone pure function — used by the real
// activation-time scan below and independently unit-testable without a DB.
//
// Two rules, both hard-block:
//  1. Exact match: same normalized directory AND overlapping pattern (identical,
//     or either side is a catch-all glob like "*"/"*.*").
//  2. Ancestor match: one directory is nested under the other AND the ANCESTOR
//     side's listener is recursive (only then does it actually reach into the
//     descendant's files — a non-recursive parent's Glob never descends into
//     subdirectories). Deliberately has no pattern-overlap gate: an ancestor
//     listener reads whatever the descendant's writer drops regardless of what
//     pattern the descendant interface configured for itself.
func dirListenersConflict(dirA, patternA string, recursiveA bool, dirB, patternB string, recursiveB bool) (conflict bool, description string) {
	if dirA == dirB && (patternA == patternB || isCatchAllPattern(patternA) || isCatchAllPattern(patternB)) {
		return true, fmt.Sprintf("same directory (%s) with an overlapping pattern (%s vs %s)", dirA, patternA, patternB)
	}
	if isAncestorDir(dirA, dirB) && recursiveA {
		return true, fmt.Sprintf("%s recursively sweeps subdirectory %s", dirA, dirB)
	}
	if isAncestorDir(dirB, dirA) && recursiveB {
		return true, fmt.Sprintf("%s recursively sweeps subdirectory %s", dirB, dirA)
	}
	return false, ""
}

// findFileListenerConflictsLocked scans every OTHER currently-active interface's
// file_listener connector.inbound step for a directory/pattern overlap with the
// candidate (dirPath, pattern, recursive) about to be activated as interfaceID,
// via dirListenersConflict. Must be called while pe.mutex is held (reads
// pe.activeInterfaces).
func (pe *ProcessingEngine) findFileListenerConflictsLocked(interfaceID, dirPath, pattern string, recursive bool) (conflictingIDs []string, reason string) {
	normDir := normalizeDirPath(dirPath)

	activeIDs := make([]string, 0, len(pe.activeInterfaces))
	for id := range pe.activeInterfaces {
		if id == interfaceID {
			continue
		}
		activeIDs = append(activeIDs, id)
	}

	var reasons []string
	for _, ifaceID := range activeIDs {
		rows, err := pe.db.Query(`
			SELECT ts.config::text
			FROM transformation_steps ts
			JOIN transformation_pipelines tp ON tp.id = ts.pipeline_id
			WHERE tp.interface_id = $1
			  AND ts.step_type = 'connector.inbound'
			  AND ts.enabled = true
		`, ifaceID)
		if err != nil {
			continue
		}
		for rows.Next() {
			var cfgJSON string
			if rows.Scan(&cfgJSON) != nil {
				continue
			}
			var wrapper struct {
				ConnectorType string                 `json:"connectorType"`
				Config        map[string]interface{} `json:"config"`
			}
			if json.Unmarshal([]byte(cfgJSON), &wrapper) != nil {
				continue
			}
			if MapLegacyConnectorType(wrapper.ConnectorType, "inbound") != "file_listener" {
				continue
			}
			otherDir, otherPattern, otherRecursive, ok := ExtractFileListenerDirConfig(wrapper.Config)
			if !ok {
				continue
			}
			normOther := normalizeDirPath(otherDir)

			if conflict, desc := dirListenersConflict(normDir, pattern, recursive, normOther, otherPattern, otherRecursive); conflict {
				conflictingIDs = append(conflictingIDs, ifaceID)
				reasons = append(reasons, fmt.Sprintf("interface %s: %s", ifaceID, desc))
				break
			}
		}
		rows.Close()
	}

	if len(conflictingIDs) == 0 {
		return nil, ""
	}
	return conflictingIDs, fmt.Sprintf(
		"PHI_SAFETY_HALT: directory conflict on %s (pattern %s) — ambiguous PHI routing: %s. "+
			"Correct the directory/pattern configuration and re-activate manually.",
		dirPath, pattern, strings.Join(reasons, "; "),
	)
}

// haltOnDirectoryConflictLocked stops every connector involved in a file_listener
// directory/pattern conflict and marks all affected interfaces as 'error'. It MUST
// be called while pe.mutex is already held (e.g. from inside ActivateInterface).
// newInterfaceID is always included in the affected set — even though it hasn't
// finished activating, an earlier connector.inbound step on the SAME interface may
// have already started this activation call and needs tearing down too (exactly
// how haltOnPortConflictLocked already handles this).
func (pe *ProcessingEngine) haltOnDirectoryConflictLocked(newInterfaceID, dirPath, pattern, reason string, conflictingIDs []string) {
	affected := make(map[string]bool)
	affected[newInterfaceID] = true
	for _, id := range conflictingIDs {
		affected[id] = true
	}

	affectedList := make([]string, 0, len(affected))
	for id := range affected {
		affectedList = append(affectedList, id)
	}
	log.Printf("🚨 [HIPAA] Halting %d interface(s) due to directory conflict on %s: %v", len(affected), dirPath, affectedList)

	pe.stopAndClearInterfacesLocked(affected)

	// DB updates and HIPAA audit are I/O-bound — run asynchronously so we don't
	// hold the engine mutex during database writes.
	go pe.persistConflictHalt(affectedList, reason, "directory_conflict_ambiguous_routing", map[string]interface{}{
		"directory": dirPath,
		"pattern":   pattern,
	})
}

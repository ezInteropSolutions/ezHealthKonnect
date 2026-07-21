// processing/directory_conflict_test.go
// Unit tests for the PHI Safety file_listener directory/pattern conflict helpers.

package processing

import (
	"testing"

	"ezhealthkonnect/services/connectors"
)

// ─── normalizeDirPath ────────────────────────────────────────────────────────

func TestNormalizeDirPath_Basic(t *testing.T) {
	if got := normalizeDirPath("/data/ccd/inbox"); got != "/data/ccd/inbox" {
		t.Errorf("expected '/data/ccd/inbox', got %q", got)
	}
}

func TestNormalizeDirPath_TrailingSlash(t *testing.T) {
	if got := normalizeDirPath("/data/ccd/inbox/"); got != "/data/ccd/inbox" {
		t.Errorf("expected trailing slash stripped, got %q", got)
	}
}

func TestNormalizeDirPath_Whitespace(t *testing.T) {
	if got := normalizeDirPath("  /data/ccd/inbox  "); got != "/data/ccd/inbox" {
		t.Errorf("expected whitespace trimmed, got %q", got)
	}
}

func TestNormalizeDirPath_Empty(t *testing.T) {
	if got := normalizeDirPath(""); got != "" {
		t.Errorf("expected empty string to stay empty, got %q", got)
	}
}

// ─── isAncestorDir ───────────────────────────────────────────────────────────

func TestIsAncestorDir_SameDir(t *testing.T) {
	if !isAncestorDir("/data/ccd/inbox", "/data/ccd/inbox") {
		t.Error("expected a directory to be its own ancestor")
	}
}

func TestIsAncestorDir_Child(t *testing.T) {
	if !isAncestorDir("/data/ccd/inbox", "/data/ccd/inbox/csv_test_inbox") {
		t.Error("expected child directory to be recognized as descendant")
	}
}

func TestIsAncestorDir_Unrelated(t *testing.T) {
	if isAncestorDir("/data/ccd/inbox", "/data/hl7/incoming") {
		t.Error("expected unrelated directories to not be ancestor/descendant")
	}
}

// Guards against a naive strings.HasPrefix(child, parent) check (without the
// trailing separator), which would wrongly treat "/data/ccd/inbox2" as a child
// of "/data/ccd/inbox" just because it shares a string prefix.
func TestIsAncestorDir_SimilarSiblingNotAncestor(t *testing.T) {
	if isAncestorDir("/data/ccd/inbox", "/data/ccd/inbox2") {
		t.Error("expected sibling directory with shared string prefix to NOT be treated as descendant")
	}
}

func TestIsAncestorDir_EmptyInputs(t *testing.T) {
	if isAncestorDir("", "/data/ccd/inbox") || isAncestorDir("/data/ccd/inbox", "") {
		t.Error("expected empty path to never be treated as ancestor or descendant")
	}
}

// ─── isCatchAllPattern ───────────────────────────────────────────────────────

func TestIsCatchAllPattern(t *testing.T) {
	cases := map[string]bool{
		"*":      true,
		"*.*":    true,
		"*.hl7":  false,
		"*.ccd":  false,
		"":       false,
	}
	for pattern, want := range cases {
		if got := isCatchAllPattern(pattern); got != want {
			t.Errorf("isCatchAllPattern(%q) = %v, want %v", pattern, got, want)
		}
	}
}

// ─── ExtractFileListenerDirConfig ────────────────────────────────────────────

func TestExtractFileListenerDirConfig_SnakeCase(t *testing.T) {
	cfg := map[string]interface{}{
		"directory_path": "/data/ccd/inbox",
		"file_pattern":   "*.*",
		"recursive":      true,
	}
	dir, pattern, recursive, ok := connectors.ExtractFileListenerDirConfig(cfg)
	if !ok || dir != "/data/ccd/inbox" || pattern != "*.*" || !recursive {
		t.Errorf("unexpected result: dir=%q pattern=%q recursive=%v ok=%v", dir, pattern, recursive, ok)
	}
}

func TestExtractFileListenerDirConfig_CamelCase(t *testing.T) {
	cfg := map[string]interface{}{
		"directoryPath": "/data/hl7/incoming",
		"filePattern":   "*.hl7",
	}
	dir, pattern, recursive, ok := connectors.ExtractFileListenerDirConfig(cfg)
	if !ok || dir != "/data/hl7/incoming" || pattern != "*.hl7" || recursive {
		t.Errorf("unexpected result: dir=%q pattern=%q recursive=%v ok=%v", dir, pattern, recursive, ok)
	}
}

func TestExtractFileListenerDirConfig_DefaultPattern(t *testing.T) {
	cfg := map[string]interface{}{"directory_path": "/data/ccd/inbox"}
	_, pattern, _, ok := connectors.ExtractFileListenerDirConfig(cfg)
	if !ok || pattern != "*" {
		t.Errorf("expected default pattern '*', got %q (ok=%v)", pattern, ok)
	}
}

func TestExtractFileListenerDirConfig_MissingDirectory(t *testing.T) {
	cfg := map[string]interface{}{"file_pattern": "*.*"}
	_, _, _, ok := connectors.ExtractFileListenerDirConfig(cfg)
	if ok {
		t.Error("expected ok=false when directory_path is missing")
	}
}

// ─── dirListenersConflict ────────────────────────────────────────────────────

func TestDirListenersConflict_SameDirSamePattern(t *testing.T) {
	conflict, _ := dirListenersConflict("/data/ccd/inbox", "*.*", false, "/data/ccd/inbox", "*.*", false)
	if !conflict {
		t.Error("expected conflict for two listeners on the same directory + pattern")
	}
}

func TestDirListenersConflict_SameDirCatchAllOverlap(t *testing.T) {
	// One side is a catch-all ("*.*"), so it overlaps any other pattern on the same dir.
	conflict, _ := dirListenersConflict("/data/ccd/inbox", "*.*", false, "/data/ccd/inbox", "*.hl7", false)
	if !conflict {
		t.Error("expected conflict when one pattern is a catch-all on the same directory")
	}
}

func TestDirListenersConflict_SameDirDisjointPattern(t *testing.T) {
	conflict, _ := dirListenersConflict("/data/ccd/inbox", "*.hl7", false, "/data/ccd/inbox", "*.ccd", false)
	if conflict {
		t.Error("expected no conflict for disjoint non-catch-all patterns on the same directory")
	}
}

// Real pair from production: CCD Interface (/data/ccd/inbox, *.*, non-recursive)
// vs CCD to CSV Export (Test) (/data/ccd/inbox/csv_test_inbox, a child directory).
// Since the ancestor (CCD Interface) is NOT recursive, its Glob never descends
// into the child directory, so this must NOT be flagged.
func TestDirListenersConflict_NonRecursiveAncestor_NoConflict(t *testing.T) {
	conflict, _ := dirListenersConflict(
		"/data/ccd/inbox", "*.*", false,
		"/data/ccd/inbox/csv_test_inbox", "*", false,
	)
	if conflict {
		t.Error("expected no conflict when the ancestor directory's listener is non-recursive")
	}
}

func TestDirListenersConflict_RecursiveAncestor_Conflict(t *testing.T) {
	conflict, _ := dirListenersConflict(
		"/data/ccd/inbox", "*.*", true,
		"/data/ccd/inbox/csv_test_inbox", "*", false,
	)
	if !conflict {
		t.Error("expected conflict when the ancestor directory's listener is recursive")
	}
}

func TestDirListenersConflict_RecursiveDescendantSide(t *testing.T) {
	// Same pair, but the recursive flag is on the side passed as B instead of A —
	// the rule must be symmetric regardless of argument order.
	conflict, _ := dirListenersConflict(
		"/data/ccd/inbox/csv_test_inbox", "*", false,
		"/data/ccd/inbox", "*.*", true,
	)
	if !conflict {
		t.Error("expected conflict regardless of which argument position the recursive ancestor is passed in")
	}
}

func TestDirListenersConflict_UnrelatedDirectories(t *testing.T) {
	conflict, _ := dirListenersConflict("/data/ccd/inbox", "*.*", true, "/data/hl7/incoming", "*.hl7", true)
	if conflict {
		t.Error("expected no conflict for unrelated directories")
	}
}

package ai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot is two levels up from services/ai (go test runs in the package dir).
var repoRoot = filepath.Join("..", "..")

// Every configured standard must point at a real schema tree — a renamed or
// moved schema directory would otherwise silently drop that standard from
// ezCompanion's knowledge (ingestion only logs "directory not found").
func TestStandardSchemaSources_DirectoriesExistAndHoldSchemas(t *testing.T) {
	seen := map[string]bool{}
	for _, src := range standardSchemaSources {
		if seen[src.SourceType] {
			t.Errorf("duplicate source type %q", src.SourceType)
		}
		seen[src.SourceType] = true

		dir := filepath.Join(repoRoot, src.Dir)
		count := 0
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".json") {
				count++
			}
			return nil
		})
		if count == 0 {
			t.Errorf("%s: no schema files under %s", src.SourceType, dir)
		}
	}
}

func TestStandardSchemaChunkText_IsSelfDescribing(t *testing.T) {
	src := StandardSchemaSource{SourceType: "edi_x12_schema", Label: "EDI X12 005010"}
	got := standardSchemaChunkText(src, "transactionSets/835", `{"id":"835"}`)
	for _, want := range []string{"Standard: EDI X12 005010", "Schema definition: transactionSets/835", `{"id":"835"}`} {
		if !strings.Contains(got, want) {
			t.Errorf("chunk text missing %q:\n%s", want, got)
		}
	}
}

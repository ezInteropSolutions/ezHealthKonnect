// cda/schema_loader_test.go
package cda

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update", false, "update golden test fixtures")

// schemaSnapshot captures everything downstream consumers rely on the loader
// to resolve, sourced through the same access CDASchemaLoader's public API
// exposes today (plus profile.DocumentTemplates, read directly since this
// test lives in package cda alongside c32_normalizer.go's own same-package
// access to that field — there is no public getter for it). A byte-identical
// snapshot before/after the ccda_2_1.json -> ccda_2_1/ directory-tree
// restructuring proves the split changes nothing about what
// cda/builder/cda/document actually consume.
type schemaSnapshot struct {
	DocumentTemplates    map[string]string
	DocumentTypeMetadata map[string]DocumentTypeMetadata
	DocumentTypeSections map[string]DocumentTypeSectionInfo
	DocumentTypes        []string
	Sections             []*CDASectionDef
}

func buildSnapshot(loader *CDASchemaLoader) schemaSnapshot {
	// AllDocumentTypes() iterates a Go map internally (pre-existing, not
	// introduced by this test) — sorted here so the snapshot itself is
	// deterministic across runs regardless of that.
	docTypes := loader.AllDocumentTypes()
	sort.Strings(docTypes)

	dtSections := make(map[string]DocumentTypeSectionInfo, len(docTypes))
	dtMeta := make(map[string]DocumentTypeMetadata, len(docTypes))
	for _, dt := range docTypes {
		if info := loader.GetDocumentTypeSections(dt); info != nil {
			dtSections[dt] = *info
		}
		if meta := loader.GetDocumentTypeMetadata(dt); meta != nil {
			dtMeta[dt] = *meta
		}
	}

	return schemaSnapshot{
		DocumentTemplates:    loader.profile.DocumentTemplates,
		DocumentTypeMetadata: dtMeta,
		DocumentTypeSections: dtSections,
		DocumentTypes:        docTypes,
		Sections:             loader.AllSections(),
	}
}

// TestSchemaLoaderResolvesGoldenSnapshot proves the schema loader's
// fully-resolved output is byte-identical before and after the
// ccda_2_1.json -> ccda_2_1/{manifest,sections,entries} directory-tree
// restructuring (see the CDA schema restructuring plan). Run with -update to
// (re)generate the fixture — the only sanctioned way to change it, so any
// future intentional schema change (including the 3 named gap-fixes the
// restructuring itself introduces — see the plan) has an auditable
// regeneration path instead of hand-edited golden JSON.
func TestSchemaLoaderResolvesGoldenSnapshot(t *testing.T) {
	loader, err := NewCDASchemaLoader("./schemas")
	require.NoError(t, err, "loading CDA schema")

	got, err := json.MarshalIndent(buildSnapshot(loader), "", "  ")
	require.NoError(t, err, "marshalling schema snapshot")

	golden := filepath.Join("testdata", "schema_snapshot_golden.json")

	if *updateGolden {
		require.NoError(t, os.MkdirAll(filepath.Dir(golden), 0755))
		require.NoError(t, os.WriteFile(golden, got, 0644))
		return
	}

	want, err := os.ReadFile(golden)
	require.NoError(t, err, "reading golden fixture %s — run with -update to generate it", golden)
	assert.Equal(t, string(want), string(got),
		"schema snapshot drifted from golden fixture — run with -update if this is an intentional schema change")
}

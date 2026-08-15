// cda/schema_disk_types_test.go
package cda

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =====================================
// resolveXPath — the single highest-risk piece of new logic (see
// schema_disk_types.go's own doc comment on resolveXPath for the real-schema
// bug this exact function shape was hardened against: header-section paths
// with no anchor).
// =====================================

func TestResolveXPath(t *testing.T) {
	cases := []struct {
		name   string
		anchor string
		raw    string
		want   string
	}{
		{"empty raw passes through", "entry/act", "", ""},
		{"already-absolute entry path passes through unchanged", "entry/act", "entry/act/entryRelationship[@typeCode='SUBJ']/observation/value/@code", "entry/act/entryRelationship[@typeCode='SUBJ']/observation/value/@code"},
		{"no anchor (header section) passes through unchanged even without entry/ prefix", "", "ClinicalDocument/custodian/assignedCustodian/representedCustodianOrganization/name", "ClinicalDocument/custodian/assignedCustodian/representedCustodianOrganization/name"},
		{"relative path gets prefixed with the anchor", "entry/observation", "value/@code", "entry/observation/value/@code"},
		{"relative path with predicate gets prefixed with the anchor", "entry/act/entryRelationship[@typeCode='SUBJ']/observation", "entryRelationship[@typeCode='REFR']/observation[templateId/@root='2.16.840.1.113883.10.20.22.4.113']/value/@code", "entry/act/entryRelationship[@typeCode='SUBJ']/observation/entryRelationship[@typeCode='REFR']/observation[templateId/@root='2.16.840.1.113883.10.20.22.4.113']/value/@code"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, resolveXPath(c.anchor, c.raw))
		})
	}
}

// =====================================
// resolveAnchor — templateIdRef substitution
// =====================================

func TestResolveAnchor_LiteralTemplateIDPassesThrough(t *testing.T) {
	anchor, err := resolveAnchor(onDiskStructuralTemplateAnchor{
		Path:          "entry/act/author",
		TemplateID:    "2.16.840.1.113883.10.20.22.4.119",
		TemplateIDExt: "2014-06-09",
	}, map[string]onDiskTemplateConstant{})
	require.NoError(t, err)
	assert.Equal(t, "2.16.840.1.113883.10.20.22.4.119", anchor.TemplateID)
	assert.Equal(t, "2014-06-09", anchor.TemplateIDExt)
}

func TestResolveAnchor_TemplateIDRefResolvesAgainstConstants(t *testing.T) {
	constants := map[string]onDiskTemplateConstant{
		"authorParticipation": {TemplateID: "2.16.840.1.113883.10.20.22.4.119"},
	}
	anchor, err := resolveAnchor(onDiskStructuralTemplateAnchor{
		Path:          "entry/act/author",
		TemplateIDRef: "authorParticipation",
	}, constants)
	require.NoError(t, err)
	assert.Equal(t, "entry/act/author", anchor.Path)
	assert.Equal(t, "2.16.840.1.113883.10.20.22.4.119", anchor.TemplateID)
	assert.Empty(t, anchor.TemplateIDExt)
}

func TestResolveAnchor_UnknownTemplateIDRefFailsFast(t *testing.T) {
	_, err := resolveAnchor(onDiskStructuralTemplateAnchor{
		Path:          "entry/act/author",
		TemplateIDRef: "doesNotExist",
	}, map[string]onDiskTemplateConstant{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "doesNotExist")
}

// =====================================
// resolveSection — root-anchor and observation-anchor template merging
// =====================================

func TestResolveSection_RootAnchorTemplate(t *testing.T) {
	templates := map[string]*onDiskEntryTemplate{
		"medication-activity": {
			Key:              "medication-activity",
			Anchor:           "root",
			EntryTemplateID:  "2.16.840.1.113883.10.20.22.4.16",
			EntryElementPath: "entry/substanceAdministration",
		},
	}
	disk := &onDiskSectionDef{
		Key:           "medications",
		EntryTemplate: "medication-activity",
		Fields: []*CDAFieldDef{
			{Key: "drugCode", XPath: "entry/substanceAdministration/consumable/manufacturedProduct/manufacturedMaterial/code/@code"},
		},
	}
	sec, err := resolveSection(disk, templates, nil)
	require.NoError(t, err)
	assert.Equal(t, "2.16.840.1.113883.10.20.22.4.16", sec.EntryTemplateID)
	assert.Equal(t, "entry/substanceAdministration", sec.EntryElementPath)
	assert.Empty(t, sec.ObservationElementPath)
	require.Len(t, sec.Fields, 1)
	assert.Equal(t, "entry/substanceAdministration/consumable/manufacturedProduct/manufacturedMaterial/code/@code", sec.Fields[0].XPath)
}

func TestResolveSection_ObservationAnchorTemplate(t *testing.T) {
	templates := map[string]*onDiskEntryTemplate{
		"problem-observation-wrapped": {
			Key:                          "problem-observation-wrapped",
			Anchor:                       "observation",
			ObsTemplateID:                "2.16.840.1.113883.10.20.22.4.4",
			ObservationElementPathSuffix: "entryRelationship[@typeCode='SUBJ']/observation",
			ObsFixedCode:                 "55607006",
			ObsFixedCodeSystem:           "2.16.840.1.113883.6.96",
		},
	}
	disk := &onDiskSectionDef{
		Key:                 "dischargeDiagnosis",
		EntryTemplateID:     "2.16.840.1.113883.10.20.22.4.33",
		EntryElementPath:    "entry/act",
		ObservationTemplate: "problem-observation-wrapped",
		Fields: []*CDAFieldDef{
			{Key: "conditionCode", XPath: "entry/act/entryRelationship[@typeCode='SUBJ']/observation/value/@code"},
		},
	}
	sec, err := resolveSection(disk, templates, nil)
	require.NoError(t, err)
	// EntryTemplateID/EntryElementPath stay section-owned — verified via the
	// real schema that each "wrapped" cluster member cites its own distinct
	// wrapping-act template, so the template must never overwrite these.
	assert.Equal(t, "2.16.840.1.113883.10.20.22.4.33", sec.EntryTemplateID)
	assert.Equal(t, "entry/act", sec.EntryElementPath)
	assert.Equal(t, "2.16.840.1.113883.10.20.22.4.4", sec.ObsTemplateID)
	assert.Equal(t, "entry/act/entryRelationship[@typeCode='SUBJ']/observation", sec.ObservationElementPath)
	assert.Equal(t, "55607006", sec.ObsFixedCode)
	assert.Equal(t, "2.16.840.1.113883.6.96", sec.ObsFixedCodeSystem)
}

func TestResolveSection_UnknownEntryTemplateFailsFast(t *testing.T) {
	disk := &onDiskSectionDef{Key: "x", EntryTemplate: "doesNotExist"}
	_, err := resolveSection(disk, map[string]*onDiskEntryTemplate{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "doesNotExist")
}

func TestResolveSection_EntryTemplateWrongAnchorFailsFast(t *testing.T) {
	templates := map[string]*onDiskEntryTemplate{
		"observation-shaped": {Key: "observation-shaped", Anchor: "observation"},
	}
	disk := &onDiskSectionDef{Key: "x", EntryTemplate: "observation-shaped"}
	_, err := resolveSection(disk, templates, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a root-anchor template")
}

func TestResolveSection_UnknownObservationTemplateFailsFast(t *testing.T) {
	disk := &onDiskSectionDef{Key: "x", EntryElementPath: "entry/act", ObservationTemplate: "doesNotExist"}
	_, err := resolveSection(disk, map[string]*onDiskEntryTemplate{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "doesNotExist")
}

func TestResolveSection_ObservationTemplateWithoutEntryElementPathFailsFast(t *testing.T) {
	templates := map[string]*onDiskEntryTemplate{
		"obs-tmpl": {Key: "obs-tmpl", Anchor: "observation", ObservationElementPathSuffix: "observation"},
	}
	disk := &onDiskSectionDef{Key: "x", ObservationTemplate: "obs-tmpl"} // no EntryElementPath, no EntryTemplate
	_, err := resolveSection(disk, templates, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires entryElementPath")
}

// =====================================
// loadProfile — fail-fast paths on the manifest/section-file contract
// =====================================

// writeMinimalTree writes just enough of a ccda_2_1/ tree (manifest.json +
// one section) under dir for loadProfile()-level fail-fast tests, without
// needing the real 69-section schema on disk.
func writeMinimalTree(t *testing.T, dir string, manifestJSON string, sectionFiles map[string]string) {
	t.Helper()
	root := filepath.Join(dir, "ccda_2_1")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sections"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "manifest.json"), []byte(manifestJSON), 0644))
	for name, content := range sectionFiles {
		require.NoError(t, os.WriteFile(filepath.Join(root, "sections", name), []byte(content), 0644))
	}
}

func TestNewCDASchemaLoader_DuplicateSectionOrderKeyFailsFast(t *testing.T) {
	dir := t.TempDir()
	writeMinimalTree(t, dir, `{"sectionOrder": ["a", "a"]}`, map[string]string{
		"a.json": `{"key": "a", "fields": []}`,
	})
	writeC32Fixture(t, dir)

	_, err := NewCDASchemaLoader(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate section key")
}

func TestNewCDASchemaLoader_SectionKeyMismatchFailsFast(t *testing.T) {
	dir := t.TempDir()
	writeMinimalTree(t, dir, `{"sectionOrder": ["a"]}`, map[string]string{
		"a.json": `{"key": "notA", "fields": []}`,
	})
	writeC32Fixture(t, dir)

	_, err := NewCDASchemaLoader(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `declares key "notA"`)
}

func TestNewCDASchemaLoader_MissingSectionFileFailsFast(t *testing.T) {
	dir := t.TempDir()
	writeMinimalTree(t, dir, `{"sectionOrder": ["missing"]}`, map[string]string{})
	writeC32Fixture(t, dir)

	_, err := NewCDASchemaLoader(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot read section")
}

// writeC32Fixture writes a minimal, valid c32_mapping.json — NewCDASchemaLoader
// always loads it after loadProfile(), so every loadProfile()-focused test
// above needs one present to reach the code path under test rather than
// failing earlier for an unrelated reason.
func writeC32Fixture(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c32_mapping.json"), []byte(`{
		"profileDetection": {"c32TemplateIds": [], "hitspTemplateIds": []},
		"mappings": []
	}`), 0644))
}

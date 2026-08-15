// cda/schema_disk_types.go
// On-disk-only types for the cda/schemas/ccda_2_1/ directory tree
// (manifest.json + sections/*.json + entries/*.json) and the load-time
// resolution logic that turns them into the same *CDASectionDef/
// *CDAProfileDef shapes cda/schema_types.go has always defined. These types
// exist only so schema_loader.go can parse the on-disk JSON shape — no
// downstream package (cda/builder, cda/document, services/cda_fhir,
// services/cda_coverage) ever sees them.
package cda

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// onDiskManifest is cda/schemas/ccda_2_1/manifest.json — Tier 1.
type onDiskManifest struct {
	Profile              string                             `json:"profile"`
	Version              string                             `json:"version"`
	HL7Version           string                             `json:"hl7Version"`
	DocumentTemplates    map[string]string                  `json:"documentTemplates"`
	DocumentTypeMetadata map[string]DocumentTypeMetadata    `json:"documentTypeMetadata"`
	DocumentTypeSections map[string]DocumentTypeSectionInfo `json:"documentTypeSections"`

	// SectionOrder makes AllSections()' iteration order an explicit decision
	// rather than an accident of directory listing — a directory of files has
	// no inherent order the way the old single JSON array did.
	SectionOrder []string `json:"sectionOrder"`
}

// onDiskSectionDef mirrors CDASectionDef's JSON shape exactly (Tier 2, one
// file under sections/), plus two tier-3 template-reference fields with no
// counterpart on the resolved, in-memory CDASectionDef — resolveSection
// resolves them away, so cda/builder and every other downstream consumer
// never sees this type.
type onDiskSectionDef struct {
	Key                 string         `json:"key"`
	USCDIClass          string         `json:"uscdiClass"`
	DisplayName         string         `json:"displayName"`
	LOINCCode           string         `json:"loincCode"`
	TemplateID          string         `json:"templateId"`
	TemplateIDExt       string         `json:"templateIdExtension,omitempty"`
	TemplateIDOptional  string         `json:"templateIdOptional"`
	Conformance         string         `json:"conformance"`
	EntryTemplateID     string         `json:"entryTemplateId,omitempty"`
	EntryTemplateIDExt  string         `json:"entryTemplateIdExtension,omitempty"`
	EntryTemplate       string         `json:"entryTemplate,omitempty"`
	ObsTemplateID       string         `json:"observationTemplateId,omitempty"`
	ObsTemplateIDExt    string         `json:"observationTemplateIdExtension,omitempty"`
	ObservationTemplate string         `json:"observationTemplate,omitempty"`
	IsHeader            bool           `json:"isHeader,omitempty"`
	Fields              []*CDAFieldDef `json:"fields"`

	EntryElementPath       string `json:"entryElementPath,omitempty"`
	ObservationElementPath string `json:"observationElementPath,omitempty"`

	StructuralTemplateIDs []onDiskStructuralTemplateAnchor `json:"structuralTemplateIds,omitempty"`

	EntryStatusCodeOverride string `json:"entryStatusCodeOverride,omitempty"`
	EntryClassCodeOverride  string `json:"entryClassCodeOverride,omitempty"`
	EntryMoodCodeOverride   string `json:"entryMoodCodeOverride,omitempty"`

	EntryFixedCode        string `json:"entryFixedCode,omitempty"`
	EntryFixedCodeSystem  string `json:"entryFixedCodeSystem,omitempty"`
	EntryFixedCodeDisplay string `json:"entryFixedCodeDisplay,omitempty"`

	EntryCodeTranslationCode    string `json:"entryCodeTranslationCode,omitempty"`
	EntryCodeTranslationSystem  string `json:"entryCodeTranslationSystem,omitempty"`
	EntryCodeTranslationDisplay string `json:"entryCodeTranslationDisplay,omitempty"`

	ObsCodeTranslationCode    string `json:"obsCodeTranslationCode,omitempty"`
	ObsCodeTranslationSystem  string `json:"obsCodeTranslationSystem,omitempty"`
	ObsCodeTranslationDisplay string `json:"obsCodeTranslationDisplay,omitempty"`

	ObsFixedCode        string `json:"observationFixedCode,omitempty"`
	ObsFixedCodeSystem  string `json:"observationFixedCodeSystem,omitempty"`
	ObsFixedCodeDisplay string `json:"observationFixedCodeDisplay,omitempty"`

	RepeatingGroups     []RepeatingGroup                `json:"repeatingGroups,omitempty"`
	AlternateArchetypes []onDiskAlternateEntryArchetype `json:"alternateArchetypes,omitempty"`
}

// onDiskAlternateEntryArchetype mirrors AlternateEntryArchetype, plus the
// same StructuralTemplateIDs templateIdRef indirection onDiskSectionDef gets.
type onDiskAlternateEntryArchetype struct {
	EntriesKey         string         `json:"entriesKey"`
	EntryElementPath   string         `json:"entryElementPath"`
	EntryTemplateID    string         `json:"entryTemplateId,omitempty"`
	EntryTemplateIDExt string         `json:"entryTemplateIdExtension,omitempty"`
	Fields             []*CDAFieldDef `json:"fields"`

	StructuralTemplateIDs []onDiskStructuralTemplateAnchor `json:"structuralTemplateIds,omitempty"`
}

// onDiskStructuralTemplateAnchor mirrors StructuralTemplateAnchor, plus
// TemplateIDRef — an alternative to the literal TemplateID/TemplateIDExt
// pair that resolves against entries/_constants.json instead. Used for
// small, shape-less repeated anchors (Author Participation, Medication
// Information) that don't warrant a full entry template.
type onDiskStructuralTemplateAnchor struct {
	Path          string `json:"path"`
	TemplateID    string `json:"templateId,omitempty"`
	TemplateIDExt string `json:"templateIdExtension,omitempty"`
	TemplateIDRef string `json:"templateIdRef,omitempty"`

	FixedCode        string `json:"fixedCode,omitempty"`
	FixedCodeSystem  string `json:"fixedCodeSystem,omitempty"`
	FixedCodeDisplay string `json:"fixedCodeDisplay,omitempty"`
	FixedStatusCode  string `json:"fixedStatusCode,omitempty"`
}

// onDiskEntryTemplate is one file under entries/ (excluding _constants.json)
// — a reusable entry-archetype fragment referenced by section files via
// "entryTemplate"/"observationTemplate". Anchor selects which half of the
// struct is populated:
//
//   - "root": supplies CDASectionDef's EntryTemplateID/EntryElementPath (+
//     EntryFixedCode*/EntryCodeTranslation*/EntryMoodCodeOverride when THOSE
//     were also verified byte-identical across every member section).
//
//   - "observation": supplies ONLY the nested-observation anchor
//     (ObsTemplateID/ObservationElementPathSuffix/ObsFixedCode*/
//     ObsCodeTranslation*) — deliberately never EntryTemplateID/
//     EntryElementPath/EntryFixedCode, which were verified (by reading every
//     member section of both "wrapped" clusters this schema uses) to differ
//     per section — each cites its own distinct wrapping-act template with
//     its own fixed code, so those fields must stay section-owned rather
//     than templated.
type onDiskEntryTemplate struct {
	Key    string `json:"key"`
	Anchor string `json:"anchor"` // "root" | "observation"

	// Anchor == "root"
	EntryTemplateID       string `json:"entryTemplateId,omitempty"`
	EntryTemplateIDExt    string `json:"entryTemplateIdExtension,omitempty"`
	EntryElementPath      string `json:"entryElementPath,omitempty"`
	EntryMoodCodeOverride string `json:"entryMoodCodeOverride,omitempty"`

	EntryFixedCode        string `json:"entryFixedCode,omitempty"`
	EntryFixedCodeSystem  string `json:"entryFixedCodeSystem,omitempty"`
	EntryFixedCodeDisplay string `json:"entryFixedCodeDisplay,omitempty"`

	EntryCodeTranslationCode    string `json:"entryCodeTranslationCode,omitempty"`
	EntryCodeTranslationSystem  string `json:"entryCodeTranslationSystem,omitempty"`
	EntryCodeTranslationDisplay string `json:"entryCodeTranslationDisplay,omitempty"`

	// Anchor == "observation"
	ObsTemplateID                string `json:"observationTemplateId,omitempty"`
	ObsTemplateIDExt              string `json:"observationTemplateIdExtension,omitempty"`
	ObservationElementPathSuffix string `json:"observationElementPathSuffix,omitempty"`

	ObsFixedCode        string `json:"observationFixedCode,omitempty"`
	ObsFixedCodeSystem  string `json:"observationFixedCodeSystem,omitempty"`
	ObsFixedCodeDisplay string `json:"observationFixedCodeDisplay,omitempty"`

	ObsCodeTranslationCode    string `json:"obsCodeTranslationCode,omitempty"`
	ObsCodeTranslationSystem  string `json:"obsCodeTranslationSystem,omitempty"`
	ObsCodeTranslationDisplay string `json:"obsCodeTranslationDisplay,omitempty"`
}

// onDiskTemplateConstant is one entries/_constants.json entry — a flat OID
// constant for anchors that repeat the same templateId with no other shared
// shape (see onDiskStructuralTemplateAnchor.TemplateIDRef).
type onDiskTemplateConstant struct {
	TemplateID    string `json:"templateId"`
	TemplateIDExt string `json:"templateIdExtension,omitempty"`
}

// =====================================
// Readers
// =====================================

func readManifest(path string) (*onDiskManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read manifest %s: %w", path, err)
	}
	var m onDiskManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("cannot parse manifest %s: %w", path, err)
	}
	return &m, nil
}

// readTemplateConstants tolerates a missing _constants.json (Phase 1 of the
// schema restructuring has no entries/ directory at all yet) — returns an
// empty map, not an error.
func readTemplateConstants(path string) (map[string]onDiskTemplateConstant, error) {
	constants := make(map[string]onDiskTemplateConstant)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return constants, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read template constants %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &constants); err != nil {
		return nil, fmt.Errorf("cannot parse template constants %s: %w", path, err)
	}
	return constants, nil
}

// readEntryTemplates tolerates a missing entries/ directory (same reason as
// readTemplateConstants) — returns an empty map, not an error.
func readEntryTemplates(dir string) (map[string]*onDiskEntryTemplate, error) {
	templates := make(map[string]*onDiskEntryTemplate)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return templates, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot list entry templates dir %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || e.Name() == "_constants.json" || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("cannot read entry template %s: %w", path, err)
		}
		var t onDiskEntryTemplate
		if err := json.Unmarshal(data, &t); err != nil {
			return nil, fmt.Errorf("cannot parse entry template %s: %w", path, err)
		}
		if t.Key == "" {
			return nil, fmt.Errorf("entry template %s missing required \"key\"", path)
		}
		if _, dup := templates[t.Key]; dup {
			return nil, fmt.Errorf("duplicate entry template key %q (file %s)", t.Key, path)
		}
		templates[t.Key] = &t
	}
	return templates, nil
}

// =====================================
// Resolution
// =====================================

// resolveXPath expands a field's on-disk xpath string into the same absolute,
// section-relative form CDAFieldDef.XPath has always held. A raw xpath is
// treated as already pre-resolved/absolute — and passed through UNCHANGED —
// whenever it starts with "entry/" (the normal case for a section with an
// entry archetype) OR whenever the section has no anchor at all (anchor ==
// "" — the header pseudo-sections, whose fields target
// "ClinicalDocument/..." paths that are absolute from the document root, not
// relative to any entry anchor; also true, vacuously, of the 26 fully
// narrative-only sections, which have no Fields at all). This is what makes
// every section with no entryTemplate/observationTemplate reference (the
// common case) a zero-risk, byte-for-byte copy regardless of the template
// mechanism's existence — confirmed by the schema restructuring's golden-file
// regression test, which caught this exact header-path case before it was
// guarded here.
//
// Only when the section DOES have a real anchor AND the raw path does not
// already start with "entry/" is it treated as anchor-relative (the same
// convention RepeatingGroup.Fields[].XPath already uses, just resolved at
// schema-load time here instead of build time in
// cda/builder/entry_archetypes.go's writeRepeatingGroups) and prefixed with
// the section's own resolved anchor (its ObservationElementPath if it has
// one, else its EntryElementPath).
func resolveXPath(anchor, raw string) string {
	if raw == "" || anchor == "" || strings.HasPrefix(raw, "entry/") {
		return raw
	}
	return anchor + "/" + raw
}

// resolveAnchor turns an on-disk structural template anchor into the real
// StructuralTemplateAnchor cda/builder consumes, substituting TemplateIDRef
// against constants when present.
func resolveAnchor(a onDiskStructuralTemplateAnchor, constants map[string]onDiskTemplateConstant) (StructuralTemplateAnchor, error) {
	id, ext := a.TemplateID, a.TemplateIDExt
	if a.TemplateIDRef != "" {
		c, ok := constants[a.TemplateIDRef]
		if !ok {
			return StructuralTemplateAnchor{}, fmt.Errorf("unknown templateIdRef %q", a.TemplateIDRef)
		}
		id, ext = c.TemplateID, c.TemplateIDExt
	}
	return StructuralTemplateAnchor{
		Path:             a.Path,
		TemplateID:       id,
		TemplateIDExt:    ext,
		FixedCode:        a.FixedCode,
		FixedCodeSystem:  a.FixedCodeSystem,
		FixedCodeDisplay: a.FixedCodeDisplay,
		FixedStatusCode:  a.FixedStatusCode,
	}, nil
}

// resolveFields resolves one Fields[] slice (shared by section-level and
// AlternateEntryArchetype-level resolution) against anchor.
func resolveFields(fields []*CDAFieldDef, anchor string) []*CDAFieldDef {
	resolved := make([]*CDAFieldDef, len(fields))
	for i, f := range fields {
		r := *f
		r.XPath = resolveXPath(anchor, f.XPath)
		r.XPathDisplay = resolveXPath(anchor, f.XPathDisplay)
		r.XPathSystem = resolveXPath(anchor, f.XPathSystem)
		r.XPathUnit = resolveXPath(anchor, f.XPathUnit)
		r.XPathFamily = resolveXPath(anchor, f.XPathFamily)
		r.SkipIfXPathPresent = resolveXPath(anchor, f.SkipIfXPathPresent)
		resolved[i] = &r
	}
	return resolved
}

// resolveSection turns one on-disk section file into the real *CDASectionDef
// cda/builder and cda/document consume, merging in its entryTemplate/
// observationTemplate reference (if any) first so field/anchor resolution
// sees the final, merged shape.
func resolveSection(disk *onDiskSectionDef, templates map[string]*onDiskEntryTemplate, constants map[string]onDiskTemplateConstant) (*CDASectionDef, error) {
	sec := &CDASectionDef{
		Key:                         disk.Key,
		USCDIClass:                  disk.USCDIClass,
		DisplayName:                 disk.DisplayName,
		LOINCCode:                   disk.LOINCCode,
		TemplateID:                  disk.TemplateID,
		TemplateIDExt:               disk.TemplateIDExt,
		TemplateIDOptional:          disk.TemplateIDOptional,
		Conformance:                 disk.Conformance,
		EntryTemplateID:             disk.EntryTemplateID,
		EntryTemplateIDExt:          disk.EntryTemplateIDExt,
		ObsTemplateID:               disk.ObsTemplateID,
		ObsTemplateIDExt:            disk.ObsTemplateIDExt,
		IsHeader:                    disk.IsHeader,
		EntryElementPath:            disk.EntryElementPath,
		ObservationElementPath:      disk.ObservationElementPath,
		EntryStatusCodeOverride:     disk.EntryStatusCodeOverride,
		EntryClassCodeOverride:      disk.EntryClassCodeOverride,
		EntryMoodCodeOverride:       disk.EntryMoodCodeOverride,
		EntryFixedCode:              disk.EntryFixedCode,
		EntryFixedCodeSystem:        disk.EntryFixedCodeSystem,
		EntryFixedCodeDisplay:       disk.EntryFixedCodeDisplay,
		EntryCodeTranslationCode:    disk.EntryCodeTranslationCode,
		EntryCodeTranslationSystem:  disk.EntryCodeTranslationSystem,
		EntryCodeTranslationDisplay: disk.EntryCodeTranslationDisplay,
		ObsCodeTranslationCode:      disk.ObsCodeTranslationCode,
		ObsCodeTranslationSystem:    disk.ObsCodeTranslationSystem,
		ObsCodeTranslationDisplay:   disk.ObsCodeTranslationDisplay,
		ObsFixedCode:                disk.ObsFixedCode,
		ObsFixedCodeSystem:          disk.ObsFixedCodeSystem,
		ObsFixedCodeDisplay:         disk.ObsFixedCodeDisplay,
		RepeatingGroups:             disk.RepeatingGroups,
	}

	if disk.EntryTemplate != "" {
		t, ok := templates[disk.EntryTemplate]
		if !ok {
			return nil, fmt.Errorf("unknown entryTemplate %q", disk.EntryTemplate)
		}
		if t.Anchor != "root" {
			return nil, fmt.Errorf("entryTemplate %q is not a root-anchor template (anchor=%q)", disk.EntryTemplate, t.Anchor)
		}
		sec.EntryTemplateID = t.EntryTemplateID
		sec.EntryTemplateIDExt = t.EntryTemplateIDExt
		sec.EntryElementPath = t.EntryElementPath
		if t.EntryMoodCodeOverride != "" {
			sec.EntryMoodCodeOverride = t.EntryMoodCodeOverride
		}
		if t.EntryFixedCode != "" {
			sec.EntryFixedCode = t.EntryFixedCode
			sec.EntryFixedCodeSystem = t.EntryFixedCodeSystem
			sec.EntryFixedCodeDisplay = t.EntryFixedCodeDisplay
		}
		if t.EntryCodeTranslationCode != "" {
			sec.EntryCodeTranslationCode = t.EntryCodeTranslationCode
			sec.EntryCodeTranslationSystem = t.EntryCodeTranslationSystem
			sec.EntryCodeTranslationDisplay = t.EntryCodeTranslationDisplay
		}
	}

	if disk.ObservationTemplate != "" {
		t, ok := templates[disk.ObservationTemplate]
		if !ok {
			return nil, fmt.Errorf("unknown observationTemplate %q", disk.ObservationTemplate)
		}
		if t.Anchor != "observation" {
			return nil, fmt.Errorf("observationTemplate %q is not an observation-anchor template (anchor=%q)", disk.ObservationTemplate, t.Anchor)
		}
		if sec.EntryElementPath == "" {
			return nil, fmt.Errorf("observationTemplate %q requires entryElementPath to already be set on the section (inline or via entryTemplate)", disk.ObservationTemplate)
		}
		sec.ObsTemplateID = t.ObsTemplateID
		sec.ObsTemplateIDExt = t.ObsTemplateIDExt
		sec.ObservationElementPath = sec.EntryElementPath + "/" + t.ObservationElementPathSuffix
		if t.ObsFixedCode != "" {
			sec.ObsFixedCode = t.ObsFixedCode
			sec.ObsFixedCodeSystem = t.ObsFixedCodeSystem
			sec.ObsFixedCodeDisplay = t.ObsFixedCodeDisplay
		}
		if t.ObsCodeTranslationCode != "" {
			sec.ObsCodeTranslationCode = t.ObsCodeTranslationCode
			sec.ObsCodeTranslationSystem = t.ObsCodeTranslationSystem
			sec.ObsCodeTranslationDisplay = t.ObsCodeTranslationDisplay
		}
	}

	anchor := sec.ObservationElementPath
	if anchor == "" {
		anchor = sec.EntryElementPath
	}
	sec.Fields = resolveFields(disk.Fields, anchor)

	for _, a := range disk.StructuralTemplateIDs {
		resolved, err := resolveAnchor(a, constants)
		if err != nil {
			return nil, fmt.Errorf("structuralTemplateIds: %w", err)
		}
		sec.StructuralTemplateIDs = append(sec.StructuralTemplateIDs, resolved)
	}

	for _, alt := range disk.AlternateArchetypes {
		resolvedAlt := AlternateEntryArchetype{
			EntriesKey:         alt.EntriesKey,
			EntryElementPath:   alt.EntryElementPath,
			EntryTemplateID:    alt.EntryTemplateID,
			EntryTemplateIDExt: alt.EntryTemplateIDExt,
			Fields:             resolveFields(alt.Fields, alt.EntryElementPath),
		}
		for _, a := range alt.StructuralTemplateIDs {
			resolved, err := resolveAnchor(a, constants)
			if err != nil {
				return nil, fmt.Errorf("alternateArchetypes[%s].structuralTemplateIds: %w", alt.EntriesKey, err)
			}
			resolvedAlt.StructuralTemplateIDs = append(resolvedAlt.StructuralTemplateIDs, resolved)
		}
		sec.AlternateArchetypes = append(sec.AlternateArchetypes, resolvedAlt)
	}

	return sec, nil
}

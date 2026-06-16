// cda/schema_loader.go
// CDASchemaLoader loads C-CDA 2.1 and C32 schema files and provides indexed
// lookup by section key, template OID, and LOINC code.
//
// Usage:
//   loader, err := cda.NewCDASchemaLoader("/app/cda/schemas")
//   section := loader.GetSection("allergiesAndIntolerances")
//   field   := loader.GetField("allergiesAndIntolerances", "medicationAllergyCode")

package cda

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// CDASchemaLoader holds the loaded profile and C32 mapping, with thread-safe
// read access. Construct once at startup and share across goroutines.
type CDASchemaLoader struct {
	schemaDir  string
	profile    *CDAProfileDef
	c32Mapping *C32MappingFile

	// Inverted index: old C32 OID → C-CDA 2.1 OID for O(1) normalisation lookups.
	c32OIDIndex map[string]string

	// Set of all C32/HITSP detection OIDs.
	c32DetectOIDs map[string]struct{}

	mu sync.RWMutex
}

// NewCDASchemaLoader loads ccda_2_1.json and c32_mapping.json from schemaDir.
func NewCDASchemaLoader(schemaDir string) (*CDASchemaLoader, error) {
	l := &CDASchemaLoader{
		schemaDir:     schemaDir,
		c32OIDIndex:   make(map[string]string),
		c32DetectOIDs: make(map[string]struct{}),
	}

	if err := l.loadProfile(); err != nil {
		return nil, err
	}
	if err := l.loadC32Mapping(); err != nil {
		return nil, err
	}
	l.buildIndexes()
	return l, nil
}

// =====================================
// Public query API
// =====================================

// GetSection returns the section definition for the given key (e.g. "allergiesAndIntolerances").
// Returns nil if the section is not defined in the schema.
func (l *CDASchemaLoader) GetSection(key string) *CDASectionDef {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.profile.sectionByKey[key]
}

// GetSectionByTemplateID finds a section whose SHALL or MAY templateId matches the given OID.
func (l *CDASchemaLoader) GetSectionByTemplateID(oid string) *CDASectionDef {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.profile.sectionByTemplateID[oid]
}

// GetSectionByLOINC finds a section by its LOINC code (e.g. "48765-2" for allergies).
func (l *CDASchemaLoader) GetSectionByLOINC(code string) *CDASectionDef {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.profile.sectionByLOINC[code]
}

// GetField returns the field definition for the given section key and field key.
// Returns nil if either the section or field is not found.
func (l *CDASchemaLoader) GetField(sectionKey, fieldKey string) *CDAFieldDef {
	l.mu.RLock()
	defer l.mu.RUnlock()

	section, ok := l.profile.sectionByKey[sectionKey]
	if !ok {
		return nil
	}
	return section.fieldByKey[fieldKey]
}

// AllSections returns all section definitions in schema order.
func (l *CDASchemaLoader) AllSections() []*CDASectionDef {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.profile.Sections
}

// IsC32DetectionOID returns true if the OID is a known C32 or HITSP profile indicator.
func (l *CDASchemaLoader) IsC32DetectionOID(oid string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	_, found := l.c32DetectOIDs[oid]
	return found
}

// ResolveC32TemplateID returns the C-CDA 2.1 replacement OID for a C32 OID.
// Returns ("", false) if the OID is not in the C32 mapping.
func (l *CDASchemaLoader) ResolveC32TemplateID(oldOID string) (string, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	newOID, found := l.c32OIDIndex[oldOID]
	return newOID, found
}

// GetDocumentTypeSections returns the section conformance map for a given document
// type (e.g. "CCD", "Discharge Summary"). Returns nil if not defined.
func (l *CDASchemaLoader) GetDocumentTypeSections(docType string) *DocumentTypeSectionInfo {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.profile.DocumentTypeSections == nil {
		return nil
	}
	info, ok := l.profile.DocumentTypeSections[docType]
	if !ok {
		return nil
	}
	return &info
}

// AllDocumentTypes returns the document types defined in documentTypeSections.
func (l *CDASchemaLoader) AllDocumentTypes() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	result := make([]string, 0, len(l.profile.DocumentTypeSections))
	for dt := range l.profile.DocumentTypeSections {
		result = append(result, dt)
	}
	return result
}

// C32MappingCount returns the number of C32→C-CDA template OID mappings loaded.
func (l *CDASchemaLoader) C32MappingCount() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.c32OIDIndex)
}

// =====================================
// Internal loading
// =====================================

func (l *CDASchemaLoader) loadProfile() error {
	path := filepath.Join(l.schemaDir, "ccda_2_1.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cda: cannot read ccda_2_1.json from %s: %w", l.schemaDir, err)
	}

	var profile CDAProfileDef
	if err := json.Unmarshal(data, &profile); err != nil {
		return fmt.Errorf("cda: cannot parse ccda_2_1.json: %w", err)
	}
	l.profile = &profile
	return nil
}

func (l *CDASchemaLoader) loadC32Mapping() error {
	path := filepath.Join(l.schemaDir, "c32_mapping.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cda: cannot read c32_mapping.json from %s: %w", l.schemaDir, err)
	}

	var mapping C32MappingFile
	if err := json.Unmarshal(data, &mapping); err != nil {
		return fmt.Errorf("cda: cannot parse c32_mapping.json: %w", err)
	}
	l.c32Mapping = &mapping
	return nil
}

func (l *CDASchemaLoader) buildIndexes() {
	// Section indexes.
	l.profile.sectionByKey = make(map[string]*CDASectionDef)
	l.profile.sectionByTemplateID = make(map[string]*CDASectionDef)
	l.profile.sectionByLOINC = make(map[string]*CDASectionDef)

	for _, sec := range l.profile.Sections {
		l.profile.sectionByKey[sec.Key] = sec

		if sec.TemplateID != "" {
			l.profile.sectionByTemplateID[sec.TemplateID] = sec
		}
		if sec.TemplateIDOptional != "" {
			l.profile.sectionByTemplateID[sec.TemplateIDOptional] = sec
		}
		if sec.LOINCCode != "" {
			l.profile.sectionByLOINC[sec.LOINCCode] = sec
		}

		// Field index within this section.
		sec.fieldByKey = make(map[string]*CDAFieldDef)
		for _, field := range sec.Fields {
			sec.fieldByKey[field.Key] = field
		}
	}

	// C32 OID mapping index: old OID → new OID.
	for _, m := range l.c32Mapping.Mappings {
		for _, oldOID := range m.OldTemplateIDs {
			// First mapping wins if multiple rows share an old OID.
			if _, exists := l.c32OIDIndex[oldOID]; !exists {
				l.c32OIDIndex[oldOID] = m.NewTemplateID
			}
		}
	}

	// C32/HITSP document-level detection OIDs.
	for _, oid := range l.c32Mapping.ProfileDetection.C32TemplateIDs {
		l.c32DetectOIDs[oid] = struct{}{}
	}
	for _, oid := range l.c32Mapping.ProfileDetection.HITSPTemplateIDs {
		l.c32DetectOIDs[oid] = struct{}{}
	}
}

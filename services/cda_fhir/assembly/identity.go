// Package assembly implements post-mapping assembly rules that run over the
// complete set of FHIR resources produced by MapDocument — deduplication, panel
// synthesis, and other cross-resource operations — after all section goroutines
// have completed. None of the types in this package are thread-safe; they are
// designed to be used exclusively in the serial post-processing phase.
package assembly

import (
	"fmt"

	mappinglog "ezhealthkonnect/services/cda_fhir/mapping_log"
)

// IdentityKey is the HL7 CDAII deduplication key: root OID (the identifier system,
// e.g. "2.16.840.1.113883.4.6" for NPI) + extension (the actual value). Together
// they uniquely identify a clinical actor across CDA documents.
type IdentityKey struct {
	Root      string
	Extension string
}

// String returns a human-readable representation suitable for logging.
func (k IdentityKey) String() string {
	if k.Extension != "" {
		return k.Root + ":" + k.Extension
	}
	return k.Root
}

// IsZero returns true when both Root and Extension are empty (unusable for dedup).
func (k IdentityKey) IsZero() bool {
	return k.Root == "" && k.Extension == ""
}

// RegistryKey returns a key unique across all resource types:
// "ResourceType|root:extension" — allows the same CDA ID to exist on both a
// Practitioner and an Organization without false-positive dedup matches.
func (k IdentityKey) RegistryKey(resourceType string) string {
	return fmt.Sprintf("%s|%s:%s", resourceType, k.Root, k.Extension)
}

// EmbedIdentityKeys stores HL7 II identifiers in the reserved "_cdaIds" internal
// field of a FHIR resource map. The field is read by ExtractIdentityKeys and stripped
// from resources before FHIR emission (in the ID-renumber pass in document_mapper.go).
//
// ids is a raw list of map[string]interface{} entries, each having "root" and
// "extension" string keys — this avoids importing cdadocument from this package.
// Callers in the mappers package use their own private embedCDAIds helper that
// writes in this exact shape; this function is the authoritative reader contract.
func EmbedIdentityKeys(r map[string]interface{}, ids []map[string]interface{}) {
	if len(ids) == 0 {
		return
	}
	arr := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		root, _ := id["root"].(string)
		ext, _ := id["extension"].(string)
		if root == "" && ext == "" {
			continue
		}
		arr = append(arr, map[string]interface{}{
			"root":      root,
			"extension": ext,
		})
	}
	if len(arr) > 0 {
		r["_cdaIds"] = arr
	}
}

// ExtractIdentityKeys reads the "_cdaIds" internal field from a resource and returns
// all non-zero IdentityKeys. Returns nil when the field is absent or empty.
func ExtractIdentityKeys(r map[string]interface{}) []IdentityKey {
	raw, ok := r["_cdaIds"]
	if !ok {
		return nil
	}
	arr, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	var keys []IdentityKey
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		root, _ := m["root"].(string)
		ext, _ := m["extension"].(string)
		k := IdentityKey{Root: root, Extension: ext}
		if !k.IsZero() {
			keys = append(keys, k)
		}
	}
	return keys
}

// ExtractLineage reads the "_cdaSection"/"_cdaEntryIndex" internal fields (set by
// declarative_document_mapper.go's per-entry loop when deep lineage is enabled)
// together with the existing "_cdaIds" field, returning a populated
// mappinglog.ResourceLineage. ok is false when neither _cdaSection nor _cdaIds is
// present — deep lineage was off for this run, or this resource never carried CDA
// provenance (e.g. a synthesized resource with no CDA origin of its own).
func ExtractLineage(r map[string]interface{}) (lineage mappinglog.ResourceLineage, ok bool) {
	section, hasSection := r["_cdaSection"].(string)
	entryIdx, _ := r["_cdaEntryIndex"].(int)
	ids := ExtractIdentityKeys(r)

	if !hasSection && len(ids) == 0 {
		return mappinglog.ResourceLineage{}, false
	}

	lineage.SectionKey = section
	lineage.EntryIndex = entryIdx
	for _, id := range ids {
		lineage.CDAIds = append(lineage.CDAIds, id.String())
	}
	return lineage, true
}

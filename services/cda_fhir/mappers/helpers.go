package mappers

import cdadocument "ezhealthkonnect/cda/document"

// ref builds a FHIR Reference map from a reference string.
func ref(reference string) map[string]interface{} {
	return map[string]interface{}{"reference": reference}
}

// findRelByTypeCode searches entryRelationships for the first entry matching typeCode
// and returns a pointer to the nested entry. Returns nil when not found.
func findRelByTypeCode(rels []cdadocument.CDAEntryRelationship, typeCode string) *cdadocument.CDAEntry {
	for i := range rels {
		if rels[i].TypeCode == typeCode {
			return &rels[i].Entry
		}
	}
	return nil
}

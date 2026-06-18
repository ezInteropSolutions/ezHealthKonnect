package mappers

import cdadocument "ezhealthkonnect/cda/document"

// embedCDAIds writes HL7 II identifiers into the reserved "_cdaIds" internal field
// of a FHIR resource map. The assembly layer (assembly.ExtractIdentityKeys) reads
// this field for cross-section deduplication; document_mapper.go strips it before
// FHIR emission. No import of the assembly package is needed here — the field format
// is a plain []interface{} of {"root":..., "extension":...} maps.
func embedCDAIds(r map[string]interface{}, ids []cdadocument.CDAII) {
	if len(ids) == 0 {
		return
	}
	arr := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		if id.Root == "" && id.Extension == "" {
			continue
		}
		arr = append(arr, map[string]interface{}{
			"root":      id.Root,
			"extension": id.Extension,
		})
	}
	if len(arr) > 0 {
		r["_cdaIds"] = arr
	}
}

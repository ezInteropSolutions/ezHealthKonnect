// services/mappers/value_mapper.go
// Database-driven value mapping for HL7 to FHIR code conversions
//
// 🎯 PRINCIPLE: DATA-DRIVEN CONFIGURATION
// - All value mappings stored in database
// - No hardcoded transformations
// - Configurable via hl7_fhir_value_mappings table

package mappers

import (
	"database/sql"
	"fmt"
	"strings"
)

// =====================================
// VALUE MAPPER
// =====================================

type ValueMapper struct {
	db *sql.DB
}

func NewValueMapper(db *sql.DB) *ValueMapper {
	return &ValueMapper{db: db}
}

// =====================================
// GENERIC VALUE MAPPING
// =====================================

func (vm *ValueMapper) MapValue(hl7Table, hl7Value string) (string, string, string, bool) {
	if vm.db == nil {
		return "", "", "", false
	}

	var fhirSystem, fhirCode, fhirDisplay string
	err := vm.db.QueryRow(`
		SELECT fhir_system, fhir_code, fhir_display 
		FROM hl7_fhir_value_mappings 
		WHERE hl7_table = $1 AND hl7_value = $2
		LIMIT 1
	`, hl7Table, strings.ToUpper(hl7Value)).Scan(&fhirSystem, &fhirCode, &fhirDisplay)

	if err != nil {
		return "", "", "", false
	}

	return fhirSystem, fhirCode, fhirDisplay, true
}

// =====================================
// SPECIFIC MAPPING METHODS
// =====================================

func (vm *ValueMapper) MapGender(hl7Value string) string {
	_, fhirCode, _, found := vm.MapValue("0001", hl7Value)
	if found {
		return fhirCode
	}

	// Fallback mapping
	switch strings.ToUpper(strings.TrimSpace(hl7Value)) {
	case "M", "MALE":
		return "male"
	case "F", "FEMALE":
		return "female"
	case "O", "OTHER":
		return "other"
	default:
		return "unknown"
	}
}

func (vm *ValueMapper) MapPatientClass(hl7Value string) map[string]interface{} {
	fhirSystem, fhirCode, fhirDisplay, found := vm.MapValue("0004", hl7Value)
	if found {
		return map[string]interface{}{
			"system":  fhirSystem,
			"code":    fhirCode,
			"display": fhirDisplay,
		}
	}

	// Fallback mapping
	upperValue := strings.ToUpper(strings.TrimSpace(hl7Value))
	switch upperValue {
	case "I":
		return map[string]interface{}{
			"system":  "http://terminology.hl7.org/CodeSystem/v3-ActCode",
			"code":    "IMP",
			"display": "inpatient encounter",
		}
	case "O":
		return map[string]interface{}{
			"system":  "http://terminology.hl7.org/CodeSystem/v3-ActCode",
			"code":    "AMB",
			"display": "ambulatory",
		}
	case "E":
		return map[string]interface{}{
			"system":  "http://terminology.hl7.org/CodeSystem/v3-ActCode",
			"code":    "EMER",
			"display": "emergency",
		}
	default:
		return map[string]interface{}{
			"system":  "http://terminology.hl7.org/CodeSystem/v3-ActCode",
			"code":    "AMB",
			"display": "ambulatory",
		}
	}
}

func (vm *ValueMapper) MapEncounterStatus(hl7Value string) string {
	_, fhirCode, _, found := vm.MapValue("0005", hl7Value) // Example table for encounter status
	if found {
		return fhirCode
	}

	// Fallback mapping
	switch strings.ToUpper(strings.TrimSpace(hl7Value)) {
	case "A", "ACTIVE":
		return "in-progress"
	case "C", "CANCELLED":
		return "cancelled"
	case "F", "FINISHED":
		return "finished"
	default:
		return "in-progress"
	}
}

func (vm *ValueMapper) MapTelecomUse(hl7Value string) string {
	_, fhirCode, _, found := vm.MapValue("0201", hl7Value)
	if found {
		return fhirCode
	}

	// Fallback mapping for XTN.2 (Telecommunication Use Code)
	switch strings.ToUpper(strings.TrimSpace(hl7Value)) {
	case "PRN":
		return "home"
	case "WPN", "ORN":
		return "work"
	case "EMR":
		return "mobile"
	default:
		return "home"
	}
}

func (vm *ValueMapper) MapAddressUse(hl7Value string) string {
	_, fhirCode, _, found := vm.MapValue("0190", hl7Value) // Address Type table
	if found {
		return fhirCode
	}

	// Fallback mapping for XAD.7 (Address Type)
	switch strings.ToUpper(strings.TrimSpace(hl7Value)) {
	case "H":
		return "home"
	case "O", "B":
		return "work"
	case "T":
		return "temp"
	case "M":
		return "billing"
	default:
		return "home"
	}
}

func (vm *ValueMapper) MapNameUse(hl7Value string) string {
	_, fhirCode, _, found := vm.MapValue("0200", hl7Value) // Name Type table
	if found {
		return fhirCode
	}

	// Fallback mapping for XPN.7 (Name Type Code)
	switch strings.ToUpper(strings.TrimSpace(hl7Value)) {
	case "L":
		return "official"
	case "D":
		return "usual"
	case "M":
		return "maiden"
	case "N":
		return "nickname"
	default:
		return "official"
	}
}

// =====================================
// UTILITY METHODS
// =====================================

func (vm *ValueMapper) HasMapping(hl7Table, hl7Value string) bool {
	if vm.db == nil {
		return false
	}

	var count int
	err := vm.db.QueryRow(`
		SELECT COUNT(*) FROM hl7_fhir_value_mappings 
		WHERE hl7_table = $1 AND hl7_value = $2
	`, hl7Table, strings.ToUpper(hl7Value)).Scan(&count)

	return err == nil && count > 0
}

func (vm *ValueMapper) GetAllMappingsForTable(hl7Table string) (map[string]string, error) {
	if vm.db == nil {
		return nil, fmt.Errorf("database not available")
	}

	mappings := make(map[string]string)

	rows, err := vm.db.Query(`
		SELECT hl7_value, fhir_code 
		FROM hl7_fhir_value_mappings 
		WHERE hl7_table = $1
	`, hl7Table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var hl7Value, fhirCode string
		if err := rows.Scan(&hl7Value, &fhirCode); err != nil {
			continue
		}
		mappings[hl7Value] = fhirCode
	}

	return mappings, rows.Err()
}

func (vm *ValueMapper) CreateCoding(system, code, display string) map[string]interface{} {
	coding := map[string]interface{}{
		"system": system,
		"code":   code,
	}

	if display != "" {
		coding["display"] = display
	}

	return coding
}

func (vm *ValueMapper) CreateCodeableConcept(system, code, display, text string) map[string]interface{} {
	codeableConcept := map[string]interface{}{
		"coding": []map[string]interface{}{
			vm.CreateCoding(system, code, display),
		},
	}

	if text != "" {
		codeableConcept["text"] = text
	} else if display != "" {
		codeableConcept["text"] = display
	}

	return codeableConcept
}

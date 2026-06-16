// services/cda_terminology/terminology_service.go
// TerminologyService — code validation and optional translation for CDA→FHIR processing.
//
// Three components:
//   CodeValidator      — format-only checks (no VSAC call) for SNOMED, LOINC, RxNorm, CVX,
//                        NDC, ICD-10-CM, CPT, and NCI Thesaurus codes.
//   TranslationRepository — CRUD against cda_code_translations (per-interface + global rows).
//   TerminologyService — facade that combines validation + translation.
//
// Mirrors the DI pattern used throughout the codebase: constructor injection,
// nil receiver safe (passthrough behaviour when no DB available).

package cdaterminology

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// =========================================================
// Validation types
// =========================================================

// ValidationResult is the outcome of a code validation check.
type ValidationResult struct {
	Valid   bool   `json:"valid"`
	System  string `json:"system"`
	Code    string `json:"code"`
	Message string `json:"message,omitempty"` // populated only on failure
}

// =========================================================
// CodeValidator — format-only checks
// =========================================================

// CodeValidator checks code format against known patterns for each code system.
// It does NOT call VSAC or any external service — this is offline format validation.
type CodeValidator struct{}

// NewCodeValidator returns a CodeValidator.
func NewCodeValidator() *CodeValidator { return &CodeValidator{} }

// Validate checks whether code is well-formed for the given system URI.
// Unknown systems are passed through as valid (passthrough mode).
func (v *CodeValidator) Validate(systemURI, code string) ValidationResult {
	code = strings.TrimSpace(code)
	if code == "" {
		return ValidationResult{Valid: false, System: systemURI, Code: code, Message: "empty code"}
	}

	switch systemURI {
	case "http://snomed.info/sct":
		return v.validateSNOMED(systemURI, code)
	case "http://www.nlm.nih.gov/research/umls/rxnorm":
		return v.validateRxNorm(systemURI, code)
	case "http://loinc.org":
		return v.validateLOINC(systemURI, code)
	case "http://hl7.org/fhir/sid/cvx":
		return v.validateCVX(systemURI, code)
	case "http://hl7.org/fhir/sid/ndc":
		return v.validateNDC(systemURI, code)
	case "http://hl7.org/fhir/sid/icd-10-cm":
		return v.validateICD10CM(systemURI, code)
	case "http://www.ama-assn.org/go/cpt":
		return v.validateCPT(systemURI, code)
	case "http://ncithesaurus.nci.nih.gov":
		return v.validateNCIT(systemURI, code)
	default:
		// Unknown system — passthrough
		return ValidationResult{Valid: true, System: systemURI, Code: code}
	}
}

var (
	reSNOMED   = regexp.MustCompile(`^\d{6,18}$`)
	reRxNorm   = regexp.MustCompile(`^\d{1,7}$`)
	reLOINC    = regexp.MustCompile(`^\d{1,5}-\d$`)
	reCVX      = regexp.MustCompile(`^\d{1,3}$`)
	reNDC      = regexp.MustCompile(`^\d{10,11}$|^\d{5}-\d{4}-\d{2}$|^\d{5}-\d{3,4}-\d{1,2}$`)
	reICD10CM  = regexp.MustCompile(`^[A-Z]\d{2}(\.\d{1,4})?$`)
	reCPT      = regexp.MustCompile(`^\d{5}$|^\d{4}[A-Z]$`)
	reNCIT     = regexp.MustCompile(`^C\d{4,7}$`)
)

func (v *CodeValidator) validateSNOMED(sys, code string) ValidationResult {
	if !reSNOMED.MatchString(code) {
		return ValidationResult{Valid: false, System: sys, Code: code,
			Message: "SNOMED CT code must be 6–18 digits"}
	}
	return ValidationResult{Valid: true, System: sys, Code: code}
}

func (v *CodeValidator) validateRxNorm(sys, code string) ValidationResult {
	if !reRxNorm.MatchString(code) {
		return ValidationResult{Valid: false, System: sys, Code: code,
			Message: "RxNorm code must be 1–7 digits"}
	}
	return ValidationResult{Valid: true, System: sys, Code: code}
}

func (v *CodeValidator) validateLOINC(sys, code string) ValidationResult {
	if !reLOINC.MatchString(code) {
		return ValidationResult{Valid: false, System: sys, Code: code,
			Message: "LOINC code must match nnnnn-n pattern (e.g. 48765-2)"}
	}
	return ValidationResult{Valid: true, System: sys, Code: code}
}

func (v *CodeValidator) validateCVX(sys, code string) ValidationResult {
	if !reCVX.MatchString(code) {
		return ValidationResult{Valid: false, System: sys, Code: code,
			Message: "CVX code must be 1–3 digits"}
	}
	return ValidationResult{Valid: true, System: sys, Code: code}
}

func (v *CodeValidator) validateNDC(sys, code string) ValidationResult {
	if !reNDC.MatchString(code) {
		return ValidationResult{Valid: false, System: sys, Code: code,
			Message: "NDC code must be 10–11 digits or nnnnn-nnnn-nn format"}
	}
	return ValidationResult{Valid: true, System: sys, Code: code}
}

func (v *CodeValidator) validateICD10CM(sys, code string) ValidationResult {
	if !reICD10CM.MatchString(strings.ToUpper(code)) {
		return ValidationResult{Valid: false, System: sys, Code: code,
			Message: "ICD-10-CM code must match letter + 2 digits (+ optional decimal suffix)"}
	}
	return ValidationResult{Valid: true, System: sys, Code: code}
}

func (v *CodeValidator) validateCPT(sys, code string) ValidationResult {
	if !reCPT.MatchString(code) {
		return ValidationResult{Valid: false, System: sys, Code: code,
			Message: "CPT code must be 5 digits or 4 digits + letter"}
	}
	return ValidationResult{Valid: true, System: sys, Code: code}
}

func (v *CodeValidator) validateNCIT(sys, code string) ValidationResult {
	if !reNCIT.MatchString(strings.ToUpper(code)) {
		return ValidationResult{Valid: false, System: sys, Code: code,
			Message: "NCI Thesaurus code must match C followed by 4–7 digits (e.g. C12345)"}
	}
	return ValidationResult{Valid: true, System: sys, Code: code}
}

// =========================================================
// Translation types
// =========================================================

// CodeTranslation is one row in cda_code_translations.
type CodeTranslation struct {
	ID            string    `json:"id"`
	InterfaceID   *string   `json:"interfaceId,omitempty"` // nil = global
	SourceSystem  string    `json:"sourceSystem"`
	SourceCode    string    `json:"sourceCode"`
	TargetSystem  string    `json:"targetSystem"`
	TargetCode    string    `json:"targetCode"`
	TargetDisplay string    `json:"targetDisplay"`
	CreatedAt     time.Time `json:"createdAt"`
}

// =========================================================
// TranslationRepository — CRUD against cda_code_translations
// =========================================================

// TranslationRepository performs CRUD operations on the cda_code_translations table.
type TranslationRepository struct {
	db *sql.DB
}

// NewTranslationRepository constructs the repository. db may be nil — all methods
// return ErrNoTranslation when db is nil (safe passthrough).
func NewTranslationRepository(db *sql.DB) *TranslationRepository {
	return &TranslationRepository{db: db}
}

// ErrNoTranslation is returned when no matching translation row exists.
var ErrNoTranslation = fmt.Errorf("cda_terminology: no translation found")

// Translate looks up the target code for sourceSystem/sourceCode → targetSystem.
// interfaceID can be empty string to search global rows only.
// Resolution order: interface-specific row → global row → ErrNoTranslation.
func (r *TranslationRepository) Translate(
	ctx context.Context,
	interfaceID, sourceSystem, sourceCode, targetSystem string,
) (string, error) {
	if r.db == nil {
		return "", ErrNoTranslation
	}

	// Try interface-specific first (only when interfaceID provided)
	if interfaceID != "" {
		var targetCode string
		err := r.db.QueryRowContext(ctx, `
			SELECT target_code
			FROM cda_code_translations
			WHERE interface_id = $1
			  AND source_system = $2
			  AND source_code   = $3
			  AND target_system = $4
			LIMIT 1
		`, interfaceID, sourceSystem, sourceCode, targetSystem).Scan(&targetCode)
		if err == nil {
			return targetCode, nil
		}
		if err != sql.ErrNoRows {
			return "", fmt.Errorf("cda_terminology: translate query: %w", err)
		}
	}

	// Fall back to global rows (interface_id IS NULL)
	var targetCode string
	err := r.db.QueryRowContext(ctx, `
		SELECT target_code
		FROM cda_code_translations
		WHERE interface_id IS NULL
		  AND source_system = $1
		  AND source_code   = $2
		  AND target_system = $3
		LIMIT 1
	`, sourceSystem, sourceCode, targetSystem).Scan(&targetCode)
	if err == sql.ErrNoRows {
		return "", ErrNoTranslation
	}
	if err != nil {
		return "", fmt.Errorf("cda_terminology: translate fallback query: %w", err)
	}
	return targetCode, nil
}

// List returns all translation rows for a given interface (plus global rows).
func (r *TranslationRepository) List(ctx context.Context, interfaceID string) ([]CodeTranslation, error) {
	if r.db == nil {
		return nil, nil
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, interface_id, source_system, source_code,
		       target_system, target_code, COALESCE(target_display,''), created_at
		FROM cda_code_translations
		WHERE interface_id = $1 OR interface_id IS NULL
		ORDER BY source_system, source_code
	`, toNullString(interfaceID))
	if err != nil {
		return nil, fmt.Errorf("cda_terminology: list translations: %w", err)
	}
	defer rows.Close()

	var result []CodeTranslation
	for rows.Next() {
		var t CodeTranslation
		var ifaceID sql.NullString
		if scanErr := rows.Scan(&t.ID, &ifaceID, &t.SourceSystem, &t.SourceCode,
			&t.TargetSystem, &t.TargetCode, &t.TargetDisplay, &t.CreatedAt); scanErr != nil {
			continue
		}
		if ifaceID.Valid {
			t.InterfaceID = &ifaceID.String
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

// Upsert inserts or updates a translation row.
func (r *TranslationRepository) Upsert(ctx context.Context, t CodeTranslation) error {
	if r.db == nil {
		return fmt.Errorf("cda_terminology: no DB connection")
	}
	var ifaceID interface{}
	if t.InterfaceID != nil && *t.InterfaceID != "" {
		ifaceID = *t.InterfaceID
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO cda_code_translations
		    (interface_id, source_system, source_code, target_system, target_code, target_display)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (interface_id, source_system, source_code, target_system)
		DO UPDATE SET target_code = EXCLUDED.target_code,
		              target_display = EXCLUDED.target_display
	`, ifaceID, t.SourceSystem, t.SourceCode, t.TargetSystem, t.TargetCode, t.TargetDisplay)
	if err != nil {
		return fmt.Errorf("cda_terminology: upsert translation: %w", err)
	}
	return nil
}

// Delete removes a translation row by ID.
func (r *TranslationRepository) Delete(ctx context.Context, id string) error {
	if r.db == nil {
		return fmt.Errorf("cda_terminology: no DB connection")
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM cda_code_translations WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("cda_terminology: delete translation: %w", err)
	}
	return nil
}

// =========================================================
// TerminologyService — facade
// =========================================================

// TerminologyService combines code validation and optional translation.
// Both operations are best-effort: failures are surfaced as ValidationResult.Message
// or ErrNoTranslation, never panics.
type TerminologyService struct {
	validator  *CodeValidator
	translator *TranslationRepository
}

// NewTerminologyService constructs the service.
// db may be nil; translation lookups will always return ErrNoTranslation in that case.
func NewTerminologyService(db *sql.DB) *TerminologyService {
	return &TerminologyService{
		validator:  NewCodeValidator(),
		translator: NewTranslationRepository(db),
	}
}

// Validate checks whether code is well-formed for the given system URI.
func (s *TerminologyService) Validate(systemURI, code string) ValidationResult {
	if s == nil {
		return ValidationResult{Valid: true, System: systemURI, Code: code}
	}
	return s.validator.Validate(systemURI, code)
}

// Translate looks up a code translation for the given interface.
// Returns ErrNoTranslation when no row exists — caller should use original code.
func (s *TerminologyService) Translate(
	ctx context.Context,
	interfaceID, sourceSystem, sourceCode, targetSystem string,
) (string, error) {
	if s == nil {
		return "", ErrNoTranslation
	}
	return s.translator.Translate(ctx, interfaceID, sourceSystem, sourceCode, targetSystem)
}

// ListTranslations returns all translations visible to an interface.
func (s *TerminologyService) ListTranslations(ctx context.Context, interfaceID string) ([]CodeTranslation, error) {
	if s == nil {
		return nil, nil
	}
	return s.translator.List(ctx, interfaceID)
}

// UpsertTranslation inserts or updates a translation row.
func (s *TerminologyService) UpsertTranslation(ctx context.Context, t CodeTranslation) error {
	if s == nil {
		return fmt.Errorf("cda_terminology: service not initialised")
	}
	return s.translator.Upsert(ctx, t)
}

// DeleteTranslation removes a translation row.
func (s *TerminologyService) DeleteTranslation(ctx context.Context, id string) error {
	if s == nil {
		return fmt.Errorf("cda_terminology: service not initialised")
	}
	return s.translator.Delete(ctx, id)
}

// =========================================================
// Helpers
// =========================================================

func toNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

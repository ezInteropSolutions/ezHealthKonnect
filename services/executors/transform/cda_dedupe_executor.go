// services/executors/transform/cda_dedupe_executor.go
// CDADedupeExecutor — pipeline step type "cda.dedupe".
//
// Removes duplicate clinical entries WITHIN a single parsed CDA/CCD document
// (not across messages/interfaces — that would need a persistent identity
// registry, a separate, larger feature). Matches entries by clinically
// meaningful identity per section (cdaDedupeIdentityRules,
// cda_dedupe_templates.go), not remove_duplicates' generic full-record-or-
// configured-field hash: a CDA entry's real identity (e.g. an allergy's
// substance code) typically sits several entryRelationships[typeCode=X] hops
// deep, which only the CDA-specific path grammar (ResolveCDAPath,
// cda_path_resolver.go) can resolve.
//
// Unlike cda.section_to_csv, this step MUTATES the typed *CDADocument in
// place (SectionsByKey[key].Entries is filtered down) and passes the SAME
// pointer forward as _cdaDocument, so a later cda.to_fhir or
// cda.section_to_csv step sees the deduplicated entries directly. Typical
// placement: connector.inbound → cda.parse → cda.dedupe → cda.to_fhir.
//
// The OOB identity rule per section can be overridden per step via
// config.overrides.<sectionKey>.keyPaths (cdaDedupeOverride) — replacing the
// built-in rule, or defining one from scratch for a section with no OOB rule
// at all. This is step-scoped config, not a persisted per-interface setting:
// edit it directly in the step's JSON config.
//
// V1 scope: no "merge" strategy (unlike remove_duplicates) — deliberately.
// A generic shallow-field merge of two CDAEntry structs risks silently
// combining data from two entries whose identity match was a heuristic, not
// a certainty (e.g. two medications with the same code+start date that are
// coincidentally similar but clinically distinct) — a real risk for
// patient-safety-sensitive data that a flat JSON merge (safe for arbitrary
// pipeline records) doesn't carry the same way. "first"/"last" only.
//
// crossMessage mode extends matching ACROSS separate documents for the same
// patient, backed by the cda_dedupe_registry table (V191 migration) — see
// checkAndRegisterCrossMessage. Requires patientIdentifierRoot (which CDA
// <id> root OID counts as "the patient identifier" for this interface) to be
// configured explicitly; there is no auto-detection, since guessing which of
// a patient's several identifiers (MRN/SSN/Medicare/etc.) is the reliable one
// is exactly the kind of silent guess clinical data shouldn't be built on.
package transform

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	cdaparser "ezhealthkonnect/services/parsers/cda"
)

type CDADedupeExecutor struct {
	*executors.BaseExecutor
	parserService *cdaparser.CDAParserService // used for auto-parse when _cdaDocument is absent
	db            *sql.DB                     // used only when config.crossMessage is true
}

func NewCDADedupeExecutor(db *sql.DB) *CDADedupeExecutor {
	exec := &CDADedupeExecutor{
		BaseExecutor: executors.NewBaseExecutor("cda.dedupe", models.ExecutorMetadata{
			Name:        "CDA Deduplication",
			Description: "Finds and removes duplicate clinical entries in a document, like the same allergy or medication listed twice — matched by the details that actually identify each entry (not just comparing raw text). Optionally also across separate documents for the same patient.",
			Version:     "1.1.0",
			Author:      "ezHealthKonnect",
			Category:    "CDA Transform",
		}),
		db: db,
	}
	if svc, err := cdaparser.NewFromSchemaDir("./cda/schemas"); err == nil {
		exec.parserService = svc
	} else {
		log.Printf("⚠️  [cda.dedupe] CDAParserService init failed (%v) — auto-parse disabled, requires an upstream cda.parse step", err)
	}
	return exec
}

type cdaDedupeConfig struct {
	SourceField           string                       `json:"sourceField"`
	Sections              []string                     `json:"sections"`
	Strategy              string                       `json:"strategy"`  // "first" (default) | "last"
	Overrides             map[string]cdaDedupeOverride `json:"overrides"` // per-section identity-key override; see cdaDedupeOverride
	CrossMessage          bool                         `json:"crossMessage"`
	PatientIdentifierRoot string                       `json:"patientIdentifierRoot"` // required when crossMessage is true
	// TrackSuppressionLineage controls whether a cross-message suppression
	// attaches per-entry lineage (identity_key + first_seen_message_id/at) to
	// this step's output, on top of the plain removed-count that's always
	// present. Pointer so nil (key omitted) means "on" — the default — and
	// only an explicit false opts out, e.g. for an org that wants to minimize
	// what per-entry detail gets captured in step_executions.step_output.
	TrackSuppressionLineage *bool `json:"trackSuppressionLineage"`
}

// cdaDedupeOverride replaces the OOB identity rule (cdaDedupeIdentityRules)
// for one section with user-supplied CDA paths — or DEFINES one for a
// section that has no OOB rule at all, letting a user extend dedupe to a
// section this step doesn't ship a built-in rule for. KeyPaths use the same
// CDA path grammar as the OOB rules (ResolveCDAPath, cda_path_resolver.go),
// relative to one section entry.
type cdaDedupeOverride struct {
	KeyPaths []string `json:"keyPaths"`
}

func (e *CDADedupeExecutor) Execute(ctx context.Context, step *models.TransformationStep, inputData map[string]interface{}) (map[string]interface{}, error) {
	start := time.Now()
	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}

	cfg := cdaDedupeConfig{Strategy: "first"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		json.Unmarshal(raw, &cfg) //nolint:errcheck
	}
	if cfg.Strategy != "last" {
		cfg.Strategy = "first"
	}

	sectionKeys := cfg.Sections
	if len(sectionKeys) == 0 {
		sectionKeys = SupportedCDADedupeSections()
	}

	doc, err := e.resolveDocument(inputData, cfg.SourceField)
	if err != nil {
		return nil, fmt.Errorf("cda.dedupe: %w", err)
	}

	outputData := make(map[string]interface{}, len(inputData)+1)
	for k, v := range inputData {
		outputData[k] = v
	}

	// Resolved once per document, only when crossMessage mode is on. A
	// missing patient identifier or interface_id degrades gracefully (log +
	// skip cross-message for this run, within-document dedup still applies)
	// — it is NOT a hard failure, since it reflects the source document's
	// own data, not a system error.
	var interfaceID, patientKey string
	var patientKeyFound bool
	if cfg.CrossMessage {
		interfaceID = resolveInterfaceIDForDedupe(inputData, ctx)
		if interfaceID == "" {
			log.Printf("⚠️  [cda.dedupe] crossMessage enabled but interface_id could not be resolved — cross-message check skipped for this message")
		}
		patientKey, patientKeyFound = resolvePatientIdentifierKey(doc, cfg.PatientIdentifierRoot)
		if !patientKeyFound {
			log.Printf("⚠️  [cda.dedupe] crossMessage enabled but no patient identifier found for root %q — cross-message check skipped for this message", cfg.PatientIdentifierRoot)
		}
	}
	messageID := resolveMessageIDForDedupe(inputData, ctx)
	crossMessageActive := cfg.CrossMessage && interfaceID != "" && patientKeyFound
	trackLineage := cfg.TrackSuppressionLineage == nil || *cfg.TrackSuppressionLineage

	sectionStats := make(map[string]interface{}, len(sectionKeys))
	totalRemoved := 0
	for _, sectionKey := range sectionKeys {
		rule, hasOOBRule := cdaDedupeIdentityRules[sectionKey]
		if override, hasOverride := cfg.Overrides[sectionKey]; hasOverride && len(override.KeyPaths) > 0 {
			// Override replaces the OOB rule, or defines identity from scratch
			// for a section with no OOB rule at all.
			rule = DedupeIdentityRule{SectionKey: sectionKey, KeyPaths: override.KeyPaths}
		} else if !hasOOBRule {
			continue
		}
		section := doc.SectionsByKey[sectionKey]
		if section == nil || len(section.Entries) == 0 {
			continue
		}
		originalCount := len(section.Entries)
		deduped, removed := dedupeSectionEntries(section.Entries, rule, cfg.Strategy)

		crossRemoved := 0
		var crossSuppressed []suppressedCrossMessageEntry
		if crossMessageActive {
			var crossErr error
			deduped, crossRemoved, crossSuppressed, crossErr = e.applyCrossMessageDedupe(ctx, interfaceID, patientKey, sectionKey, messageID, deduped, rule, trackLineage)
			if crossErr != nil {
				return nil, fmt.Errorf("cda.dedupe: cross-message check failed for section %s: %w", sectionKey, crossErr)
			}
		}

		section.Entries = deduped
		stats := map[string]interface{}{
			"original_count":        originalCount,
			"dedup_count":           len(deduped),
			"removed_count":         removed + crossRemoved,
			"in_document_removed":   removed,
			"cross_message_removed": crossRemoved,
		}
		// Per-entry lineage — which identity was dropped and which earlier
		// message first delivered it — only present when something was
		// actually cross-message-suppressed for this section.
		if len(crossSuppressed) > 0 {
			stats["cross_message_suppressed"] = crossSuppressed
		}
		sectionStats[sectionKey] = stats
		totalRemoved += removed + crossRemoved
	}

	// Same pointer, now mutated — written into the shared message object so
	// a following cda.to_fhir/cda.section_to_csv step's findCDADocument()
	// picks up the deduplicated entries directly (cda_document_resolution.go).
	setCDADocument(outputData, doc)

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [cda.dedupe] Removed %d duplicate entries across %d section(s) in %dms (cross_message_active=%v)", totalRemoved, len(sectionStats), durationMs, crossMessageActive)
	e.SetStepOutputWithDetails(outputData, map[string]interface{}{
		"section_stats":         sectionStats,
		"total_removed":         totalRemoved,
		"cross_message_active":  crossMessageActive,
		"patient_key_resolved":  patientKeyFound,
	}, map[string]interface{}{
		"duration_ms": durationMs, "success": true, "total_removed": totalRemoved, "sections_processed": len(sectionStats),
	})
	return outputData, nil
}

// resolveInterfaceIDForDedupe mirrors cda_to_fhir_executor.go's own
// three-tier interface-id lookup (inputData["interfaceId"] ->
// inputData["interface_id"] -> ctx.Value("interface_id")) — kept as a
// separate copy rather than extracting a shared helper, to avoid touching
// cda_to_fhir_executor.go's already-tested logic for an unrelated change.
func resolveInterfaceIDForDedupe(inputData map[string]interface{}, ctx context.Context) string {
	if v, ok := inputData["interfaceId"].(string); ok && v != "" {
		return v
	}
	if v, ok := inputData["interface_id"].(string); ok && v != "" {
		return v
	}
	if v, ok := ctx.Value("interface_id").(string); ok {
		return v
	}
	return ""
}

// resolveMessageIDForDedupe is best-effort audit metadata only (the registry's
// first/last-seen message id columns) — never required for the dedup MATCH
// itself, which is keyed by interface+patient+section+identity alone.
func resolveMessageIDForDedupe(inputData map[string]interface{}, ctx context.Context) string {
	if v, ok := inputData["_messageId"].(string); ok && v != "" {
		return v
	}
	if v, ok := ctx.Value("message_id").(string); ok {
		return v
	}
	return ""
}

// resolvePatientIdentifierKey finds the patient's identifier extension for
// the configured root OID. Returns ("", false) if the patient has no
// identifier with that root in THIS document — the caller must not guess a
// substitute; cross-message dedup is skipped for that run instead.
func resolvePatientIdentifierKey(doc *cdadocument.CDADocument, patientIdentifierRoot string) (string, bool) {
	if patientIdentifierRoot == "" {
		return "", false
	}
	for _, id := range doc.Header.Patient.Ids {
		if id.Root == patientIdentifierRoot && id.Extension != "" {
			return id.Extension, true
		}
	}
	return "", false
}

// suppressedCrossMessageEntry records why one entry was dropped by
// crossMessage dedup — surfaced in step output (section_stats.<key>.
// cross_message_suppressed) so a message's own processing trace answers
// "what got dropped, and which earlier message first delivered it" directly,
// without a separate lookup into cda_dedupe_registry.
type suppressedCrossMessageEntry struct {
	IdentityKey        string `json:"identity_key"`
	FirstSeenMessageID string `json:"first_seen_message_id"`
	FirstSeenAt        string `json:"first_seen_at"`
}

// applyCrossMessageDedupe checks each already-in-document-deduplicated entry
// against the persistent registry (cda_dedupe_registry), atomically
// registering new identities and dropping ones already seen in an earlier
// message for the same patient. Entries whose identity key can't be resolved
// are always kept, never checked against the registry — same "never dedupe
// on missing data" rule as within-document matching.
//
// trackLineage controls only whether a suppressed entry's identity/lineage
// detail is captured into the returned slice — the registry check and the
// suppression decision itself happen identically either way.
func (e *CDADedupeExecutor) applyCrossMessageDedupe(
	ctx context.Context, interfaceID, patientKey, sectionKey, messageID string,
	entries []cdadocument.CDAEntry, rule DedupeIdentityRule, trackLineage bool,
) ([]cdadocument.CDAEntry, int, []suppressedCrossMessageEntry, error) {
	result := make([]cdadocument.CDAEntry, 0, len(entries))
	removed := 0
	var suppressed []suppressedCrossMessageEntry
	for _, entry := range entries {
		entryMap, err := marshalEntryMap(entry)
		if err != nil {
			result = append(result, entry)
			continue
		}
		parts := make([]string, 0, len(rule.KeyPaths))
		hasNull := false
		for _, path := range rule.KeyPaths {
			val := executors.ResolveCDAPath(entryMap, path, false)
			s := cdaValueToCSVString(val)
			if s == "" {
				hasNull = true
			}
			parts = append(parts, path+"="+s)
		}
		if hasNull {
			result = append(result, entry)
			continue
		}
		identityKey := strings.Join(parts, "|")

		isNew, firstSeenMessageID, firstSeenAt, err := e.checkAndRegisterCrossMessage(ctx, interfaceID, patientKey, sectionKey, identityKey, messageID)
		if err != nil {
			return nil, 0, nil, err
		}
		if isNew {
			result = append(result, entry)
		} else {
			removed++
			if trackLineage {
				suppressed = append(suppressed, suppressedCrossMessageEntry{
					IdentityKey:        identityKey,
					FirstSeenMessageID: firstSeenMessageID,
					FirstSeenAt:        firstSeenAt.UTC().Format(time.RFC3339),
				})
			}
		}
	}
	return result, removed, suppressed, nil
}

// checkAndRegisterCrossMessage atomically checks whether
// (interfaceID, patientKey, sectionKey, identityKey) has been seen before,
// inserting a new registry row if not. Uses Postgres's "xmax = 0" trick to
// detect INSERT vs. UPDATE in a single round trip, avoiding a check-then-
// insert race under concurrent message processing for the same patient.
//
// Also returns first_seen_message_id/first_seen_at in the SAME round trip —
// on a suppression (isNew=false) this is the lineage data the caller needs to
// answer "where did this fact first come from," surfaced in step output
// (applyCrossMessageDedupe) instead of requiring a separate lookup into
// cda_dedupe_registry to find out.
func (e *CDADedupeExecutor) checkAndRegisterCrossMessage(
	ctx context.Context, interfaceID, patientKey, sectionKey, identityKey, messageID string,
) (isNew bool, firstSeenMessageID string, firstSeenAt time.Time, err error) {
	if e.db == nil {
		return false, "", time.Time{}, fmt.Errorf("crossMessage requires a database connection, none configured")
	}
	const query = `
		INSERT INTO cda_dedupe_registry
			(interface_id, patient_key, section_key, identity_key, first_seen_message_id, last_seen_message_id)
		VALUES ($1, $2, $3, $4, $5, $5)
		ON CONFLICT (interface_id, patient_key, section_key, identity_key)
		DO UPDATE SET
			last_seen_message_id = $5,
			last_seen_at = NOW(),
			seen_count = cda_dedupe_registry.seen_count + 1,
			updated_at = NOW()
		RETURNING (xmax = 0) AS inserted, COALESCE(first_seen_message_id, ''), first_seen_at
	`
	row := e.db.QueryRowContext(ctx, query, interfaceID, patientKey, sectionKey, identityKey, messageID)
	if err := row.Scan(&isNew, &firstSeenMessageID, &firstSeenAt); err != nil {
		return false, "", time.Time{}, err
	}
	return isNew, firstSeenMessageID, firstSeenAt, nil
}

// resolveDocument prefers the typed *CDADocument left inside the shared
// message object by an earlier cda.parse step (findCDADocument,
// cda_document_resolution.go); falls back to auto-parsing raw CDA XML the
// same way cda.to_fhir/cda.section_to_csv do. Returns the TYPED document
// (not a marshaled map) because deduplication mutates it in place.
func (e *CDADedupeExecutor) resolveDocument(inputData map[string]interface{}, sourceField string) (*cdadocument.CDADocument, error) {
	if doc, ok := findCDADocument(inputData); ok {
		return doc, nil
	}
	if e.parserService == nil {
		return nil, fmt.Errorf("no typed CDA document available (_cdaDocument) and auto-parse is disabled (schema directory unavailable)")
	}

	rawXML := ""
	if sourceField != "" {
		if v, ok := executors.GetFieldValue(inputData, sourceField).(string); ok {
			rawXML = v
		}
	}
	if rawXML == "" {
		candidates := []string{"content", "raw_content", "rawMessage", "rawXML", "cdaXML", "raw"}
		if msg, ok := inputData["message"].(map[string]interface{}); ok {
			for _, k := range candidates {
				if v, ok := msg[k].(string); ok && strings.Contains(v, "ClinicalDocument") {
					rawXML = v
					break
				}
			}
		}
		if rawXML == "" {
			for _, k := range candidates {
				if v, ok := inputData[k].(string); ok && strings.Contains(v, "ClinicalDocument") {
					rawXML = v
					break
				}
			}
		}
	}
	if rawXML == "" {
		return nil, fmt.Errorf("no typed CDA document available and no raw CDA XML found (expected _cdaDocument or a raw XML field)")
	}

	result := e.parserService.Parse(rawXML)
	if !result.Success {
		return nil, fmt.Errorf("auto-parse failed: %s", result.Error)
	}
	doc, ok := result.TypedDocument.(*cdadocument.CDADocument)
	if !ok {
		return nil, fmt.Errorf("auto-parse produced no typed document")
	}
	return doc, nil
}

// dedupeSectionEntries applies rule's composite identity key to every entry
// and returns the filtered slice plus how many were removed. Entries whose
// key has ANY unresolved (empty) component are never deduplicated — safer
// default for clinical data than risking a false-positive match.
func dedupeSectionEntries(entries []cdadocument.CDAEntry, rule DedupeIdentityRule, strategy string) ([]cdadocument.CDAEntry, int) {
	type keyed struct {
		entry      cdadocument.CDAEntry
		key        string
		hasNullKey bool
	}

	items := make([]keyed, len(entries))
	for i, entry := range entries {
		entryMap, err := marshalEntryMap(entry)
		hasNull := err != nil
		key := ""
		if err == nil {
			parts := make([]string, 0, len(rule.KeyPaths))
			for _, path := range rule.KeyPaths {
				val := executors.ResolveCDAPath(entryMap, path, false)
				s := cdaValueToCSVString(val)
				if s == "" {
					hasNull = true
				}
				parts = append(parts, path+"="+s)
			}
			key = strings.Join(parts, "|")
		}
		items[i] = keyed{entry: entry, key: key, hasNullKey: hasNull}
	}

	seen := make(map[string]int, len(items)) // key -> index in result
	result := make([]cdadocument.CDAEntry, 0, len(items))
	removed := 0
	for _, it := range items {
		if it.hasNullKey {
			result = append(result, it.entry)
			continue
		}
		if existingIdx, ok := seen[it.key]; ok {
			removed++
			if strategy == "last" {
				result[existingIdx] = it.entry
			}
			continue
		}
		seen[it.key] = len(result)
		result = append(result, it.entry)
	}
	return result, removed
}

// marshalEntryMap converts one typed CDAEntry to a generic map so
// ResolveCDAPath (which operates on map[string]interface{}) can resolve
// identity-key paths against it.
func marshalEntryMap(entry cdadocument.CDAEntry) (map[string]interface{}, error) {
	b, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (e *CDADedupeExecutor) Validate(step *models.TransformationStep) error {
	if step.Config == nil {
		return nil
	}
	if strategy, ok := step.Config["strategy"].(string); ok {
		switch strategy {
		case "", "first", "last":
		default:
			return fmt.Errorf("cda.dedupe: invalid strategy %q (must be first or last)", strategy)
		}
	}
	if overrides, ok := step.Config["overrides"].(map[string]interface{}); ok {
		for sectionKey, raw := range overrides {
			overrideMap, ok := raw.(map[string]interface{})
			if !ok {
				return fmt.Errorf("cda.dedupe: overrides.%s must be an object with a keyPaths array", sectionKey)
			}
			keyPaths, ok := overrideMap["keyPaths"].([]interface{})
			if !ok || len(keyPaths) == 0 {
				return fmt.Errorf("cda.dedupe: overrides.%s.keyPaths must be a non-empty array of CDA paths", sectionKey)
			}
			for i, kp := range keyPaths {
				if _, ok := kp.(string); !ok {
					return fmt.Errorf("cda.dedupe: overrides.%s.keyPaths[%d] must be a string", sectionKey, i)
				}
			}
		}
	}
	if crossMessage, ok := step.Config["crossMessage"].(bool); ok && crossMessage {
		root, _ := step.Config["patientIdentifierRoot"].(string)
		if root == "" {
			return fmt.Errorf("cda.dedupe: patientIdentifierRoot is required when crossMessage is true — there is no automatic patient-identifier detection")
		}
	}
	if raw, present := step.Config["trackSuppressionLineage"]; present {
		if _, ok := raw.(bool); !ok {
			return fmt.Errorf("cda.dedupe: trackSuppressionLineage must be a boolean")
		}
	}
	return nil
}

func (e *CDADedupeExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "Section dedup stats", Path: "_stepOutput.section_stats", DataType: "object",
			Description: "Per-section original_count/dedup_count/removed_count/in_document_removed/cross_message_removed", Category: "CDA Transform"},
		{Name: "Cross-message suppressed entries", Path: "_stepOutput.section_stats.<sectionKey>.cross_message_suppressed", DataType: "array",
			Description: "Present only when crossMessage suppressed at least one entry for that section — one {identity_key, first_seen_message_id, first_seen_at} per dropped entry, so this message's own trace shows exactly what was removed and which earlier message first delivered it, without a separate lookup into cda_dedupe_registry.", Category: "CDA Transform"},
		{Name: "Total removed", Path: "_stepOutput.total_removed", DataType: "integer",
			Description: "Total duplicate entries removed across all processed sections (in-document + cross-message combined)", Category: "CDA Transform"},
		{Name: "Cross-message active", Path: "_stepOutput.cross_message_active", DataType: "boolean",
			Description: "True if crossMessage mode actually ran for this message (false if crossMessage was on but the patient identifier or interface_id couldn't be resolved)", Category: "CDA Transform"},
	}
}

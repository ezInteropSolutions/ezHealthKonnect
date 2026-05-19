// services/ai/operational_ingestion.go
// Embeds YOUR system's actual data (interfaces, pipelines, error patterns,
// confirmed mappings) into the vector store so the AI knows your specific setup —
// not just generic healthcare standards.
package ai

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
)

// OperationalIngestionService ingests live system data into the AI knowledge base.
type OperationalIngestionService struct {
	db        *sql.DB
	embedding *EmbeddingService
}

// NewOperationalIngestionService creates a new OperationalIngestionService.
func NewOperationalIngestionService(db *sql.DB, embedding *EmbeddingService) *OperationalIngestionService {
	return &OperationalIngestionService{db: db, embedding: embedding}
}

// ─── Interface ingestion ──────────────────────────────────────────────────────

// IngestInterface embeds a single interface's full configuration.
// Call this whenever an interface is created or updated.
func (o *OperationalIngestionService) IngestInterface(ctx context.Context, interfaceID string) error {
	if o.db == nil {
		return nil
	}
	var name, description, sourceType, sourceConn, targetType, targetConn string
	var msgType, status, interfaceStatus, deploymentMode string
	var totalProc, successProc, failedProc int
	var isActive bool
	var createdAt time.Time

	err := o.db.QueryRowContext(ctx, `
		SELECT
			name,
			COALESCE(description, ''),
			source_type,
			COALESCE(source_connectivity, ''),
			target_type,
			COALESCE(target_connectivity, ''),
			COALESCE(message_type, ''),
			COALESCE(status, ''),
			COALESCE(interface_status, ''),
			COALESCE(deployment_mode, ''),
			COALESCE(total_processed, 0),
			COALESCE(successful_processed, 0),
			COALESCE(failed_processed, 0),
			COALESCE(is_active, true),
			created_at
		FROM interfaces WHERE id = $1
	`, interfaceID).Scan(
		&name, &description, &sourceType, &sourceConn,
		&targetType, &targetConn, &msgType, &status,
		&interfaceStatus, &deploymentMode,
		&totalProc, &successProc, &failedProc,
		&isActive, &createdAt,
	)
	if err != nil {
		return fmt.Errorf("query interface %s: %w", interfaceID, err)
	}

	successRate := 0.0
	if totalProc > 0 {
		successRate = float64(successProc) / float64(totalProc) * 100
	}

	text := fmt.Sprintf(`Interface Configuration: "%s"
ID: %s
Description: %s
Source: %s (connectivity: %s)
Target: %s (connectivity: %s)
Message Type: %s
Status: %s / %s
Deployment Mode: %s
Statistics: %d messages processed (%.1f%% success, %d failures)
Active: %v
Created: %s`,
		name, interfaceID, description,
		sourceType, sourceConn,
		targetType, targetConn,
		msgType, status, interfaceStatus,
		deploymentMode,
		totalProc, successRate, failedProc,
		isActive,
		createdAt.Format("2006-01-02"),
	)

	ref := "interface:" + interfaceID
	o.deleteChunks(ctx, ref)

	_, err = o.embedding.IngestText(ctx, "operational", ref, "db:interfaces", text,
		map[string]interface{}{
			"entity_type":  "interface",
			"entity_id":    interfaceID,
			"name":         name,
			"message_type": msgType,
		})
	return err
}

// IngestAllInterfaces rebuilds embedded knowledge for all active interfaces.
func (o *OperationalIngestionService) IngestAllInterfaces(ctx context.Context) *IngestionResult {
	result := &IngestionResult{SourceType: "operational:interfaces"}
	if o.db == nil {
		return result
	}
	rows, err := o.db.QueryContext(ctx, `SELECT id FROM interfaces WHERE is_active = true ORDER BY created_at`)
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
		return result
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}

	for _, id := range ids {
		result.FilesScanned++
		if err := o.IngestInterface(ctx, id); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("interface %s: %v", id, err))
		} else {
			result.ChunksStored++
		}
	}

	log.Printf("✅ AI KB — Operational Interfaces: %d interfaces, %d errors", result.FilesScanned, len(result.Errors))
	return result
}

// ─── Pipeline ingestion ───────────────────────────────────────────────────────

// IngestPipeline embeds a pipeline and all its steps.
// Call this whenever a pipeline is saved.
func (o *OperationalIngestionService) IngestPipeline(ctx context.Context, pipelineID string) error {
	if o.db == nil {
		return nil
	}
	var interfaceID, messageType, pipelineName string
	var version int

	err := o.db.QueryRowContext(ctx, `
		SELECT interface_id, message_type, pipeline_name, version
		FROM transformation_pipelines WHERE id = $1
	`, pipelineID).Scan(&interfaceID, &messageType, &pipelineName, &version)
	if err != nil {
		return fmt.Errorf("query pipeline %s: %w", pipelineID, err)
	}

	// Get interface name for richer context
	var interfaceName string
	_ = o.db.QueryRowContext(ctx, `SELECT name FROM interfaces WHERE id = $1`, interfaceID).
		Scan(&interfaceName)

	// Fetch steps
	rows, err := o.db.QueryContext(ctx, `
		SELECT step_name, step_type, sequence, enabled, COALESCE(config::text, '{}')
		FROM transformation_steps
		WHERE pipeline_id = $1
		ORDER BY sequence
	`, pipelineID)
	if err != nil {
		return fmt.Errorf("query steps for pipeline %s: %w", pipelineID, err)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Transformation Pipeline: \"%s\"\n", pipelineName))
	sb.WriteString(fmt.Sprintf("Interface: \"%s\" (ID: %s)\n", interfaceName, interfaceID))
	sb.WriteString(fmt.Sprintf("Message Type: %s | Version: %d\n\n", messageType, version))
	sb.WriteString("Steps (in execution order):\n")

	stepCount := 0
	for rows.Next() {
		var stepName, stepType, configJSON string
		var seq int
		var enabled bool
		if err := rows.Scan(&stepName, &stepType, &seq, &enabled, &configJSON); err != nil {
			continue
		}
		state := ""
		if !enabled {
			state = " [DISABLED]"
		}
		// Truncate large configs
		if len(configJSON) > 200 {
			configJSON = configJSON[:200] + "..."
		}
		sb.WriteString(fmt.Sprintf("  Seq %3d: [%-32s] %s%s\n    Config: %s\n",
			seq, stepType, stepName, state, configJSON))
		stepCount++
	}
	sb.WriteString(fmt.Sprintf("\nTotal Steps: %d", stepCount))

	ref := "pipeline:" + pipelineID
	o.deleteChunks(ctx, ref)

	_, err = o.embedding.IngestText(ctx, "operational", ref, "db:transformation_pipelines", sb.String(),
		map[string]interface{}{
			"entity_type":  "pipeline",
			"entity_id":    pipelineID,
			"interface_id": interfaceID,
			"message_type": messageType,
		})
	return err
}

// IngestAllPipelines rebuilds embedded knowledge for all enabled pipelines.
func (o *OperationalIngestionService) IngestAllPipelines(ctx context.Context) *IngestionResult {
	result := &IngestionResult{SourceType: "operational:pipelines"}
	if o.db == nil {
		return result
	}
	rows, err := o.db.QueryContext(ctx, `SELECT id FROM transformation_pipelines WHERE enabled = true ORDER BY created_at`)
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
		return result
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}

	for _, id := range ids {
		result.FilesScanned++
		if err := o.IngestPipeline(ctx, id); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("pipeline %s: %v", id, err))
		} else {
			result.ChunksStored++
		}
	}

	log.Printf("✅ AI KB — Operational Pipelines: %d pipelines, %d errors", result.FilesScanned, len(result.Errors))
	return result
}

// ─── Error pattern ingestion ──────────────────────────────────────────────────

// IngestErrorPatterns embeds recurring error patterns from audit_logs.
// Groups by interface + error message, surfaces patterns seen 3+ times in 30 days.
func (o *OperationalIngestionService) IngestErrorPatterns(ctx context.Context) *IngestionResult {
	result := &IngestionResult{SourceType: "operational:errors"}
	if o.db == nil {
		return result
	}
	rows, err := o.db.QueryContext(ctx, `
		SELECT
			COALESCE(entity_id, 'unknown')      AS interface_id,
			COALESCE(error_message, 'unknown')  AS error_message,
			COUNT(*)                             AS occurrence_count,
			MAX(created_at)                      AS last_seen
		FROM audit_logs
		WHERE result    = 'FAILURE'
		  AND error_message IS NOT NULL
		  AND created_at > NOW() - INTERVAL '30 days'
		GROUP BY entity_id, error_message
		HAVING COUNT(*) >= 3
		ORDER BY COUNT(*) DESC
		LIMIT 200
	`)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("query error patterns: %v", err))
		return result
	}
	defer rows.Close()

	// Clear stale error pattern chunks before re-ingesting
	_, _ = o.db.ExecContext(ctx,
		`DELETE FROM ai_knowledge_chunks WHERE source_type = 'operational' AND source_ref LIKE 'error_pattern:%'`)

	for rows.Next() {
		var interfaceID, errorMsg string
		var count int
		var lastSeen time.Time
		if err := rows.Scan(&interfaceID, &errorMsg, &count, &lastSeen); err != nil {
			continue
		}

		// Get interface name for context
		var interfaceName string
		_ = o.db.QueryRowContext(ctx, `SELECT COALESCE(name,'') FROM interfaces WHERE id = $1`, interfaceID).
			Scan(&interfaceName)

		text := fmt.Sprintf(`Recurring Error Pattern
Interface: "%s" (ID: %s)
Error Message: %s
Occurrences: %d times in the last 30 days
Last Seen: %s`,
			interfaceName, interfaceID,
			errorMsg,
			count,
			lastSeen.Format("2006-01-02 15:04 UTC"),
		)

		result.FilesScanned++
		ref := fmt.Sprintf("error_pattern:%s:%d", interfaceID, lastSeen.Unix())
		n, err := o.embedding.IngestText(ctx, "operational", ref, "db:audit_logs", text,
			map[string]interface{}{
				"entity_type":  "error_pattern",
				"interface_id": interfaceID,
				"count":        count,
			})
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("embed error for %s: %v", interfaceID, err))
		} else {
			result.ChunksStored += n
		}
	}

	log.Printf("✅ AI KB — Error Patterns: %d patterns, %d chunks, %d errors",
		result.FilesScanned, result.ChunksStored, len(result.Errors))
	return result
}

// ─── Feedback loop ingestion ──────────────────────────────────────────────────

// IngestConfirmedMapping embeds a user-validated field mapping as high-confidence knowledge.
// The more confirmed mappings are embedded, the better future suggestions become.
func (o *OperationalIngestionService) IngestConfirmedMapping(ctx context.Context, interfaceID, sourceField, targetField, reasoning string) error {
	if o.db == nil {
		return nil
	}
	var interfaceName, msgType string
	_ = o.db.QueryRowContext(ctx,
		`SELECT name, COALESCE(message_type,'') FROM interfaces WHERE id = $1`,
		interfaceID).Scan(&interfaceName, &msgType)

	text := fmt.Sprintf(`Confirmed Field Mapping (User Validated — High Confidence)
Interface: "%s" (ID: %s)
Message Type: %s
Source Field: %s
Target Field: %s
Reasoning: %s
Validation Status: CONFIRMED BY USER`,
		interfaceName, interfaceID, msgType,
		sourceField, targetField, reasoning,
	)

	ref := fmt.Sprintf("confirmed_mapping:%s:%s", interfaceID, sourceField)
	o.deleteChunks(ctx, ref)

	_, err := o.embedding.IngestText(ctx, "operational", ref, "db:user_feedback", text,
		map[string]interface{}{
			"entity_type":  "confirmed_mapping",
			"interface_id": interfaceID,
			"source_field": sourceField,
			"target_field": targetField,
		})
	return err
}

// IngestRejectedMapping embeds a rejected suggestion so the model avoids it.
func (o *OperationalIngestionService) IngestRejectedMapping(ctx context.Context, interfaceID, sourceField, targetField, reason string) error {
	if o.db == nil {
		return nil
	}
	var interfaceName, msgType string
	_ = o.db.QueryRowContext(ctx,
		`SELECT name, COALESCE(message_type,'') FROM interfaces WHERE id = $1`,
		interfaceID).Scan(&interfaceName, &msgType)

	text := fmt.Sprintf(`Rejected Field Mapping (User Rejected — Avoid This)
Interface: "%s" (ID: %s)
Message Type: %s
Source Field: %s
Target Field: %s (INCORRECT — do not suggest this mapping)
Rejection Reason: %s`,
		interfaceName, interfaceID, msgType,
		sourceField, targetField, reason,
	)

	ref := fmt.Sprintf("rejected_mapping:%s:%s:%s", interfaceID, sourceField, targetField)
	o.deleteChunks(ctx, ref)

	_, err := o.embedding.IngestText(ctx, "operational", ref, "db:user_feedback", text,
		map[string]interface{}{
			"entity_type":  "rejected_mapping",
			"interface_id": interfaceID,
		})
	return err
}

// IngestResolvedError embeds a known error+solution pair for future reference.
func (o *OperationalIngestionService) IngestResolvedError(ctx context.Context, interfaceID, errorMsg, solution string) error {
	if o.db == nil {
		return nil
	}
	var interfaceName string
	_ = o.db.QueryRowContext(ctx, `SELECT name FROM interfaces WHERE id = $1`, interfaceID).
		Scan(&interfaceName)

	text := fmt.Sprintf(`Known Error Resolution (Verified Fix)
Interface: "%s" (ID: %s)
Error: %s
Solution Applied: %s
Status: RESOLVED`,
		interfaceName, interfaceID, errorMsg, solution,
	)

	ref := fmt.Sprintf("resolved_error:%s:%d", interfaceID, time.Now().Unix())
	_, err := o.embedding.IngestText(ctx, "operational", ref, "db:user_feedback", text,
		map[string]interface{}{
			"entity_type":  "resolved_error",
			"interface_id": interfaceID,
		})
	return err
}

// ─── IG Knowledge Ingestion ───────────────────────────────────────────────────

// IngestIGMappings embeds HL7v2→FHIR IG data into the AI knowledge base:
//   - ig_field_anchors       (field-level HL7→FHIR mappings, source_type "ig_anchors")
//   - assembly_rules         (cross-resource wiring rules, source_type "ig_assemblyrules")
//   - hl7_fhir_value_mappings (code translation tables, source_type "ig_valuesets")
func (o *OperationalIngestionService) IngestIGMappings(ctx context.Context) []IngestionResult {
	var results []IngestionResult
	if r := o.ingestIGAnchors(ctx); r != nil {
		results = append(results, *r)
	}
	if r := o.ingestAssemblyRules(ctx); r != nil {
		results = append(results, *r)
	}
	if r := o.ingestValueMappings(ctx); r != nil {
		results = append(results, *r)
	}
	return results
}

// ingestIGAnchors embeds ig_field_anchors grouped by segment into the AI KB.
func (o *OperationalIngestionService) ingestIGAnchors(ctx context.Context) *IngestionResult {
	result := &IngestionResult{SourceType: "ig_anchors"}
	if o.db == nil {
		return result
	}

	rows, err := o.db.QueryContext(ctx, `
		SELECT segment, field_pos, fhir_resource, fhir_path, fhir_type, confidence, ig_source
		FROM   ig_field_anchors
		WHERE  is_active = true
		ORDER  BY segment, field_pos
	`)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("query ig_field_anchors: %v", err))
		return result
	}
	defer rows.Close()

	type anchorRow struct {
		segment, fieldPos, fhirResource, fhirPath, fhirType, igSource string
		confidence                                                      float64
	}
	bySegment := make(map[string][]anchorRow)
	var segOrder []string

	for rows.Next() {
		var a anchorRow
		var fhirType sql.NullString
		if err := rows.Scan(&a.segment, &a.fieldPos, &a.fhirResource, &a.fhirPath, &fhirType, &a.confidence, &a.igSource); err != nil {
			continue
		}
		a.fhirType = fhirType.String
		if _, seen := bySegment[a.segment]; !seen {
			segOrder = append(segOrder, a.segment)
		}
		bySegment[a.segment] = append(bySegment[a.segment], a)
	}
	if err := rows.Err(); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("rows error: %v", err))
	}

	for _, seg := range segOrder {
		anchors := bySegment[seg]
		result.FilesScanned++

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("HL7v2 to FHIR IG Field Anchors: %s Segment\n", seg))
		sb.WriteString(fmt.Sprintf("Source: %s\n\n", anchors[0].igSource))
		for _, a := range anchors {
			typeHint := ""
			if a.fhirType != "" {
				typeHint = " (" + a.fhirType + ")"
			}
			sb.WriteString(fmt.Sprintf("  %s.%s → %s.%s%s [confidence %.2f]\n",
				seg, a.fieldPos, a.fhirResource, a.fhirPath, typeHint, a.confidence))
		}

		ref := "ig_anchor:" + seg
		_, _ = o.db.ExecContext(ctx,
			`DELETE FROM ai_knowledge_chunks WHERE source_type = 'ig_anchors' AND source_ref = $1`, ref)

		if _, err := o.embedding.IngestText(ctx, "ig_anchors", ref, "db:ig_field_anchors", sb.String(),
			map[string]interface{}{
				"entity_type": "ig_anchor",
				"segment":     seg,
			}); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("embed segment %s: %v", seg, err))
		} else {
			result.ChunksStored++
		}
	}

	log.Printf("✅ AI KB — IG Anchors: %d segments, %d errors", result.FilesScanned, len(result.Errors))
	return result
}

// ingestAssemblyRules embeds assembly_rules grouped by message_type into the AI KB.
func (o *OperationalIngestionService) ingestAssemblyRules(ctx context.Context) *IngestionResult {
	result := &IngestionResult{SourceType: "ig_assemblyrules"}
	if o.db == nil {
		return result
	}

	rows, err := o.db.QueryContext(ctx, `
		SELECT message_type, rule_type, source_resource, target_resource, reference_path
		FROM   assembly_rules
		WHERE  is_active = true
		ORDER  BY message_type, sequence
	`)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("query assembly_rules: %v", err))
		return result
	}
	defer rows.Close()

	type ruleRow struct {
		messageType, ruleType, sourceResource, targetResource, referencePath string
	}
	byMsgType := make(map[string][]ruleRow)
	var msgOrder []string

	for rows.Next() {
		var r ruleRow
		if err := rows.Scan(&r.messageType, &r.ruleType, &r.sourceResource, &r.targetResource, &r.referencePath); err != nil {
			continue
		}
		if _, seen := byMsgType[r.messageType]; !seen {
			msgOrder = append(msgOrder, r.messageType)
		}
		byMsgType[r.messageType] = append(byMsgType[r.messageType], r)
	}
	if err := rows.Err(); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("rows error: %v", err))
	}

	for _, msgType := range msgOrder {
		rules := byMsgType[msgType]
		result.FilesScanned++

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("FHIR Bundle Assembly Rules: %s\n\n", msgType))
		for _, r := range rules {
			sb.WriteString(fmt.Sprintf("  [%s] %s.%s → %s\n",
				r.ruleType, r.sourceResource, r.referencePath, r.targetResource))
		}

		ref := "assembly_rules:" + msgType
		_, _ = o.db.ExecContext(ctx,
			`DELETE FROM ai_knowledge_chunks WHERE source_type = 'ig_assemblyrules' AND source_ref = $1`, ref)

		if _, err := o.embedding.IngestText(ctx, "ig_assemblyrules", ref, "db:assembly_rules", sb.String(),
			map[string]interface{}{
				"entity_type":  "assembly_rules",
				"message_type": msgType,
			}); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("embed %s: %v", msgType, err))
		} else {
			result.ChunksStored++
		}
	}

	log.Printf("✅ AI KB — Assembly Rules: %d message types, %d errors", result.FilesScanned, len(result.Errors))
	return result
}

// ingestValueMappings embeds hl7_fhir_value_mappings grouped by hl7_table into the AI KB.
func (o *OperationalIngestionService) ingestValueMappings(ctx context.Context) *IngestionResult {
	result := &IngestionResult{SourceType: "ig_valuesets"}
	if o.db == nil {
		return result
	}

	rows, err := o.db.QueryContext(ctx, `
		SELECT hl7_table, hl7_value, fhir_code, fhir_display, fhir_system, COALESCE(ig_source, 'v2-to-fhir-r4')
		FROM   hl7_fhir_value_mappings
		WHERE  is_active = true
		ORDER  BY hl7_table, hl7_value
	`)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("query hl7_fhir_value_mappings: %v", err))
		return result
	}
	defer rows.Close()

	type valueRow struct {
		table, hl7Value, fhirCode, fhirDisplay, fhirSystem, igSource string
	}
	byTable := make(map[string][]valueRow)
	var tableOrder []string

	for rows.Next() {
		var v valueRow
		var fhirDisplay sql.NullString
		if err := rows.Scan(&v.table, &v.hl7Value, &v.fhirCode, &fhirDisplay, &v.fhirSystem, &v.igSource); err != nil {
			continue
		}
		v.fhirDisplay = fhirDisplay.String
		if _, seen := byTable[v.table]; !seen {
			tableOrder = append(tableOrder, v.table)
		}
		byTable[v.table] = append(byTable[v.table], v)
	}
	if err := rows.Err(); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("rows error: %v", err))
	}

	for _, table := range tableOrder {
		values := byTable[table]
		result.FilesScanned++

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("HL7 Table %s → FHIR Value Mappings\n", table))
		sb.WriteString(fmt.Sprintf("Source: %s\n", values[0].igSource))
		sb.WriteString(fmt.Sprintf("FHIR System: %s\n\n", values[0].fhirSystem))
		for _, v := range values {
			sb.WriteString(fmt.Sprintf("  HL7 \"%s\" → FHIR \"%s\" (%s)\n",
				v.hl7Value, v.fhirCode, v.fhirDisplay))
		}

		ref := "hl7_valueset:" + table
		_, _ = o.db.ExecContext(ctx,
			`DELETE FROM ai_knowledge_chunks WHERE source_type = 'ig_valuesets' AND source_ref = $1`, ref)

		if _, err := o.embedding.IngestText(ctx, "ig_valuesets", ref, "db:hl7_fhir_value_mappings", sb.String(),
			map[string]interface{}{
				"entity_type": "value_mapping",
				"hl7_table":   table,
				"fhir_system": values[0].fhirSystem,
			}); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("embed table %s: %v", table, err))
		} else {
			result.ChunksStored++
		}
	}

	log.Printf("✅ AI KB — HL7 Value Mappings: %d tables, %d errors", result.FilesScanned, len(result.Errors))
	return result
}

// ─── Bulk rebuild ─────────────────────────────────────────────────────────────

// IngestAll rebuilds all operational knowledge (interfaces + pipelines + errors + IG).
func (o *OperationalIngestionService) IngestAll(ctx context.Context) []IngestionResult {
	var results []IngestionResult

	if r := o.IngestAllInterfaces(ctx); r != nil {
		results = append(results, *r)
	}
	if r := o.IngestAllPipelines(ctx); r != nil {
		results = append(results, *r)
	}
	if r := o.IngestErrorPatterns(ctx); r != nil {
		results = append(results, *r)
	}
	for _, r := range o.IngestIGMappings(ctx) {
		results = append(results, r)
	}
	return results
}

// ─── Helper ───────────────────────────────────────────────────────────────────

func (o *OperationalIngestionService) deleteChunks(ctx context.Context, ref string) {
	_, _ = o.db.ExecContext(ctx,
		`DELETE FROM ai_knowledge_chunks WHERE source_type = 'operational' AND source_ref = $1`, ref)
}

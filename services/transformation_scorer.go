// services/transformation_scorer.go
//
// TransformationScorer evaluates the quality of each FHIR transformation
// and persists a score to transformation_quality_scores.
//
// Scoring is entirely async — callers fire ScoreAndPersist in a goroutine
// so it has zero impact on delivery latency.
package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"math"
	"strings"
	"time"

	"ezhealthkonnect/services/manifest"

	"github.com/lib/pq"
)

// requiredFHIRFields defines the minimum fields expected per resource type.
// Absence of any of these contributes to a lower quality score and flags
// the transformation for integration team review.
var requiredFHIRFields = map[string][]string{
	"Patient":           {"identifier", "name"},
	"Encounter":         {"status", "class"},
	"Observation":       {"status", "code"},
	"DiagnosticReport":  {"status", "code"},
	"Immunization":      {"status", "vaccineCode"},
	"DocumentReference": {"status", "content"},
	"ServiceRequest":    {"status", "code"},
	"Appointment":       {"status"},
	"ChargeItem":        {"status", "code"},
}

// QualityScore is the result of scoring one transformation.
type QualityScore struct {
	MessageID            string
	InterfaceID          string
	MessageType          string
	Score                float64
	ResourceCounts       map[string]int
	ExpectedResources    []string
	MissingResources     []string
	EmptyRequiredFields  map[string]bool
	UnhandledSegments    []string
	Warnings             []string
	Errors               []string
	Flagged              bool
	FlagReason           string
}

// TransformationScorer computes quality scores and writes them to PostgreSQL.
type TransformationScorer struct {
	db *sql.DB
}

// NewTransformationScorer creates a scorer backed by the given DB connection.
func NewTransformationScorer(db *sql.DB) *TransformationScorer {
	return &TransformationScorer{db: db}
}

// ScoreAndPersist scores a TransformResponse and writes the result to
// transformation_quality_scores. Designed to run in a goroutine.
func (s *TransformationScorer) ScoreAndPersist(
	messageID, interfaceID, messageType string,
	resp *TransformResponse,
) {
	if resp == nil {
		return
	}
	qs := Score(messageID, interfaceID, messageType, resp)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.persist(ctx, qs); err != nil {
		log.Printf("[scorer] failed to persist quality score for %s: %v", messageID, err)
	}
}

// Score computes a QualityScore from a TransformResponse without touching the DB.
// Exported so it can be unit-tested directly.
func Score(messageID, interfaceID, messageType string, resp *TransformResponse) *QualityScore {
	qs := &QualityScore{
		MessageID:           messageID,
		InterfaceID:         interfaceID,
		MessageType:         messageType,
		ResourceCounts:      resp.ResourceCounts,
		Warnings:            resp.Warnings,
		Errors:              resp.Errors,
		EmptyRequiredFields: map[string]bool{},
	}

	score := 100.0

	// ── 1. Missing expected resources (-20 each, capped at -60) ──────────────
	qs.ExpectedResources = expectedResources(messageType)
	produced := resp.ResourceCounts
	if produced == nil {
		produced = map[string]int{}
	}
	for _, expected := range qs.ExpectedResources {
		if produced[expected] == 0 {
			qs.MissingResources = append(qs.MissingResources, expected)
		}
	}
	missingPenalty := math.Min(float64(len(qs.MissingResources))*20.0, 60.0)
	score -= missingPenalty

	// ── 2. Empty required FHIR fields (-15 each, capped at -30) ─────────────
	emptyCount := 0
	for _, res := range resp.FHIRResources {
		rt, _ := res["resourceType"].(string)
		fields, ok := requiredFHIRFields[rt]
		if !ok {
			continue
		}
		for _, f := range fields {
			v, exists := res[f]
			if !exists || isEmptyValue(v) {
				key := rt + "." + f
				if !qs.EmptyRequiredFields[key] {
					qs.EmptyRequiredFields[key] = true
					emptyCount++
				}
			}
		}
	}
	emptyPenalty := math.Min(float64(emptyCount)*15.0, 30.0)
	score -= emptyPenalty

	// ── 3. Warnings (-5 each, capped at -10) ─────────────────────────────────
	warnPenalty := math.Min(float64(len(resp.Warnings))*5.0, 10.0)
	score -= warnPenalty

	// ── 4. Errors (-10 each, uncapped) ───────────────────────────────────────
	score -= float64(len(resp.Errors)) * 10.0

	qs.Score = math.Max(score, 0.0)

	// ── Flag decision ─────────────────────────────────────────────────────────
	var reasons []string
	if len(qs.MissingResources) > 0 {
		reasons = append(reasons, "missing resources: "+strings.Join(qs.MissingResources, ", "))
	}
	if len(resp.Errors) > 0 {
		reasons = append(reasons, "transformation errors")
	}
	if qs.Score < 80.0 && len(reasons) == 0 {
		reasons = append(reasons, "score below threshold")
	}
	if len(reasons) > 0 {
		qs.Flagged = true
		qs.FlagReason = strings.Join(reasons, "; ")
	}

	return qs
}

// expectedResources returns the resource types the IG expects for a message type.
// Falls back to ["Patient"] when the message type is unknown.
func expectedResources(messageType string) []string {
	// MessageType is e.g. "ADT^A01" — extract the event code after "^"
	eventCode := messageType
	if parts := strings.SplitN(messageType, "^", 2); len(parts) == 2 {
		eventCode = parts[1]
	}
	// Further strip trigger event suffix (e.g. "A01" from "A01^ADT_A01")
	if parts := strings.SplitN(eventCode, "^", 2); len(parts) == 2 {
		eventCode = parts[0]
	}

	m := manifest.Lookup(eventCode)
	if m == nil {
		return []string{"Patient"}
	}
	out := make([]string, len(m.AllowedResources))
	copy(out, m.AllowedResources)
	return out
}

// isEmptyValue returns true for nil, empty string, empty slice, and empty map.
func isEmptyValue(v interface{}) bool {
	if v == nil {
		return true
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) == ""
	case []interface{}:
		return len(t) == 0
	case map[string]interface{}:
		return len(t) == 0
	}
	return false
}

// persist writes a QualityScore to the database.
func (s *TransformationScorer) persist(ctx context.Context, qs *QualityScore) error {
	countsJSON, _ := json.Marshal(qs.ResourceCounts)
	emptyJSON, _ := json.Marshal(qs.EmptyRequiredFields)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO transformation_quality_scores (
			message_id, interface_id, message_type,
			score, resource_counts, expected_resources, missing_resources,
			empty_required_fields, unhandled_segments,
			warnings, errors, flagged, flag_reason
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		qs.MessageID,
		qs.InterfaceID,
		qs.MessageType,
		qs.Score,
		countsJSON,
		pq.Array(qs.ExpectedResources),
		pq.Array(qs.MissingResources),
		emptyJSON,
		pq.Array(qs.UnhandledSegments),
		pq.Array(qs.Warnings),
		pq.Array(qs.Errors),
		qs.Flagged,
		qs.FlagReason,
	)
	return err
}

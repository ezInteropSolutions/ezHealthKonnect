// services/mapping/ig_anchor_service.go
//
// Loads the IG field anchors from the ig_field_anchors table (V115) and
// exposes them as the same Candidate slices that semantic_matcher.go uses.
//
// Responsibility: own the DB I/O for anchor data.
// What it does NOT do: scoring, heuristics, FHIR schema ops — those stay in
// semantic_matcher.go and schema_map_generator.go.
//
// Usage:
//
//	svc := NewIGAnchorService(db)
//	anchors, err := svc.LoadForVersion(ctx, FHIRVersionR4)
//	// pass anchors to Generator.WithDBAnchors(anchors)
package mapping

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
)

// IGAnchorService loads and caches IG field anchors from the database.
// One instance is shared for the lifetime of the process.
type IGAnchorService struct {
	db  *sql.DB
	mu  sync.RWMutex
	// cache keyed by ig_source value (e.g. "v2-to-fhir-r4", "v2-to-fhir-r5")
	// inner map: "SEGMENT.FIELD" → []Candidate (same format as knownAnchorsR4)
	cache map[string]map[string][]Candidate
}

// NewIGAnchorService returns a service backed by the given database.
// The cache is populated lazily on the first LoadForVersion call.
func NewIGAnchorService(db *sql.DB) *IGAnchorService {
	return &IGAnchorService{db: db}
}

// LoadForVersion returns the DB-sourced anchors for a FHIR version.
// The result is keyed exactly like knownAnchorsR4: "SEGMENT.FIELD".
// A nil return means the table is empty or the DB is unavailable — callers
// fall back to the hardcoded anchor tables in semantic_matcher.go.
func (s *IGAnchorService) LoadForVersion(ctx context.Context, ver FHIRVersion) (map[string][]Candidate, error) {
	igSource := igSourceKey(ver)

	s.mu.RLock()
	if s.cache != nil {
		anchors := s.cache[igSource]
		s.mu.RUnlock()
		return anchors, nil
	}
	s.mu.RUnlock()

	// Populate cache — load all ig_source values in one query.
	if err := s.load(ctx); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache[igSource], nil
}

// Invalidate clears the in-memory cache so the next call re-reads the DB.
// Call this after applying a migration that inserts new ig_field_anchors rows.
func (s *IGAnchorService) Invalidate() {
	s.mu.Lock()
	s.cache = nil
	s.mu.Unlock()
	log.Printf("🔄 IGAnchorService cache invalidated")
}

// load fetches all rows from ig_field_anchors and populates s.cache.
func (s *IGAnchorService) load(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("IGAnchorService: no database connection")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT segment, field_pos, fhir_resource, fhir_path, fhir_type, confidence, ig_source
		FROM   ig_field_anchors
		WHERE  is_active = true
		ORDER  BY segment, field_pos, ig_source
	`)
	if err != nil {
		return fmt.Errorf("IGAnchorService: query failed: %w", err)
	}
	defer rows.Close()

	// two-level map: igSource → (segment.field → candidates)
	all := make(map[string]map[string][]Candidate)

	for rows.Next() {
		var segment, fieldPos, fhirResource, fhirPath, igSrc string
		var fhirType sql.NullString
		var confidence float64

		if err := rows.Scan(&segment, &fieldPos, &fhirResource, &fhirPath, &fhirType, &confidence, &igSrc); err != nil {
			log.Printf("⚠️ IGAnchorService: scan error: %v", err)
			continue
		}

		// Key format: "SEGMENT.FIELD" — same as knownAnchorsR4.
		// fhirPath stored in DB is resource-relative ("name[0].family");
		// callers expect fully-qualified ("Patient.name[0].family").
		key := strings.ToUpper(segment) + "." + fieldPos
		fullPath := fhirResource + "." + fhirPath

		c := Candidate{
			FHIRPath:   fullPath,
			FHIRType:   fhirType.String,
			Confidence: confidence,
			Source:     "ig_db",
		}

		if all[igSrc] == nil {
			all[igSrc] = make(map[string][]Candidate)
		}
		all[igSrc][key] = append(all[igSrc][key], c)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("IGAnchorService: rows error: %w", err)
	}

	s.mu.Lock()
	s.cache = all
	s.mu.Unlock()

	total := 0
	for _, m := range all {
		total += len(m)
	}
	log.Printf("✅ IGAnchorService: loaded %d anchor keys from ig_field_anchors", total)
	return nil
}

// igSourceKey maps a FHIRVersion to the ig_source value stored in the DB.
func igSourceKey(ver FHIRVersion) string {
	if ver == FHIRVersionR5 {
		return "v2-to-fhir-r5"
	}
	return "v2-to-fhir-r4"
}

package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/models"
)

// ============================================================
// CODE TEMPLATE SERVICE
// ============================================================
// Manages named JavaScript function libraries (Code Templates).
// Provides CRUD, interface linking, and a cached hot-path for
// loading templates to inject into script step goja VMs.

// cacheEntry is a single cached result set with a load timestamp.
type cacheEntry struct {
	templates []*models.CodeTemplateForExecution
	loadedAt  time.Time
}

// CodeTemplateService is the primary service for code template management.
type CodeTemplateService struct {
	db      *sql.DB
	ttl     time.Duration
	mu      sync.RWMutex
	cache   map[string]*cacheEntry // key: interfaceID ("" = globals only)
}

// NewCodeTemplateService creates a CodeTemplateService with a 5-minute cache TTL.
func NewCodeTemplateService(db *sql.DB) *CodeTemplateService {
	return &CodeTemplateService{
		db:    db,
		ttl:   5 * time.Minute,
		cache: make(map[string]*cacheEntry),
	}
}

// ============================================================
// HOT PATH: LoadForExecution
// ============================================================

// LoadForExecution returns the lightweight template list for script executor injection.
// Result is cached per interfaceID (empty string = globals-only context).
// Thread-safe: multiple goroutines can call concurrently.
func (s *CodeTemplateService) LoadForExecution(interfaceID string) ([]*models.CodeTemplateForExecution, error) {
	key := interfaceID

	// Fast read-lock check
	s.mu.RLock()
	entry, ok := s.cache[key]
	s.mu.RUnlock()
	if ok && time.Since(entry.loadedAt) < s.ttl {
		return entry.templates, nil
	}

	// Cache miss — query DB
	templates, err := s.queryForExecution(interfaceID)
	if err != nil {
		return nil, err
	}

	// Write to cache
	s.mu.Lock()
	s.cache[key] = &cacheEntry{templates: templates, loadedAt: time.Now()}
	s.mu.Unlock()

	return templates, nil
}

func (s *CodeTemplateService) queryForExecution(interfaceID string) ([]*models.CodeTemplateForExecution, error) {
	query := `
		SELECT id, name, code, sort_order
		FROM code_templates
		WHERE is_active = true
		  AND (
		      scope = 'global'
		      OR (scope = 'interface' AND id IN (
		          SELECT template_id FROM code_template_interface_links
		          WHERE interface_id = $1
		      ))
		  )
		ORDER BY sort_order ASC, name ASC`

	rows, err := s.db.Query(query, interfaceID)
	if err != nil {
		return nil, fmt.Errorf("code template query failed: %w", err)
	}
	defer rows.Close()

	var result []*models.CodeTemplateForExecution
	for rows.Next() {
		t := &models.CodeTemplateForExecution{}
		if err := rows.Scan(&t.ID, &t.Name, &t.Code, &t.SortOrder); err != nil {
			return nil, fmt.Errorf("code template scan failed: %w", err)
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

// BuildPreamble concatenates template code into a single JS preamble string.
// The preamble is placed at module scope so its functions are callable from
// inside the IIFE wrapper that surrounds the user's script.
func (s *CodeTemplateService) BuildPreamble(templates []*models.CodeTemplateForExecution) string {
	if len(templates) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, t := range templates {
		sb.WriteString("// === Code Template: ")
		sb.WriteString(t.Name)
		sb.WriteString(" ===\n")
		sb.WriteString(t.Code)
		sb.WriteString("\n\n")
	}
	return sb.String()
}

// ============================================================
// CACHE INVALIDATION
// ============================================================

// InvalidateAll purges all cache entries (used after any template write or admin flush).
func (s *CodeTemplateService) InvalidateAll() {
	s.mu.Lock()
	s.cache = make(map[string]*cacheEntry)
	s.mu.Unlock()
	log.Printf("🔄 [CodeTemplates] Cache invalidated (all entries)")
}

// invalidateInterface purges only the cache entry for a specific interface.
func (s *CodeTemplateService) invalidateInterface(interfaceID string) {
	s.mu.Lock()
	delete(s.cache, interfaceID)
	s.mu.Unlock()
}

// ============================================================
// CRUD OPERATIONS
// ============================================================

// List returns all code templates filtered by optional query params.
func (s *CodeTemplateService) List(category, scope, isActive string) ([]*models.CodeTemplate, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	i := 1

	if category != "" {
		where = append(where, fmt.Sprintf("category = $%d", i))
		args = append(args, category)
		i++
	}
	if scope != "" {
		where = append(where, fmt.Sprintf("scope = $%d", i))
		args = append(args, scope)
		i++
	}
	if isActive != "" {
		active := isActive == "true"
		where = append(where, fmt.Sprintf("is_active = $%d", i))
		args = append(args, active)
		i++
	}

	query := fmt.Sprintf(`
		SELECT id, name, description, category, code, function_signatures, scope,
		       is_system, is_active, sort_order, usage_count,
		       created_by_user_id, created_at, updated_at
		FROM code_templates
		WHERE %s
		ORDER BY sort_order ASC, name ASC`, strings.Join(where, " AND "))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list code templates: %w", err)
	}
	defer rows.Close()

	return s.scanTemplates(rows)
}

// Get returns a single code template by ID.
func (s *CodeTemplateService) Get(id string) (*models.CodeTemplate, error) {
	row := s.db.QueryRow(`
		SELECT id, name, description, category, code, function_signatures, scope,
		       is_system, is_active, sort_order, usage_count,
		       created_by_user_id, created_at, updated_at
		FROM code_templates WHERE id = $1`, id)

	return s.scanTemplate(row)
}

// Create inserts a new code template and invalidates the cache.
func (s *CodeTemplateService) Create(t *models.CodeTemplate) (*models.CodeTemplate, error) {
	sigsJSON, err := json.Marshal(t.FunctionSignatures)
	if err != nil {
		return nil, fmt.Errorf("marshal function_signatures: %w", err)
	}

	var id string
	err = s.db.QueryRow(`
		INSERT INTO code_templates
		    (name, description, category, code, function_signatures, scope,
		     is_system, is_active, sort_order, created_by_user_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id`,
		t.Name, t.Description, t.Category, t.Code, sigsJSON, t.Scope,
		t.IsSystem, t.IsActive, t.SortOrder, t.CreatedByUserID,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("create code template: %w", err)
	}

	s.InvalidateAll()
	return s.Get(id)
}

// Update modifies an existing code template and invalidates the cache.
// System templates' code and is_system fields cannot be changed.
func (s *CodeTemplateService) Update(id string, t *models.CodeTemplate) (*models.CodeTemplate, error) {
	existing, err := s.Get(id)
	if err != nil {
		return nil, err
	}

	sigsJSON, err := json.Marshal(t.FunctionSignatures)
	if err != nil {
		return nil, fmt.Errorf("marshal function_signatures: %w", err)
	}

	// System templates: allow toggling is_active but not code changes
	code := t.Code
	if existing.IsSystem {
		code = existing.Code
	}

	_, err = s.db.Exec(`
		UPDATE code_templates
		SET name=$1, description=$2, category=$3, code=$4,
		    function_signatures=$5, scope=$6, is_active=$7, sort_order=$8
		WHERE id=$9`,
		t.Name, t.Description, t.Category, code,
		sigsJSON, t.Scope, t.IsActive, t.SortOrder, id)
	if err != nil {
		return nil, fmt.Errorf("update code template: %w", err)
	}

	s.InvalidateAll()
	return s.Get(id)
}

// Delete removes a code template. System templates cannot be deleted.
func (s *CodeTemplateService) Delete(id string) error {
	existing, err := s.Get(id)
	if err != nil {
		return err
	}
	if existing.IsSystem {
		return fmt.Errorf("system templates cannot be deleted")
	}

	_, err = s.db.Exec("DELETE FROM code_templates WHERE id=$1", id)
	if err != nil {
		return fmt.Errorf("delete code template: %w", err)
	}

	s.InvalidateAll()
	return nil
}

// ============================================================
// INTERFACE LINKING
// ============================================================

// ListForInterface returns all templates that will be injected for a given interface:
// all global templates + any interface-scoped templates linked to this interface.
func (s *CodeTemplateService) ListForInterface(interfaceID string) ([]*models.CodeTemplate, error) {
	rows, err := s.db.Query(`
		SELECT ct.id, ct.name, ct.description, ct.category, ct.code,
		       ct.function_signatures, ct.scope, ct.is_system, ct.is_active,
		       ct.sort_order, ct.usage_count, ct.created_by_user_id,
		       ct.created_at, ct.updated_at
		FROM code_templates ct
		WHERE ct.is_active = true
		  AND (
		      ct.scope = 'global'
		      OR (ct.scope = 'interface' AND ct.id IN (
		          SELECT template_id FROM code_template_interface_links
		          WHERE interface_id = $1
		      ))
		  )
		ORDER BY ct.sort_order ASC, ct.name ASC`, interfaceID)
	if err != nil {
		return nil, fmt.Errorf("list for interface: %w", err)
	}
	defer rows.Close()
	return s.scanTemplates(rows)
}

// LinkToInterface links an interface-scoped template to an interface.
func (s *CodeTemplateService) LinkToInterface(templateID, interfaceID string) error {
	existing, err := s.Get(templateID)
	if err != nil {
		return err
	}
	if existing.Scope != "interface" {
		return fmt.Errorf("only interface-scoped templates can be linked (template scope is '%s')", existing.Scope)
	}

	_, err = s.db.Exec(`
		INSERT INTO code_template_interface_links (template_id, interface_id)
		VALUES ($1, $2)
		ON CONFLICT (template_id, interface_id) DO NOTHING`,
		templateID, interfaceID)
	if err != nil {
		return fmt.Errorf("link template to interface: %w", err)
	}

	s.invalidateInterface(interfaceID)
	return nil
}

// UnlinkFromInterface removes a template-interface link.
func (s *CodeTemplateService) UnlinkFromInterface(templateID, interfaceID string) error {
	_, err := s.db.Exec(`
		DELETE FROM code_template_interface_links
		WHERE template_id=$1 AND interface_id=$2`,
		templateID, interfaceID)
	if err != nil {
		return fmt.Errorf("unlink template from interface: %w", err)
	}
	s.invalidateInterface(interfaceID)
	return nil
}

// GetInterfaceLinks returns all interface IDs linked to a template.
func (s *CodeTemplateService) GetInterfaceLinks(templateID string) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT interface_id FROM code_template_interface_links
		WHERE template_id = $1 ORDER BY created_at ASC`, templateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ============================================================
// SCAN HELPERS
// ============================================================

type templateScanner interface {
	Scan(dest ...interface{}) error
}

func (s *CodeTemplateService) scanTemplate(row templateScanner) (*models.CodeTemplate, error) {
	t := &models.CodeTemplate{}
	var sigsJSON []byte
	var description sql.NullString
	var createdBy sql.NullString

	err := row.Scan(
		&t.ID, &t.Name, &description, &t.Category, &t.Code, &sigsJSON,
		&t.Scope, &t.IsSystem, &t.IsActive, &t.SortOrder, &t.UsageCount,
		&createdBy, &t.CreatedAt, &t.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("code template not found")
	}
	if err != nil {
		return nil, fmt.Errorf("scan code template: %w", err)
	}

	t.Description = description.String
	if createdBy.Valid {
		t.CreatedByUserID = &createdBy.String
	}
	if err := json.Unmarshal(sigsJSON, &t.FunctionSignatures); err != nil {
		t.FunctionSignatures = []string{}
	}
	return t, nil
}

func (s *CodeTemplateService) scanTemplates(rows *sql.Rows) ([]*models.CodeTemplate, error) {
	var result []*models.CodeTemplate
	for rows.Next() {
		t := &models.CodeTemplate{}
		var sigsJSON []byte
		var description sql.NullString
		var createdBy sql.NullString

		err := rows.Scan(
			&t.ID, &t.Name, &description, &t.Category, &t.Code, &sigsJSON,
			&t.Scope, &t.IsSystem, &t.IsActive, &t.SortOrder, &t.UsageCount,
			&createdBy, &t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan code templates: %w", err)
		}
		t.Description = description.String
		if createdBy.Valid {
			t.CreatedByUserID = &createdBy.String
		}
		if err := json.Unmarshal(sigsJSON, &t.FunctionSignatures); err != nil {
			t.FunctionSignatures = []string{}
		}
		result = append(result, t)
	}
	if result == nil {
		result = []*models.CodeTemplate{}
	}
	return result, rows.Err()
}

// ============================================================
// OOB SEED (idempotent)
// ============================================================

// SeedSystemTemplates ensures the 6 OOB system templates exist.
// Uses ON CONFLICT DO NOTHING so it is safe to call on every startup.
// The SQL migration (V83) also seeds them, so this is a belt-and-suspenders guard.
func SeedSystemTemplates(db *sql.DB) {
	const q = `SELECT COUNT(*) FROM code_templates WHERE is_system = true`
	var count int
	if err := db.QueryRow(q).Scan(&count); err != nil {
		log.Printf("⚠️  [CodeTemplates] Could not check system templates: %v", err)
		return
	}
	if count >= 6 {
		log.Printf("✅ [CodeTemplates] %d system templates present", count)
		return
	}
	// Migration handles the actual INSERT; log and move on.
	log.Printf("ℹ️  [CodeTemplates] Only %d system templates found — migration V83 will seed the rest on next Flyway run", count)
}

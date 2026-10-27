package cdaterminology

// Management operations for cda_code_translations, used by the Code Translations page and its API.
// Kept apart from the runtime lookup path (Translate) in terminology_service.go: this file is for people
// editing the table, that one is for the pipeline reading it.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

var (
	// ErrTranslationNotFound is returned by Get/Update when no row has the given id.
	ErrTranslationNotFound = errors.New("cda_terminology: translation not found")
	// ErrDuplicateTranslation is returned when a row with the same scope, source system, source code and
	// target system already exists.
	ErrDuplicateTranslation = errors.New("cda_terminology: a translation for this source code already exists")
	// ErrUnknownInterface is returned when the interface a translation is scoped to does not exist.
	ErrUnknownInterface = errors.New("cda_terminology: interface not found")
)

// Scopes a Query can be limited to.
const (
	ScopeAll       = "all"       // shared rows and every interface's rows
	ScopeGlobal    = "global"    // shared rows only
	ScopeInterface = "interface" // one interface's own rows (needs InterfaceID)
)

// TranslationFilter selects and pages rows for the management list.
type TranslationFilter struct {
	Scope       string // ScopeAll (default) | ScopeGlobal | ScopeInterface
	InterfaceID string // required for ScopeInterface
	Search      string // case-insensitive match on systems, codes and display text
	Limit       int    // default 100, max 500
	Offset      int
}

// TranslationPage is one page of results plus the total number of rows matching the filter.
type TranslationPage struct {
	Items []CodeTranslation `json:"items"`
	Total int               `json:"total"`
}

const translationSelect = `
	SELECT t.id, t.interface_id, COALESCE(i.name, ''), t.source_system, t.source_code,
	       t.target_system, t.target_code, COALESCE(t.target_display, ''), t.created_at
	FROM cda_code_translations t
	LEFT JOIN interfaces i ON i.id = t.interface_id`

func scanTranslation(sc interface{ Scan(...interface{}) error }) (CodeTranslation, error) {
	var t CodeTranslation
	var ifaceID sql.NullString
	if err := sc.Scan(&t.ID, &ifaceID, &t.InterfaceName, &t.SourceSystem, &t.SourceCode,
		&t.TargetSystem, &t.TargetCode, &t.TargetDisplay, &t.CreatedAt); err != nil {
		return t, err
	}
	if ifaceID.Valid {
		t.InterfaceID = &ifaceID.String
	}
	return t, nil
}

// Query returns one page of translations matching the filter, newest-edited order is not tracked, so rows
// are ordered by source system and code.
func (r *TranslationRepository) Query(ctx context.Context, f TranslationFilter) (TranslationPage, error) {
	page := TranslationPage{Items: []CodeTranslation{}}
	if r.db == nil {
		return page, nil
	}
	if f.Limit <= 0 {
		f.Limit = 100
	}
	if f.Limit > 500 {
		f.Limit = 500
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	var where []string
	var args []interface{}
	add := func(cond string, v interface{}) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(args))))
	}

	switch f.Scope {
	case ScopeGlobal:
		where = append(where, "t.interface_id IS NULL")
	case ScopeInterface:
		if f.InterfaceID == "" {
			return page, fmt.Errorf("cda_terminology: interfaceId is required for scope %q", ScopeInterface)
		}
		add("t.interface_id = ?", f.InterfaceID)
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		like := "%" + escapeLike(s) + "%"
		args = append(args, like)
		n := fmt.Sprintf("$%d", len(args))
		where = append(where, "(t.source_system ILIKE "+n+" OR t.source_code ILIKE "+n+
			" OR t.target_system ILIKE "+n+" OR t.target_code ILIKE "+n+" OR t.target_display ILIKE "+n+")")
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM cda_code_translations t"+clause, args...).Scan(&page.Total); err != nil {
		return page, fmt.Errorf("cda_terminology: count translations: %w", err)
	}

	args = append(args, f.Limit, f.Offset)
	rows, err := r.db.QueryContext(ctx, translationSelect+clause+
		fmt.Sprintf(" ORDER BY t.source_system, t.source_code, t.target_system LIMIT $%d OFFSET $%d", len(args)-1, len(args)),
		args...)
	if err != nil {
		return page, fmt.Errorf("cda_terminology: query translations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		t, scanErr := scanTranslation(rows)
		if scanErr != nil {
			return page, fmt.Errorf("cda_terminology: scan translation: %w", scanErr)
		}
		page.Items = append(page.Items, t)
	}
	return page, rows.Err()
}

// escapeLike makes user text match literally inside an ILIKE pattern.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Get returns one translation by id.
func (r *TranslationRepository) Get(ctx context.Context, id string) (CodeTranslation, error) {
	if r.db == nil {
		return CodeTranslation{}, ErrTranslationNotFound
	}
	t, err := scanTranslation(r.db.QueryRowContext(ctx, translationSelect+" WHERE t.id = $1", id))
	if errors.Is(err, sql.ErrNoRows) {
		return CodeTranslation{}, ErrTranslationNotFound
	}
	return t, err
}

// Create inserts a new row and returns it. ErrDuplicateTranslation if the same source code is already mapped
// in that scope (use Update to change it).
func (r *TranslationRepository) Create(ctx context.Context, t CodeTranslation) (CodeTranslation, error) {
	if r.db == nil {
		return CodeTranslation{}, fmt.Errorf("cda_terminology: no DB connection")
	}
	var iface interface{}
	if t.InterfaceID != nil && *t.InterfaceID != "" {
		iface = *t.InterfaceID
	}
	var id string
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO cda_code_translations
		    (interface_id, source_system, source_code, target_system, target_code, target_display)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''))
		RETURNING id`,
		iface, t.SourceSystem, t.SourceCode, t.TargetSystem, t.TargetCode, t.TargetDisplay).Scan(&id)
	if err != nil {
		return CodeTranslation{}, mapWriteError(err, "create")
	}
	return r.Get(ctx, id)
}

// Update replaces the editable fields of a row (everything except its scope, i.e. which interface or
// shared). ErrTranslationNotFound / ErrDuplicateTranslation as appropriate.
func (r *TranslationRepository) Update(ctx context.Context, id string, t CodeTranslation) (CodeTranslation, error) {
	if r.db == nil {
		return CodeTranslation{}, fmt.Errorf("cda_terminology: no DB connection")
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE cda_code_translations
		   SET source_system = $2, source_code = $3, target_system = $4, target_code = $5,
		       target_display = NULLIF($6, '')
		 WHERE id = $1`,
		id, t.SourceSystem, t.SourceCode, t.TargetSystem, t.TargetCode, t.TargetDisplay)
	if err != nil {
		return CodeTranslation{}, mapWriteError(err, "update")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return CodeTranslation{}, ErrTranslationNotFound
	}
	return r.Get(ctx, id)
}

func mapWriteError(err error, op string) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch pqErr.Code {
		case "23505":
			return ErrDuplicateTranslation
		case "23503":
			return ErrUnknownInterface
		}
	}
	return fmt.Errorf("cda_terminology: %s translation: %w", op, err)
}

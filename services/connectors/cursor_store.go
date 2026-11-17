// services/connectors/cursor_store.go
//
// CursorStore keeps an inbound connector's position in its source durably,
// so a restart neither repeats nor skips work: the last event sequence a
// webhook receiver finished, a poller's watermark, and so on. One value per
// (interface, key); a connector picks a key that is stable for its listener
// or source (for example its endpoint path).
//
// Connectors receive a store through CursorAwareConnector, injected by the
// engine at activation, the same way PipelineAwareConnector receives a
// pipeline executor.
package connectors

import (
	"context"
	"database/sql"
	"errors"
)

// CursorStore loads and saves connector cursors.
type CursorStore interface {
	// Load returns the saved cursor, or found=false when none was saved yet.
	Load(ctx context.Context, interfaceID, key string) (value string, found bool, err error)
	// Save records the cursor, replacing any earlier value.
	Save(ctx context.Context, interfaceID, key, value string) error
}

// CursorAwareConnector is implemented by connectors that keep a durable
// cursor.
type CursorAwareConnector interface {
	SetCursorStore(store CursorStore)
}

// PostgresCursorStore keeps cursors in the connector_cursors table (V289).
type PostgresCursorStore struct {
	db *sql.DB
}

// NewPostgresCursorStore returns a store over db.
func NewPostgresCursorStore(db *sql.DB) *PostgresCursorStore {
	return &PostgresCursorStore{db: db}
}

// Load implements CursorStore.
func (s *PostgresCursorStore) Load(ctx context.Context, interfaceID, key string) (string, bool, error) {
	if s == nil || s.db == nil {
		return "", false, errors.New("cursor store has no database")
	}
	var v string
	err := s.db.QueryRowContext(ctx,
		`SELECT cursor_value FROM connector_cursors WHERE interface_id = $1 AND cursor_key = $2`,
		interfaceID, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// Save implements CursorStore.
func (s *PostgresCursorStore) Save(ctx context.Context, interfaceID, key, value string) error {
	if s == nil || s.db == nil {
		return errors.New("cursor store has no database")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO connector_cursors (interface_id, cursor_key, cursor_value, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (interface_id, cursor_key)
		DO UPDATE SET cursor_value = EXCLUDED.cursor_value, updated_at = NOW()`,
		interfaceID, key, value)
	return err
}

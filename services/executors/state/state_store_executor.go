// services/executors/state/state_store_executor.go
// StateStoreExecutor — pipeline step type "state.store".
//
// A small key-value store with expiry that lets one message's pipeline leave
// something for a later message's: e.g. where the next page of a query's
// results starts, kept under the query's tag until the sender asks for more
// (the HL7 continuation protocol) or cancels. Entries belong to the pipeline
// and a namespace, and expire after ttlSeconds.
//
// Config keys:
//
//	operation   — "put", "get" or "delete"
//	namespace   — groups entries within the pipeline (e.g. "pdq_continuation")
//	keyFrom     — paths whose values, joined by "|", make the key (e.g. the
//	              sender's application and the query tag)
//	skipWhen    — a path: when it holds a value (other than false), the
//	              step does nothing, e.g. don't replace a stored page while
//	              answering a query that named an unknown one
//
//	put:  values       — []{name, path, transform?}: the entry's fields;
//	                     transform "url_query" keeps only a URL's query string
//	      requireValue — the name of a value without which nothing is stored
//	                     and any existing entry is deleted (e.g. no next page)
//	      token        — true adds a random "token" field, also output, which
//	                     a later get can be required to match
//	      ttlSeconds   — how long the entry lives (default 600)
//	get:  when         — a path: look up only when it has a value (e.g. the
//	                     continuation pointer); otherwise the step does nothing
//	      matchToken   — a path whose value must equal the stored token
//	delete: (key only)
//
// Output: put {stored, token}; get {found, missing, value}: missing is true
// when a lookup was asked for (when had a value) but there was no live entry
// or its token didn't match; delete {deleted}.
package state

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
)

// Store keeps entries by scope and key until they expire.
type Store interface {
	Put(ctx context.Context, scope, key string, value map[string]interface{}, ttl time.Duration) error
	// Get returns a live entry; ok is false when there is none.
	Get(ctx context.Context, scope, key string) (map[string]interface{}, bool, error)
	Delete(ctx context.Context, scope, key string) (bool, error)
}

// StateStoreExecutor runs state.store steps against a Store.
type StateStoreExecutor struct {
	*executors.BaseExecutor
	store Store
}

// NewStateStoreExecutor constructs the executor over store.
func NewStateStoreExecutor(store Store) *StateStoreExecutor {
	return &StateStoreExecutor{
		BaseExecutor: executors.NewBaseExecutor("state.store", models.ExecutorMetadata{
			Name:        "State Store",
			Description: "Keeps a small value with an expiry for a later message's pipeline (e.g. a query's next page)",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "Control",
		}),
		store: store,
	}
}

type stateValue struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Transform string `json:"transform,omitempty"`
}

type stateConfig struct {
	Operation    string       `json:"operation"`
	Namespace    string       `json:"namespace"`
	KeyFrom      []string     `json:"keyFrom"`
	Values       []stateValue `json:"values"`
	RequireValue string       `json:"requireValue"`
	Token        bool         `json:"token"`
	TTLSeconds   int          `json:"ttlSeconds"`
	When         string       `json:"when"`
	SkipWhen     string       `json:"skipWhen"`
	MatchToken   string       `json:"matchToken"`
}

const defaultTTL = 10 * time.Minute

// valueTransforms shape a stored value.
var valueTransforms = map[string]func(string) string{
	"url_query": func(v string) string {
		if u, err := url.Parse(v); err == nil && strings.Contains(v, "?") {
			return u.RawQuery
		}
		return ""
	},
}

func readConfig(step *models.TransformationStep) (stateConfig, error) {
	var cfg stateConfig
	raw, _ := json.Marshal(step.Config)
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("state.store: invalid config: %w", err)
	}
	return cfg, nil
}

// Execute runs the configured operation.
func (e *StateStoreExecutor) Execute(ctx context.Context, step *models.TransformationStep, inputData map[string]interface{}) (map[string]interface{}, error) {
	start := time.Now()
	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}
	cfg, err := readConfig(step)
	if err != nil {
		return nil, err
	}
	if e.store == nil {
		return nil, fmt.Errorf("state.store: no store is configured")
	}
	scope := step.PipelineID + "/" + cfg.Namespace
	key, haveKey := e.key(cfg, inputData)

	var out map[string]interface{}
	if v := stringAt(inputData, cfg.SkipWhen); v != "" && v != "false" {
		cfg.Operation = "skipped"
	}
	switch cfg.Operation {
	case "skipped":
		out = map[string]interface{}{"skipped": true}
	case "put":
		out, err = e.put(ctx, cfg, scope, key, haveKey, inputData)
	case "get":
		out, err = e.get(ctx, cfg, scope, key, haveKey, inputData)
	case "delete":
		deleted := false
		if haveKey {
			deleted, err = e.store.Delete(ctx, scope, key)
		}
		out = map[string]interface{}{"deleted": deleted}
	default:
		return nil, fmt.Errorf("state.store: operation %q is not put, get or delete", cfg.Operation)
	}
	if err != nil {
		return nil, fmt.Errorf("state.store %s: %w", cfg.Operation, err)
	}

	outputData := make(map[string]interface{}, len(inputData))
	for k, v := range inputData {
		outputData[k] = v
	}
	log.Printf("  ✅ [state.store] %s %s in %dms", cfg.Operation, cfg.Namespace, time.Since(start).Milliseconds())
	e.SetStepOutputWithDetails(outputData, out, map[string]interface{}{"duration_ms": time.Since(start).Milliseconds(), "success": true})
	return outputData, nil
}

// key joins the keyFrom values; haveKey is false when any is empty.
func (e *StateStoreExecutor) key(cfg stateConfig, data map[string]interface{}) (string, bool) {
	if len(cfg.KeyFrom) == 0 {
		return "", false
	}
	parts := make([]string, len(cfg.KeyFrom))
	for i, p := range cfg.KeyFrom {
		parts[i] = stringAt(data, p)
		if parts[i] == "" {
			return "", false
		}
	}
	return strings.Join(parts, "|"), true
}

func (e *StateStoreExecutor) put(ctx context.Context, cfg stateConfig, scope, key string, haveKey bool, data map[string]interface{}) (map[string]interface{}, error) {
	out := map[string]interface{}{"stored": false}
	if !haveKey {
		return out, nil
	}
	value := map[string]interface{}{}
	for _, v := range cfg.Values {
		s := stringAt(data, v.Path)
		if t, ok := valueTransforms[v.Transform]; ok {
			s = t(s)
		} else if v.Transform != "" {
			return nil, fmt.Errorf("unknown transform %q", v.Transform)
		}
		if s != "" {
			value[v.Name] = s
		}
	}
	if cfg.RequireValue != "" && value[cfg.RequireValue] == nil {
		_, err := e.store.Delete(ctx, scope, key)
		return out, err
	}
	if cfg.Token {
		token, err := newToken()
		if err != nil {
			return nil, err
		}
		value["token"] = token
		out["token"] = token
	}
	ttl := defaultTTL
	if cfg.TTLSeconds > 0 {
		ttl = time.Duration(cfg.TTLSeconds) * time.Second
	}
	if err := e.store.Put(ctx, scope, key, value, ttl); err != nil {
		return nil, err
	}
	out["stored"] = true
	return out, nil
}

func (e *StateStoreExecutor) get(ctx context.Context, cfg stateConfig, scope, key string, haveKey bool, data map[string]interface{}) (map[string]interface{}, error) {
	out := map[string]interface{}{"found": false, "missing": false}
	if cfg.When != "" && stringAt(data, cfg.When) == "" {
		return out, nil
	}
	if !haveKey {
		out["missing"] = true
		return out, nil
	}
	value, ok, err := e.store.Get(ctx, scope, key)
	if err != nil {
		return nil, err
	}
	if ok && cfg.MatchToken != "" {
		stored, _ := value["token"].(string)
		ok = stored != "" && stored == stringAt(data, cfg.MatchToken)
	}
	if !ok {
		out["missing"] = true
		return out, nil
	}
	out["found"] = true
	out["value"] = value
	return out, nil
}

func stringAt(data map[string]interface{}, path string) string {
	if path == "" {
		return ""
	}
	v := executors.GetNestedValue(data, path)
	if v == nil {
		v = executors.GetFieldValue(data, path)
	}
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Validate checks the operation and key.
func (e *StateStoreExecutor) Validate(step *models.TransformationStep) error {
	cfg, err := readConfig(step)
	if err != nil {
		return err
	}
	switch cfg.Operation {
	case "put", "get", "delete":
	default:
		return fmt.Errorf("state.store: operation %q is not put, get or delete", cfg.Operation)
	}
	if len(cfg.KeyFrom) == 0 {
		return fmt.Errorf("state.store: keyFrom is required")
	}
	return nil
}

// GetOutputVariables declares the outputs for the field picker.
func (e *StateStoreExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		{Name: "Token", Path: "token", DataType: "string", Description: "put: the random token stored", Category: "Control"},
		{Name: "Found", Path: "found", DataType: "boolean", Description: "get: a live entry was found", Category: "Control"},
		{Name: "Missing", Path: "missing", DataType: "boolean", Description: "get: asked for, but none (or the token didn't match)", Category: "Control"},
		{Name: "Value", Path: "value", DataType: "object", Description: "get: the entry's fields", Category: "Control"},
	}
}

// PostgresStore keeps entries in the pipeline_state table.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore returns a Store over db (nil when db is nil).
func NewPostgresStore(db *sql.DB) Store {
	if db == nil {
		return nil
	}
	return &PostgresStore{db: db}
}

// Put stores value under (scope, key), replacing any entry, and clears
// expired entries.
func (s *PostgresStore) Put(ctx context.Context, scope, key string, value map[string]interface{}, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM pipeline_state WHERE expires_at < now()`); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO pipeline_state (scope, key, value, expires_at)
		VALUES ($1, $2, $3, now() + make_interval(secs => $4))
		ON CONFLICT (scope, key) DO UPDATE SET value = EXCLUDED.value, expires_at = EXCLUDED.expires_at`,
		scope, key, raw, ttl.Seconds())
	return err
}

// Get returns the live entry under (scope, key).
func (s *PostgresStore) Get(ctx context.Context, scope, key string) (map[string]interface{}, bool, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM pipeline_state WHERE scope = $1 AND key = $2 AND expires_at > now()`, scope, key).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	value := map[string]interface{}{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, false, err
	}
	return value, true, nil
}

// Delete removes the entry under (scope, key).
func (s *PostgresStore) Delete(ctx context.Context, scope, key string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM pipeline_state WHERE scope = $1 AND key = $2`, scope, key)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

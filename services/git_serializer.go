package services

// ================================================================
// GitSerializer — converts one interface and all its configuration
// into a portable InterfaceBundle suitable for git storage.
//
// Sensitive values are replaced with {{ENV_VAR}} placeholders so
// the bundle is safe to commit to a repository without leaking
// credentials.  The same isSensitiveKey() logic from credential_store.go
// is reused — no separate allow-list required.
// ================================================================

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"ezhealthkonnect/models"
)

// GitSerializer queries PostgreSQL and builds an InterfaceBundle.
type GitSerializer struct {
	db        *sql.DB
	credStore *CredentialStore
}

// NewGitSerializer creates a GitSerializer.
func NewGitSerializer(db *sql.DB, credStore *CredentialStore) *GitSerializer {
	return &GitSerializer{db: db, credStore: credStore}
}

// ── Public API ────────────────────────────────────────────────────────────────

// SerializeInterface builds a complete InterfaceBundle for the given interface ID.
// Returns (nil, nil) when the interface does not exist (caller should 404).
func (gs *GitSerializer) SerializeInterface(
	ctx context.Context,
	interfaceID string,
	exportedBy string,
) (*models.InterfaceBundle, error) {

	// 1. Interface row
	iface, err := gs.queryInterface(ctx, interfaceID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query interface: %w", err)
	}

	// 2. Pipelines + steps
	pipelines, err := gs.queryPipelines(ctx, interfaceID)
	if err != nil {
		return nil, fmt.Errorf("query pipelines: %w", err)
	}

	// 3. Connectivity config
	connectivity, err := gs.queryConnectivity(ctx, interfaceID)
	if err != nil {
		return nil, fmt.Errorf("query connectivity: %w", err)
	}

	name, _ := iface["name"].(string)

	return &models.InterfaceBundle{
		BundleVersion: "1.0",
		ExportedAt:    time.Now().UTC(),
		Metadata: models.InterfaceBundleMetadata{
			InterfaceID:   interfaceID,
			InterfaceName: name,
			ExportedBy:    exportedBy,
			AppVersion:    "2.0",
		},
		Interface:    iface,
		Pipelines:    pipelines,
		Connectivity: connectivity,
	}, nil
}

// MakeInterfaceSlug converts an interface name to a git-safe directory name.
// "Epic ADT Inbound v2" → "epic-adt-inbound-v2"
func MakeInterfaceSlug(name string) string {
	// Lowercase
	s := strings.ToLower(name)
	// Replace any non-alphanumeric run with a single hyphen
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "-")
	// Trim leading/trailing hyphens
	s = strings.Trim(s, "-")
	if s == "" {
		return "interface"
	}
	return s
}

// ── Database queries ──────────────────────────────────────────────────────────

func (gs *GitSerializer) queryInterface(ctx context.Context, id string) (map[string]interface{}, error) {
	row := gs.db.QueryRowContext(ctx, `
		SELECT id, name, COALESCE(description,'') AS description,
		       COALESCE(source_type,'') AS source_type,
		       COALESCE(target_type,'') AS target_type,
		       COALESCE(message_type,'') AS message_type,
		       COALESCE(status,'') AS status,
		       is_active, created_at, updated_at
		FROM interfaces
		WHERE id = $1`, id)

	var (
		rid, name, desc, srcType, tgtType, msgType, status string
		isActive                                            bool
		createdAt, updatedAt                               time.Time
	)
	if err := row.Scan(&rid, &name, &desc, &srcType, &tgtType, &msgType, &status, &isActive, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"id":           rid,
		"name":         name,
		"description":  desc,
		"source_type":  srcType,
		"target_type":  tgtType,
		"message_type": msgType,
		"status":       status,
		"is_active":    isActive,
		"created_at":   createdAt,
		"updated_at":   updatedAt,
	}, nil
}

func (gs *GitSerializer) queryPipelines(ctx context.Context, interfaceID string) ([]models.PipelineBundle, error) {
	rows, err := gs.db.QueryContext(ctx, `
		SELECT id, interface_id,
		       COALESCE(message_type,'') AS message_type,
		       COALESCE(pipeline_name,'') AS pipeline_name,
		       COALESCE(enabled, true) AS enabled,
		       COALESCE(version, 1) AS version
		FROM transformation_pipelines
		WHERE interface_id = $1
		ORDER BY message_type`, interfaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bundles []models.PipelineBundle
	for rows.Next() {
		var pid, ifid, msgType, pname string
		var enabled bool
		var version int
		if err := rows.Scan(&pid, &ifid, &msgType, &pname, &enabled, &version); err != nil {
			continue
		}
		steps, err := gs.querySteps(ctx, pid)
		if err != nil {
			return nil, fmt.Errorf("steps for pipeline %s: %w", pid, err)
		}
		bundles = append(bundles, models.PipelineBundle{
			Pipeline: map[string]interface{}{
				"id":            pid,
				"interface_id":  ifid,
				"message_type":  msgType,
				"pipeline_name": pname,
				"enabled":       enabled,
				"version":       version,
			},
			Steps: steps,
		})
	}
	return bundles, rows.Err()
}

func (gs *GitSerializer) querySteps(ctx context.Context, pipelineID string) ([]map[string]interface{}, error) {
	rows, err := gs.db.QueryContext(ctx, `
		SELECT id, pipeline_id, COALESCE(step_name,'') AS step_name,
		       COALESCE(step_type,'') AS step_type,
		       COALESCE(sequence, 0) AS sequence,
		       COALESCE(enabled, true) AS enabled,
		       COALESCE(config, '{}')::text AS config,
		       COALESCE(on_error_strategy,'rethrow') AS on_error_strategy,
		       COALESCE(execution_mode,'sequential') AS execution_mode,
		       COALESCE(position_x, 0) AS position_x,
		       COALESCE(position_y, 0) AS position_y
		FROM transformation_steps
		WHERE pipeline_id = $1
		ORDER BY sequence ASC`, pipelineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []map[string]interface{}
	for rows.Next() {
		var id, pid, name, stepType, configStr, onErr, execMode string
		var seq, posX, posY int
		var enabled bool
		if err := rows.Scan(&id, &pid, &name, &stepType, &seq, &enabled,
			&configStr, &onErr, &execMode, &posX, &posY); err != nil {
			continue
		}

		// Parse config JSON and scrub sensitive fields
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(configStr), &cfg); err != nil {
			cfg = map[string]interface{}{}
		}
		cfg = gs.placeholderSensitiveValues(cfg)

		steps = append(steps, map[string]interface{}{
			"id":                 id,
			"pipeline_id":        pid,
			"step_name":          name,
			"step_type":          stepType,
			"sequence":           seq,
			"enabled":            enabled,
			"config":             cfg,
			"on_error_strategy":  onErr,
			"execution_mode":     execMode,
			"position_x":         posX,
			"position_y":         posY,
		})
	}
	return steps, rows.Err()
}

func (gs *GitSerializer) queryConnectivity(ctx context.Context, interfaceID string) (map[string]interface{}, error) {
	row := gs.db.QueryRowContext(ctx, `
		SELECT COALESCE(source_connectivity_type_id::text,'') AS src_type_id,
		       COALESCE(source_config, '{}')::text AS src_config,
		       COALESCE(source_enabled, false) AS src_enabled,
		       COALESCE(target_connectivity_type_id::text,'') AS tgt_type_id,
		       COALESCE(target_config, '{}')::text AS tgt_config,
		       COALESCE(target_enabled, false) AS tgt_enabled
		FROM interface_connectivity
		WHERE interface_id = $1`, interfaceID)

	var srcTypeID, srcConfigStr, tgtTypeID, tgtConfigStr string
	var srcEnabled, tgtEnabled bool

	err := row.Scan(&srcTypeID, &srcConfigStr, &srcEnabled, &tgtTypeID, &tgtConfigStr, &tgtEnabled)
	if err == sql.ErrNoRows {
		return map[string]interface{}{}, nil
	}
	if err != nil {
		return nil, err
	}

	srcConfig := gs.decryptAndPlaceholder([]byte(srcConfigStr))
	tgtConfig := gs.decryptAndPlaceholder([]byte(tgtConfigStr))

	return map[string]interface{}{
		"source": map[string]interface{}{
			"connectivity_type_id": srcTypeID,
			"config":               srcConfig,
			"enabled":              srcEnabled,
		},
		"target": map[string]interface{}{
			"connectivity_type_id": tgtTypeID,
			"config":               tgtConfig,
			"enabled":              tgtEnabled,
		},
	}, nil
}

// ── Sensitive value scrubbing ─────────────────────────────────────────────────

// decryptAndPlaceholder decrypts a JSONB blob (if encrypted) and then replaces
// all sensitive field values with {{ENV_VAR}} placeholders.
func (gs *GitSerializer) decryptAndPlaceholder(raw []byte) map[string]interface{} {
	var m map[string]interface{}

	// Try decrypt first (no-op in dev passthrough mode)
	if gs.credStore != nil {
		if dec, err := gs.credStore.DecryptConfig(raw); err == nil {
			raw = dec
		}
	}

	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]interface{}{}
	}
	return gs.placeholderSensitiveValues(m)
}

// placeholderSensitiveValues recursively walks a map and replaces values
// whose key is sensitive (password, token, secret, api_key, etc.) with
// an {{UPPER_SNAKE_CASE}} placeholder.
func (gs *GitSerializer) placeholderSensitiveValues(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return m
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		if isSensitiveKey(k) {
			result[k] = "{{" + makeEnvVarName(k) + "}}"
		} else {
			switch val := v.(type) {
			case map[string]interface{}:
				result[k] = gs.placeholderSensitiveValues(val)
			default:
				result[k] = val
			}
		}
	}
	return result
}

// makeEnvVarName converts camelCase or snake_case to UPPER_SNAKE_CASE.
// "accessKeyId" → "ACCESS_KEY_ID", "db_password" → "DB_PASSWORD"
func makeEnvVarName(key string) string {
	// Insert underscore before each uppercase letter in camelCase runs
	var b strings.Builder
	runes := []rune(key)
	for i, r := range runes {
		if unicode.IsUpper(r) && i > 0 && !unicode.IsUpper(runes[i-1]) {
			b.WriteRune('_')
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	s := b.String()
	// Replace non-alphanumeric with underscore, collapse runs
	re := regexp.MustCompile(`[^A-Z0-9]+`)
	s = re.ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}

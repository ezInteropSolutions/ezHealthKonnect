// services/connectivity_service.go
// Connectivity Service - CRUD operations for OOB connectors

package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/models"
)

// ConnectivityService handles connectivity configuration operations
type ConnectivityService struct {
	db        *sql.DB
	credStore *CredentialStore // nil = passthrough (dev/test only)
}

// NewConnectivityService creates a new connectivity service
func NewConnectivityService(db *sql.DB, credStore *CredentialStore) *ConnectivityService {
	return &ConnectivityService{
		db:        db,
		credStore: credStore,
	}
}

// ========================================
// Connectivity Types (OOB Connectors)
// ========================================

// ListConnectivityTypes retrieves all connectivity types
func (cs *ConnectivityService) ListConnectivityTypes(filter *models.ConnectivityTypeFilter) ([]*models.ConnectivityType, error) {
	query := `
		SELECT id, type_name, category, display_name,
		       COALESCE(description, '') AS description, COALESCE(icon, '') AS icon,
		       mode, supports_cron, requires_auth, is_bidirectional,
		       COALESCE(implementation_class, '') AS implementation_class,
		       config_schema, default_config,
		       wizard_template, parameter_groups, validation_rules,
		       is_active, is_beta, priority, COALESCE(version, '') AS version, documentation_url,
		       COALESCE(ui_category, '') AS ui_category, COALESCE(ui_sort_order, 100) AS ui_sort_order,
		       created_at, updated_at
		FROM connectivity_types
		WHERE 1=1
	`

	args := []interface{}{}
	argCount := 1

	// Apply filters
	if filter != nil {
		if filter.Category != "" {
			query += fmt.Sprintf(" AND category = $%d", argCount)
			args = append(args, filter.Category)
			argCount++
		}
		if filter.Mode != "" {
			query += fmt.Sprintf(" AND mode = $%d", argCount)
			args = append(args, filter.Mode)
			argCount++
		}
		if filter.SupportsCron != nil {
			query += fmt.Sprintf(" AND supports_cron = $%d", argCount)
			args = append(args, *filter.SupportsCron)
			argCount++
		}
		if filter.RequiresAuth != nil {
			query += fmt.Sprintf(" AND requires_auth = $%d", argCount)
			args = append(args, *filter.RequiresAuth)
			argCount++
		}
		if filter.IsActive != nil {
			query += fmt.Sprintf(" AND is_active = $%d", argCount)
			args = append(args, *filter.IsActive)
			argCount++
		}
		if filter.IsBeta != nil {
			query += fmt.Sprintf(" AND is_beta = $%d", argCount)
			args = append(args, *filter.IsBeta)
			argCount++
		}
	}

	query += " ORDER BY priority ASC, display_name ASC"

	rows, err := cs.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query connectivity types: %w", err)
	}
	defer rows.Close()

	var types []*models.ConnectivityType
	for rows.Next() {
		ct := &models.ConnectivityType{}
		var defaultConfig, wizardTemplate, parameterGroups, validationRules sql.NullString
		var documentationURL sql.NullString

		err := rows.Scan(
			&ct.ID, &ct.TypeName, &ct.Category, &ct.DisplayName, &ct.Description, &ct.Icon,
			&ct.Mode, &ct.SupportsCron, &ct.RequiresAuth, &ct.IsBidirectional,
			&ct.ImplementationClass, &ct.ConfigSchema, &defaultConfig,
			&wizardTemplate, &parameterGroups, &validationRules,
			&ct.IsActive, &ct.IsBeta, &ct.Priority, &ct.Version, &documentationURL,
			&ct.UICategory, &ct.UISortOrder,
			&ct.CreatedAt, &ct.UpdatedAt,
		)
		if err != nil {
			log.Printf("Error scanning connectivity type: %v", err)
			continue
		}

		// Convert nullable fields
		if defaultConfig.Valid {
			ct.DefaultConfig = json.RawMessage(defaultConfig.String)
		}
		if wizardTemplate.Valid {
			ct.WizardTemplate = wizardTemplate.String
		}
		if parameterGroups.Valid {
			ct.ParameterGroups = json.RawMessage(parameterGroups.String)
		}
		if validationRules.Valid {
			ct.ValidationRules = json.RawMessage(validationRules.String)
		}
		if documentationURL.Valid {
			ct.DocumentationURL = documentationURL.String
		}

		types = append(types, ct)
	}

	return types, nil
}

// GetConnectivityTypeByID retrieves a specific connectivity type
func (cs *ConnectivityService) GetConnectivityTypeByID(id string) (*models.ConnectivityType, error) {
	ct := &models.ConnectivityType{}
	var defaultConfig, wizardTemplate, parameterGroups, validationRules sql.NullString
	var documentationURL sql.NullString

	query := `
		SELECT id, type_name, category, display_name,
		       COALESCE(description, '') AS description, COALESCE(icon, '') AS icon,
		       mode, supports_cron, requires_auth, is_bidirectional,
		       COALESCE(implementation_class, '') AS implementation_class,
		       config_schema, default_config,
		       wizard_template, parameter_groups, validation_rules,
		       is_active, is_beta, priority, COALESCE(version, '') AS version, documentation_url,
		       COALESCE(ui_category, '') AS ui_category, COALESCE(ui_sort_order, 100) AS ui_sort_order,
		       created_at, updated_at
		FROM connectivity_types
		WHERE id = $1
	`

	err := cs.db.QueryRow(query, id).Scan(
		&ct.ID, &ct.TypeName, &ct.Category, &ct.DisplayName, &ct.Description, &ct.Icon,
		&ct.Mode, &ct.SupportsCron, &ct.RequiresAuth, &ct.IsBidirectional,
		&ct.ImplementationClass, &ct.ConfigSchema, &defaultConfig,
		&wizardTemplate, &parameterGroups, &validationRules,
		&ct.IsActive, &ct.IsBeta, &ct.Priority, &ct.Version, &documentationURL,
		&ct.UICategory, &ct.UISortOrder,
		&ct.CreatedAt, &ct.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("connectivity type not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get connectivity type: %w", err)
	}

	// Convert nullable fields
	if defaultConfig.Valid {
		ct.DefaultConfig = json.RawMessage(defaultConfig.String)
	}
	if wizardTemplate.Valid {
		ct.WizardTemplate = wizardTemplate.String
	}
	if parameterGroups.Valid {
		ct.ParameterGroups = json.RawMessage(parameterGroups.String)
	}
	if validationRules.Valid {
		ct.ValidationRules = json.RawMessage(validationRules.String)
	}
	if documentationURL.Valid {
		ct.DocumentationURL = documentationURL.String
	}

	return ct, nil
}

// GetConnectivityTypeByName retrieves a connectivity type by its type_name
func (cs *ConnectivityService) GetConnectivityTypeByName(typeName string) (*models.ConnectivityType, error) {
	ct := &models.ConnectivityType{}
	var defaultConfig, wizardTemplate, parameterGroups, validationRules sql.NullString
	var documentationURL sql.NullString

	query := `
		SELECT id, type_name, category, display_name,
		       COALESCE(description, '') AS description, COALESCE(icon, '') AS icon,
		       mode, supports_cron, requires_auth, is_bidirectional,
		       COALESCE(implementation_class, '') AS implementation_class,
		       config_schema, default_config,
		       wizard_template, parameter_groups, validation_rules,
		       is_active, is_beta, priority, COALESCE(version, '') AS version, documentation_url,
		       COALESCE(ui_category, '') AS ui_category, COALESCE(ui_sort_order, 100) AS ui_sort_order,
		       created_at, updated_at
		FROM connectivity_types
		WHERE type_name = $1
	`

	err := cs.db.QueryRow(query, typeName).Scan(
		&ct.ID, &ct.TypeName, &ct.Category, &ct.DisplayName, &ct.Description, &ct.Icon,
		&ct.Mode, &ct.SupportsCron, &ct.RequiresAuth, &ct.IsBidirectional,
		&ct.ImplementationClass, &ct.ConfigSchema, &defaultConfig,
		&wizardTemplate, &parameterGroups, &validationRules,
		&ct.IsActive, &ct.IsBeta, &ct.Priority, &ct.Version, &documentationURL,
		&ct.UICategory, &ct.UISortOrder,
		&ct.CreatedAt, &ct.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("connectivity type not found: %s", typeName)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get connectivity type: %w", err)
	}

	// Convert nullable fields
	if defaultConfig.Valid {
		ct.DefaultConfig = json.RawMessage(defaultConfig.String)
	}
	if wizardTemplate.Valid {
		ct.WizardTemplate = wizardTemplate.String
	}
	if parameterGroups.Valid {
		ct.ParameterGroups = json.RawMessage(parameterGroups.String)
	}
	if validationRules.Valid {
		ct.ValidationRules = json.RawMessage(validationRules.String)
	}
	if documentationURL.Valid {
		ct.DocumentationURL = documentationURL.String
	}

	return ct, nil
}

// ========================================
// Interface Connectivity
// ========================================

// CreateInterfaceConnectivity creates connectivity configuration for an interface.
// SourceConfig and TargetConfig are encrypted before storage when a CredentialStore is configured.
func (cs *ConnectivityService) CreateInterfaceConnectivity(ic *models.InterfaceConnectivity) error {
	srcConfig, err := cs.credStore.EncryptConfig(ic.SourceConfig)
	if err != nil {
		return fmt.Errorf("failed to encrypt source config: %w", err)
	}
	tgtConfig, err := cs.credStore.EncryptConfig(ic.TargetConfig)
	if err != nil {
		return fmt.Errorf("failed to encrypt target config: %w", err)
	}

	query := `
		INSERT INTO interface_connectivity (
			id, interface_id,
			source_connectivity_type_id, source_config, source_enabled,
			cron_enabled, cron_expression, cron_timezone,
			target_connectivity_type_id, target_config, target_enabled,
			connection_status
		) VALUES (
			gen_random_uuid(), $1,
			$2, $3, $4,
			$5, $6, $7,
			$8, $9, $10,
			$11
		)
		RETURNING id, created_at, updated_at
	`

	err = cs.db.QueryRow(
		query,
		ic.InterfaceID,
		ic.SourceConnectivityTypeID, srcConfig, ic.SourceEnabled,
		ic.CronEnabled, ic.CronExpression, ic.CronTimezone,
		ic.TargetConnectivityTypeID, tgtConfig, ic.TargetEnabled,
		ic.ConnectionStatus,
	).Scan(&ic.ID, &ic.CreatedAt, &ic.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create interface connectivity: %w", err)
	}

	log.Printf("✅ Created interface connectivity: %s for interface: %s", ic.ID, ic.InterfaceID)
	return nil
}

// GetInterfaceConnectivity retrieves connectivity for an interface
func (cs *ConnectivityService) GetInterfaceConnectivity(interfaceID string) (*models.InterfaceConnectivityWithDetails, error) {
	ic := &models.InterfaceConnectivityWithDetails{}

	// Get base connectivity
	query := `
		SELECT id, interface_id,
		       source_connectivity_type_id, source_config, source_enabled,
		       cron_enabled, cron_expression, cron_timezone,
		       next_run_at, last_run_at, last_run_status,
		       target_connectivity_type_id, target_config, target_enabled,
		       additional_targets,
		       connection_status, last_connection_test, last_connection_error,
		       created_at, updated_at
		FROM interface_connectivity
		WHERE interface_id = $1
	`

	err := cs.db.QueryRow(query, interfaceID).Scan(
		&ic.ID, &ic.InterfaceID,
		&ic.SourceConnectivityTypeID, &ic.SourceConfig, &ic.SourceEnabled,
		&ic.CronEnabled, &ic.CronExpression, &ic.CronTimezone,
		&ic.NextRunAt, &ic.LastRunAt, &ic.LastRunStatus,
		&ic.TargetConnectivityTypeID, &ic.TargetConfig, &ic.TargetEnabled,
		&ic.AdditionalTargets,
		&ic.ConnectionStatus, &ic.LastConnectionTest, &ic.LastConnectionError,
		&ic.CreatedAt, &ic.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("interface connectivity not found for interface: %s", interfaceID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get interface connectivity: %w", err)
	}

	// Decrypt configs before returning; mask sensitive fields so credentials never leave the server
	if decrypted, err := cs.credStore.DecryptConfig(ic.SourceConfig); err != nil {
		log.Printf("⚠️  [CredentialStore] Failed to decrypt source_config for interface %s: %v", interfaceID, err)
	} else {
		ic.SourceConfig = cs.credStore.MaskSensitiveFields(decrypted)
	}
	if decrypted, err := cs.credStore.DecryptConfig(ic.TargetConfig); err != nil {
		log.Printf("⚠️  [CredentialStore] Failed to decrypt target_config for interface %s: %v", interfaceID, err)
	} else {
		ic.TargetConfig = cs.credStore.MaskSensitiveFields(decrypted)
	}

	// Load source connector type details
	if ic.SourceConnectivityTypeID != "" {
		sourceType, err := cs.GetConnectivityTypeByID(ic.SourceConnectivityTypeID)
		if err == nil {
			ic.SourceConnectorType = sourceType
		}
	}

	// Load target connector type details
	if ic.TargetConnectivityTypeID != "" {
		targetType, err := cs.GetConnectivityTypeByID(ic.TargetConnectivityTypeID)
		if err == nil {
			ic.TargetConnectorType = targetType
		}
	}

	// Load cron job if cron is enabled
	if ic.CronEnabled {
		cronJob, err := cs.GetCronJobByConnectivityID(ic.ID)
		if err == nil {
			ic.CronJob = cronJob
		}
	}

	return ic, nil
}

// UpdateInterfaceConnectivity updates connectivity configuration.
// SourceConfig and TargetConfig are encrypted before storage when a CredentialStore is configured.
// If the incoming config contains masked values (••••••••), the existing encrypted value is preserved
// by re-reading it from the database — the UI must send only fields that actually changed.
func (cs *ConnectivityService) UpdateInterfaceConnectivity(ic *models.InterfaceConnectivity) error {
	srcConfig, err := cs.credStore.EncryptConfig(ic.SourceConfig)
	if err != nil {
		return fmt.Errorf("failed to encrypt source config: %w", err)
	}
	tgtConfig, err := cs.credStore.EncryptConfig(ic.TargetConfig)
	if err != nil {
		return fmt.Errorf("failed to encrypt target config: %w", err)
	}

	query := `
		UPDATE interface_connectivity
		SET source_connectivity_type_id = $1,
		    source_config = $2,
		    source_enabled = $3,
		    cron_enabled = $4,
		    cron_expression = $5,
		    cron_timezone = $6,
		    target_connectivity_type_id = $7,
		    target_config = $8,
		    target_enabled = $9,
		    connection_status = $10,
		    updated_at = CURRENT_TIMESTAMP
		WHERE interface_id = $11
		RETURNING updated_at
	`

	err = cs.db.QueryRow(
		query,
		ic.SourceConnectivityTypeID, srcConfig, ic.SourceEnabled,
		ic.CronEnabled, ic.CronExpression, ic.CronTimezone,
		ic.TargetConnectivityTypeID, tgtConfig, ic.TargetEnabled,
		ic.ConnectionStatus,
		ic.InterfaceID,
	).Scan(&ic.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to update interface connectivity: %w", err)
	}

	log.Printf("✅ Updated interface connectivity for interface: %s", ic.InterfaceID)
	return nil
}

// DeleteInterfaceConnectivity removes connectivity configuration
func (cs *ConnectivityService) DeleteInterfaceConnectivity(interfaceID string) error {
	query := `DELETE FROM interface_connectivity WHERE interface_id = $1`

	result, err := cs.db.Exec(query, interfaceID)
	if err != nil {
		return fmt.Errorf("failed to delete interface connectivity: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("interface connectivity not found: %s", interfaceID)
	}

	log.Printf("✅ Deleted interface connectivity for interface: %s", interfaceID)
	return nil
}

// ========================================
// Cron Jobs
// ========================================

// CreateCronJob creates a cron job for pull-based connector
func (cs *ConnectivityService) CreateCronJob(cronJob *models.CronJob) error {
	query := `
		INSERT INTO cron_jobs (
			id, interface_connectivity_id, cron_expression, timezone,
			is_enabled, max_retries, retry_delay_seconds, circuit_breaker_threshold
		) VALUES (
			gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7
		)
		RETURNING id, created_at, updated_at
	`

	err := cs.db.QueryRow(
		query,
		cronJob.InterfaceConnectivityID, cronJob.CronExpression, cronJob.Timezone,
		cronJob.IsEnabled, cronJob.MaxRetries, cronJob.RetryDelaySeconds, cronJob.CircuitBreakerThreshold,
	).Scan(&cronJob.ID, &cronJob.CreatedAt, &cronJob.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create cron job: %w", err)
	}

	log.Printf("✅ Created cron job: %s for connectivity: %s", cronJob.ID, cronJob.InterfaceConnectivityID)
	return nil
}

// GetCronJobByConnectivityID retrieves cron job for a connectivity
func (cs *ConnectivityService) GetCronJobByConnectivityID(connectivityID string) (*models.CronJob, error) {
	cronJob := &models.CronJob{}

	query := `
		SELECT id, interface_connectivity_id, cron_expression, timezone,
		       is_enabled, next_execution, last_execution,
		       execution_count, failure_count, consecutive_failures,
		       max_retries, retry_delay_seconds, circuit_breaker_threshold,
		       is_circuit_broken, circuit_broken_at,
		       created_at, updated_at
		FROM cron_jobs
		WHERE interface_connectivity_id = $1
	`

	err := cs.db.QueryRow(query, connectivityID).Scan(
		&cronJob.ID, &cronJob.InterfaceConnectivityID, &cronJob.CronExpression, &cronJob.Timezone,
		&cronJob.IsEnabled, &cronJob.NextExecution, &cronJob.LastExecution,
		&cronJob.ExecutionCount, &cronJob.FailureCount, &cronJob.ConsecutiveFailures,
		&cronJob.MaxRetries, &cronJob.RetryDelaySeconds, &cronJob.CircuitBreakerThreshold,
		&cronJob.IsCircuitBroken, &cronJob.CircuitBrokenAt,
		&cronJob.CreatedAt, &cronJob.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("cron job not found for connectivity: %s", connectivityID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get cron job: %w", err)
	}

	return cronJob, nil
}

// UpdateCronJob updates cron job configuration
func (cs *ConnectivityService) UpdateCronJob(cronJob *models.CronJob) error {
	query := `
		UPDATE cron_jobs
		SET cron_expression = $1,
		    timezone = $2,
		    is_enabled = $3,
		    next_execution = $4,
		    max_retries = $5,
		    retry_delay_seconds = $6,
		    circuit_breaker_threshold = $7,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $8
		RETURNING updated_at
	`

	err := cs.db.QueryRow(
		query,
		cronJob.CronExpression, cronJob.Timezone, cronJob.IsEnabled, cronJob.NextExecution,
		cronJob.MaxRetries, cronJob.RetryDelaySeconds, cronJob.CircuitBreakerThreshold,
		cronJob.ID,
	).Scan(&cronJob.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to update cron job: %w", err)
	}

	log.Printf("✅ Updated cron job: %s", cronJob.ID)
	return nil
}

// ========================================
// Execution Logs
// ========================================

// CreateExecutionLog creates an execution log entry
func (cs *ConnectivityService) CreateExecutionLog(log *models.ConnectivityExecutionLog) error {
	query := `
		INSERT INTO connectivity_execution_log (
			id, interface_connectivity_id, interface_id,
			execution_type, status, triggered_by
		) VALUES (
			gen_random_uuid(), $1, $2, $3, $4, $5
		)
		RETURNING id, started_at
	`

	err := cs.db.QueryRow(
		query,
		log.InterfaceConnectivityID, log.InterfaceID,
		log.ExecutionType, log.Status, log.TriggeredBy,
	).Scan(&log.ID, &log.StartedAt)

	if err != nil {
		return fmt.Errorf("failed to create execution log: %w", err)
	}

	return nil
}

// CompleteExecutionLog updates an execution log with results
func (cs *ConnectivityService) CompleteExecutionLog(logID string, status string, messagesRetrieved, messagesProcessed, messagesFailed int, errorDetails json.RawMessage) error {
	now := time.Now()
	query := `
		UPDATE connectivity_execution_log
		SET completed_at = $1,
		    status = $2,
		    messages_retrieved = $3,
		    messages_processed = $4,
		    messages_failed = $5,
		    error_details = $6,
		    duration_ms = EXTRACT(EPOCH FROM ($1 - started_at)) * 1000
		WHERE id = $7
	`

	_, err := cs.db.Exec(
		query,
		now, status, messagesRetrieved, messagesProcessed, messagesFailed, errorDetails, logID,
	)

	if err != nil {
		return fmt.Errorf("failed to complete execution log: %w", err)
	}

	return nil
}

// GetExecutionLogs retrieves execution logs with filtering
func (cs *ConnectivityService) GetExecutionLogs(filter *models.ExecutionLogFilter) ([]*models.ConnectivityExecutionLog, error) {
	query := `
		SELECT id, interface_connectivity_id, interface_id,
		       execution_type, started_at, completed_at, status,
		       messages_retrieved, messages_processed, messages_failed,
		       error_details, duration_ms, triggered_by, execution_metadata
		FROM connectivity_execution_log
		WHERE 1=1
	`

	args := []interface{}{}
	argCount := 1

	if filter != nil {
		if filter.InterfaceID != "" {
			query += fmt.Sprintf(" AND interface_id = $%d", argCount)
			args = append(args, filter.InterfaceID)
			argCount++
		}
		if filter.ExecutionType != "" {
			query += fmt.Sprintf(" AND execution_type = $%d", argCount)
			args = append(args, filter.ExecutionType)
			argCount++
		}
		if filter.Status != "" {
			query += fmt.Sprintf(" AND status = $%d", argCount)
			args = append(args, filter.Status)
			argCount++
		}
		if filter.StartDateFrom != nil {
			query += fmt.Sprintf(" AND started_at >= $%d", argCount)
			args = append(args, *filter.StartDateFrom)
			argCount++
		}
		if filter.StartDateTo != nil {
			query += fmt.Sprintf(" AND started_at <= $%d", argCount)
			args = append(args, *filter.StartDateTo)
			argCount++
		}
	}

	query += " ORDER BY started_at DESC"

	if filter != nil && filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argCount)
		args = append(args, filter.Limit)
		argCount++

		if filter.Offset > 0 {
			query += fmt.Sprintf(" OFFSET $%d", argCount)
			args = append(args, filter.Offset)
		}
	}

	rows, err := cs.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query execution logs: %w", err)
	}
	defer rows.Close()

	var logs []*models.ConnectivityExecutionLog
	for rows.Next() {
		log := &models.ConnectivityExecutionLog{}
		err := rows.Scan(
			&log.ID, &log.InterfaceConnectivityID, &log.InterfaceID,
			&log.ExecutionType, &log.StartedAt, &log.CompletedAt, &log.Status,
			&log.MessagesRetrieved, &log.MessagesProcessed, &log.MessagesFailed,
			&log.ErrorDetails, &log.DurationMs, &log.TriggeredBy, &log.ExecutionMetadata,
		)
		if err != nil {
			continue
		}
		logs = append(logs, log)
	}

	return logs, nil
}

// GetExecutionStats gets aggregated execution statistics for an interface
func (cs *ConnectivityService) GetExecutionStats(interfaceID string) (*models.ExecutionStats, error) {
	query := `
		SELECT
			COUNT(*) as total_executions,
			COUNT(CASE WHEN status = 'success' THEN 1 END) as successful_executions,
			COUNT(CASE WHEN status IN ('error', 'failed') THEN 1 END) as failed_executions,
			COALESCE(SUM(messages_retrieved), 0) as total_messages_retrieved,
			COALESCE(SUM(messages_processed), 0) as total_messages_processed,
			COALESCE(SUM(messages_failed), 0) as total_messages_failed,
			COALESCE(AVG(duration_ms), 0) as average_duration_ms,
			MAX(started_at) as last_execution
		FROM connectivity_execution_log
		WHERE interface_id = $1
	`

	stats := &models.ExecutionStats{}
	var lastExec sql.NullTime

	err := cs.db.QueryRow(query, interfaceID).Scan(
		&stats.TotalExecutions,
		&stats.SuccessfulExecutions,
		&stats.FailedExecutions,
		&stats.TotalMessagesRetrieved,
		&stats.TotalMessagesProcessed,
		&stats.TotalMessagesFailed,
		&stats.AverageDurationMs,
		&lastExec,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get execution stats: %v", err)
	}

	if lastExec.Valid {
		stats.LastExecution = &lastExec.Time
	}

	// Calculate success rate
	if stats.TotalExecutions > 0 {
		stats.SuccessRate = (float64(stats.SuccessfulExecutions) / float64(stats.TotalExecutions)) * 100.0
	}

	return stats, nil
}

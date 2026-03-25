package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// ErrSystemRule is returned when a caller attempts to delete an OOB system rule.
var ErrSystemRule = errors.New("system rules cannot be deleted")

// ─── Domain models ─────────────────────────────────────────────────────────────

// AlertRule is a configurable threshold rule that triggers monitoring_alerts.
type AlertRule struct {
	ID                     string   `json:"id"`
	Name                   string   `json:"name"`
	Description            string   `json:"description"`
	InterfaceID            *string  `json:"interface_id"`            // nil = global
	InterfaceName          string   `json:"interface_name,omitempty"` // enriched at query time
	RuleType               string   `json:"rule_type"`
	Condition              string   `json:"condition"`
	ThresholdWarning       *float64 `json:"threshold_warning"`
	ThresholdCritical      *float64 `json:"threshold_critical"`
	EvaluationWindowMins   int      `json:"evaluation_window_minutes"`
	CooldownMinutes        int      `json:"cooldown_minutes"`
	IsEnabled              bool     `json:"is_enabled"`
	IsSystem               bool     `json:"is_system"`
	ChannelCount           int      `json:"channel_count,omitempty"` // enriched at query time
	CreatedByUserID        *string  `json:"created_by_user_id"`
	CreatedAt              string   `json:"created_at"`
	UpdatedAt              string   `json:"updated_at"`
}

// AlertChannel is a notification destination (email, webhook, or Slack).
type AlertChannel struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	ChannelType     string          `json:"channel_type"` // email | webhook | slack
	Config          json.RawMessage `json:"config"`
	IsEnabled       bool            `json:"is_enabled"`
	TestStatus      string          `json:"test_status"`
	TestError       string          `json:"test_error,omitempty"`
	LastTestedAt    *string         `json:"last_tested_at"`
	CreatedByUserID *string         `json:"created_by_user_id"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
}

// AlertRuleChannel is a single rule→channel assignment with an optional severity filter.
type AlertRuleChannel struct {
	RuleID         string   `json:"rule_id"`
	ChannelID      string   `json:"channel_id"`
	ChannelName    string   `json:"channel_name,omitempty"`
	ChannelType    string   `json:"channel_type,omitempty"`
	SeverityFilter []string `json:"severity_filter"` // empty = all severities
}

// AlertSilence suppresses alert firing during maintenance windows.
type AlertSilence struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Reason          string  `json:"reason"`
	InterfaceID     *string `json:"interface_id"`
	InterfaceName   string  `json:"interface_name,omitempty"`
	AlertRuleID     *string `json:"alert_rule_id"`
	AlertRuleName   string  `json:"alert_rule_name,omitempty"`
	StartsAt        string  `json:"starts_at"`
	EndsAt          string  `json:"ends_at"`
	IsActive        bool    `json:"is_active"`
	CreatedByUserID *string `json:"created_by_user_id"`
	CreatedAt       string  `json:"created_at"`
}

// FiredAlert extends MonitoringAlert with the rule that triggered it.
type FiredAlert struct {
	ID             string  `json:"id"`
	InterfaceID    *string `json:"interface_id"`
	InterfaceName  string  `json:"interface_name"`
	AlertType      string  `json:"alert_type"`
	Severity       string  `json:"severity"`
	Message        string  `json:"message"`
	Acknowledged   bool    `json:"acknowledged"`
	AcknowledgedBy string  `json:"acknowledged_by,omitempty"`
	AcknowledgedAt *string `json:"acknowledged_at,omitempty"`
	AutoResolved   bool    `json:"auto_resolved"`
	ResolvedAt     *string `json:"resolved_at,omitempty"`
	CreatedAt      string  `json:"created_at"`
	ElapsedSeconds int64   `json:"elapsed_seconds"`
}

// ─── Service ───────────────────────────────────────────────────────────────────

// AlertRuleService manages alert rules, channels, silences, and fired-alert queries.
type AlertRuleService struct {
	db              *sql.DB
	notificationSvc *AlertNotificationService
}

// NewAlertRuleService creates a new AlertRuleService.
func NewAlertRuleService(db *sql.DB) *AlertRuleService {
	return &AlertRuleService{
		db:              db,
		notificationSvc: NewAlertNotificationService(db),
	}
}

// ─── Alert Rules CRUD ──────────────────────────────────────────────────────────

// ListRules returns all rules. Pass empty string for interfaceID to get all;
// pass a UUID to get only the global rules plus rules for that interface.
func (s *AlertRuleService) ListRules(ctx context.Context, interfaceID string) ([]*AlertRule, error) {
	query := `
		SELECT ar.id, ar.name, COALESCE(ar.description,''), ar.interface_id,
		       COALESCE(i.name,''), ar.rule_type, ar.condition,
		       ar.threshold_warning, ar.threshold_critical,
		       ar.evaluation_window_minutes, ar.cooldown_minutes,
		       ar.is_enabled, ar.is_system,
		       ar.created_by_user_id,
		       to_char(ar.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       to_char(ar.updated_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       COUNT(arc.channel_id)::int
		FROM alert_rules ar
		LEFT JOIN interfaces i ON i.id = ar.interface_id
		LEFT JOIN alert_rule_channels arc ON arc.rule_id = ar.id`

	var args []interface{}
	if interfaceID != "" {
		query += ` WHERE ar.interface_id IS NULL OR ar.interface_id = $1`
		args = append(args, interfaceID)
	}
	query += ` GROUP BY ar.id, i.name ORDER BY ar.is_system DESC, ar.created_at ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		if isUndefinedTable(err) {
			return []*AlertRule{}, nil
		}
		return nil, fmt.Errorf("list alert rules: %w", err)
	}
	defer rows.Close()

	var rules []*AlertRule
	for rows.Next() {
		r := &AlertRule{}
		if err := rows.Scan(
			&r.ID, &r.Name, &r.Description, &r.InterfaceID,
			&r.InterfaceName, &r.RuleType, &r.Condition,
			&r.ThresholdWarning, &r.ThresholdCritical,
			&r.EvaluationWindowMins, &r.CooldownMinutes,
			&r.IsEnabled, &r.IsSystem,
			&r.CreatedByUserID,
			&r.CreatedAt, &r.UpdatedAt,
			&r.ChannelCount,
		); err != nil {
			return nil, fmt.Errorf("scan alert rule: %w", err)
		}
		rules = append(rules, r)
	}
	return rules, nil
}

// GetRule returns a single rule by ID.
func (s *AlertRuleService) GetRule(ctx context.Context, id string) (*AlertRule, error) {
	r := &AlertRule{}
	err := s.db.QueryRowContext(ctx, `
		SELECT ar.id, ar.name, COALESCE(ar.description,''), ar.interface_id,
		       COALESCE(i.name,''), ar.rule_type, ar.condition,
		       ar.threshold_warning, ar.threshold_critical,
		       ar.evaluation_window_minutes, ar.cooldown_minutes,
		       ar.is_enabled, ar.is_system,
		       ar.created_by_user_id,
		       to_char(ar.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       to_char(ar.updated_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM alert_rules ar
		LEFT JOIN interfaces i ON i.id = ar.interface_id
		WHERE ar.id = $1`, id,
	).Scan(
		&r.ID, &r.Name, &r.Description, &r.InterfaceID,
		&r.InterfaceName, &r.RuleType, &r.Condition,
		&r.ThresholdWarning, &r.ThresholdCritical,
		&r.EvaluationWindowMins, &r.CooldownMinutes,
		&r.IsEnabled, &r.IsSystem,
		&r.CreatedByUserID,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return r, err
}

// CreateRule inserts a new alert rule.
func (s *AlertRuleService) CreateRule(ctx context.Context, r *AlertRule) error {
	if len(r.Name) > 255 {
		return fmt.Errorf("name exceeds maximum length of 255 characters")
	}
	return s.db.QueryRowContext(ctx, `
		INSERT INTO alert_rules
		    (name, description, interface_id, rule_type, condition,
		     threshold_warning, threshold_critical,
		     evaluation_window_minutes, cooldown_minutes, is_enabled, created_by_user_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id, to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		          to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')`,
		r.Name, r.Description, r.InterfaceID, r.RuleType, r.Condition,
		r.ThresholdWarning, r.ThresholdCritical,
		r.EvaluationWindowMins, r.CooldownMinutes, r.IsEnabled,
		r.CreatedByUserID,
	).Scan(&r.ID, &r.CreatedAt, &r.UpdatedAt)
}

// UpdateRule updates a rule's mutable fields. Returns sql.ErrNoRows if id not found.
func (s *AlertRuleService) UpdateRule(ctx context.Context, r *AlertRule) error {
	if len(r.Name) > 255 {
		return fmt.Errorf("name exceeds maximum length of 255 characters")
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE alert_rules SET
		    name=$1, description=$2, rule_type=$3, condition=$4,
		    threshold_warning=$5, threshold_critical=$6,
		    evaluation_window_minutes=$7, cooldown_minutes=$8,
		    is_enabled=$9, updated_at=NOW()
		WHERE id=$10`,
		r.Name, r.Description, r.RuleType, r.Condition,
		r.ThresholdWarning, r.ThresholdCritical,
		r.EvaluationWindowMins, r.CooldownMinutes,
		r.IsEnabled, r.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteRule removes a rule. Returns ErrSystemRule for OOB rules.
func (s *AlertRuleService) DeleteRule(ctx context.Context, id string) error {
	var isSystem bool
	if err := s.db.QueryRowContext(ctx,
		`SELECT is_system FROM alert_rules WHERE id=$1`, id,
	).Scan(&isSystem); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	if isSystem {
		return ErrSystemRule
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM alert_rules WHERE id=$1`, id)
	return err
}

// ─── Alert Channels CRUD ───────────────────────────────────────────────────────

// ListChannels returns all configured notification channels.
func (s *AlertRuleService) ListChannels(ctx context.Context) ([]*AlertChannel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, channel_type, config, is_enabled,
		       COALESCE(test_status,'untested'), COALESCE(test_error,''),
		       to_char(last_tested_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       created_by_user_id,
		       to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM alert_channels ORDER BY created_at ASC`)
	if err != nil {
		if isUndefinedTable(err) {
			return []*AlertChannel{}, nil
		}
		return nil, fmt.Errorf("list channels: %w", err)
	}
	defer rows.Close()

	var channels []*AlertChannel
	for rows.Next() {
		c := &AlertChannel{}
		var cfg []byte
		var lastTested sql.NullString
		if err := rows.Scan(
			&c.ID, &c.Name, &c.ChannelType, &cfg, &c.IsEnabled,
			&c.TestStatus, &c.TestError, &lastTested,
			&c.CreatedByUserID, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		c.Config = json.RawMessage(cfg)
		if lastTested.Valid {
			c.LastTestedAt = &lastTested.String
		}
		channels = append(channels, c)
	}
	return channels, nil
}

// GetChannel returns a single channel by ID.
func (s *AlertRuleService) GetChannel(ctx context.Context, id string) (*AlertChannel, error) {
	c := &AlertChannel{}
	var cfg []byte
	var lastTested sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, channel_type, config, is_enabled,
		       COALESCE(test_status,'untested'), COALESCE(test_error,''),
		       to_char(last_tested_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       created_by_user_id,
		       to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM alert_channels WHERE id=$1`, id,
	).Scan(
		&c.ID, &c.Name, &c.ChannelType, &cfg, &c.IsEnabled,
		&c.TestStatus, &c.TestError, &lastTested,
		&c.CreatedByUserID, &c.CreatedAt, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Config = json.RawMessage(cfg)
	if lastTested.Valid {
		c.LastTestedAt = &lastTested.String
	}
	return c, nil
}

var validChannelTypes = map[string]bool{"email": true, "webhook": true, "slack": true}

// CreateChannel inserts a new notification channel.
func (s *AlertRuleService) CreateChannel(ctx context.Context, c *AlertChannel) error {
	if !validChannelTypes[c.ChannelType] {
		return fmt.Errorf("invalid channel_type %q: must be one of email, webhook, slack", c.ChannelType)
	}
	cfg, _ := json.Marshal(c.Config)
	return s.db.QueryRowContext(ctx, `
		INSERT INTO alert_channels (name, channel_type, config, is_enabled, created_by_user_id)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id, to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		          to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')`,
		c.Name, c.ChannelType, cfg, c.IsEnabled, c.CreatedByUserID,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

// UpdateChannel updates a channel's config.
func (s *AlertRuleService) UpdateChannel(ctx context.Context, c *AlertChannel) error {
	cfg, _ := json.Marshal(c.Config)
	_, err := s.db.ExecContext(ctx, `
		UPDATE alert_channels SET
		    name=$1, channel_type=$2, config=$3, is_enabled=$4, updated_at=NOW()
		WHERE id=$5`,
		c.Name, c.ChannelType, cfg, c.IsEnabled, c.ID,
	)
	return err
}

// DeleteChannel removes a channel and all its rule assignments.
func (s *AlertRuleService) DeleteChannel(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM alert_channels WHERE id=$1`, id)
	return err
}

// TestChannel sends a test payload and updates test_status on the channel row.
func (s *AlertRuleService) TestChannel(ctx context.Context, id string) error {
	ch, err := s.GetChannel(ctx, id)
	if err != nil || ch == nil {
		return fmt.Errorf("channel not found")
	}

	testAlert := &FiredAlert{
		ID:             "test-00000000-0000-0000-0000-000000000000",
		InterfaceName:  "Test Interface",
		AlertType:      "TEST",
		Severity:       "info",
		Message:        "This is a test notification from ezHealthKonnect Alert Management.",
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		ElapsedSeconds: 0,
	}

	dispatchErr := s.notificationSvc.DispatchToChannel(ctx, ch, testAlert)

	status := "success"
	errMsg := ""
	if dispatchErr != nil {
		status = "failure"
		errMsg = dispatchErr.Error()
	}

	_, _ = s.db.ExecContext(ctx, `
		UPDATE alert_channels SET
		    test_status=$1, test_error=$2, last_tested_at=NOW(), updated_at=NOW()
		WHERE id=$3`,
		status, errMsg, id,
	)
	return dispatchErr
}

// ─── Rule ↔ Channel Assignments ────────────────────────────────────────────────

// GetRuleChannels returns all channels assigned to a rule.
func (s *AlertRuleService) GetRuleChannels(ctx context.Context, ruleID string) ([]*AlertRuleChannel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT arc.rule_id, arc.channel_id, ac.name, ac.channel_type, arc.severity_filter
		FROM alert_rule_channels arc
		JOIN alert_channels ac ON ac.id = arc.channel_id
		WHERE arc.rule_id = $1`, ruleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []*AlertRuleChannel
	for rows.Next() {
		a := &AlertRuleChannel{}
		if err := rows.Scan(&a.RuleID, &a.ChannelID, &a.ChannelName, &a.ChannelType, pq.Array(&a.SeverityFilter)); err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	return assignments, nil
}

// AssignChannels replaces all channel assignments for a rule.
func (s *AlertRuleService) AssignChannels(ctx context.Context, ruleID string, assignments []AlertRuleChannel) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM alert_rule_channels WHERE rule_id=$1`, ruleID,
	); err != nil {
		return err
	}

	for _, a := range assignments {
		filter := a.SeverityFilter
		if filter == nil {
			filter = []string{}
		}
		filterStr := "{" + joinStrings(filter) + "}"
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO alert_rule_channels (rule_id, channel_id, severity_filter)
			 VALUES ($1,$2,$3::text[])`,
			ruleID, a.ChannelID, filterStr,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ─── Silences CRUD ─────────────────────────────────────────────────────────────

// ListSilences returns all silences. Pass activeOnly=true for only current ones.
func (s *AlertRuleService) ListSilences(ctx context.Context, activeOnly bool) ([]*AlertSilence, error) {
	query := `
		SELECT sil.id, sil.name, COALESCE(sil.reason,''),
		       sil.interface_id, COALESCE(i.name,''),
		       sil.alert_rule_id, COALESCE(ar.name,''),
		       to_char(sil.starts_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       to_char(sil.ends_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       (NOW() BETWEEN sil.starts_at AND sil.ends_at) AS is_active,
		       sil.created_by_user_id,
		       to_char(sil.created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM alert_silences sil
		LEFT JOIN interfaces i ON i.id = sil.interface_id
		LEFT JOIN alert_rules ar ON ar.id = sil.alert_rule_id`

	if activeOnly {
		query += ` WHERE NOW() BETWEEN sil.starts_at AND sil.ends_at`
	}
	query += ` ORDER BY sil.starts_at DESC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		if isUndefinedTable(err) {
			return []*AlertSilence{}, nil
		}
		return nil, fmt.Errorf("list silences: %w", err)
	}
	defer rows.Close()

	var silences []*AlertSilence
	for rows.Next() {
		sil := &AlertSilence{}
		if err := rows.Scan(
			&sil.ID, &sil.Name, &sil.Reason,
			&sil.InterfaceID, &sil.InterfaceName,
			&sil.AlertRuleID, &sil.AlertRuleName,
			&sil.StartsAt, &sil.EndsAt, &sil.IsActive,
			&sil.CreatedByUserID, &sil.CreatedAt,
		); err != nil {
			return nil, err
		}
		silences = append(silences, sil)
	}
	return silences, nil
}

// CreateSilence inserts a new maintenance window silence.
func (s *AlertRuleService) CreateSilence(ctx context.Context, sil *AlertSilence) error {
	return s.db.QueryRowContext(ctx, `
		INSERT INTO alert_silences
		    (name, reason, interface_id, alert_rule_id, starts_at, ends_at, created_by_user_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')`,
		sil.Name, sil.Reason, sil.InterfaceID, sil.AlertRuleID,
		sil.StartsAt, sil.EndsAt, sil.CreatedByUserID,
	).Scan(&sil.ID, &sil.CreatedAt)
}

// DeleteSilence removes a silence. Returns sql.ErrNoRows if not found.
func (s *AlertRuleService) DeleteSilence(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM alert_silences WHERE id=$1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ─── Fired Alerts (monitoring_alerts table) ────────────────────────────────────

// ListFiredAlerts returns active (unacknowledged, unresolved) alerts.
// Pass interfaceID="" for all; pass countOnly=true to get just the count.
func (s *AlertRuleService) ListFiredAlerts(ctx context.Context, interfaceID string, countOnly bool) ([]*FiredAlert, int, error) {
	baseWhere := `WHERE ma.acknowledged = FALSE AND ma.auto_resolved = FALSE`
	var args []interface{}
	if interfaceID != "" {
		baseWhere += ` AND ma.interface_id = $1`
		args = append(args, interfaceID)
	}

	if countOnly {
		var count int
		q := fmt.Sprintf(`SELECT COUNT(*) FROM monitoring_alerts ma %s`, baseWhere)
		if err := s.db.QueryRowContext(ctx, q, args...).Scan(&count); err != nil {
			if isUndefinedTable(err) {
				return nil, 0, nil // table not yet created — return 0 gracefully
			}
			return nil, 0, err
		}
		return nil, count, nil
	}

	q := fmt.Sprintf(`
		SELECT ma.id, ma.interface_id, COALESCE(i.name,'System'), ma.alert_type,
		       ma.severity, ma.message, ma.acknowledged,
		       COALESCE(ma.acknowledged_by,''),
		       to_char(ma.acknowledged_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       ma.auto_resolved,
		       to_char(ma.resolved_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       to_char(ma.created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       EXTRACT(EPOCH FROM (NOW() - ma.created_at))::bigint
		FROM monitoring_alerts ma
		LEFT JOIN interfaces i ON i.id = ma.interface_id
		%s
		ORDER BY
		    CASE ma.severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END,
		    ma.created_at DESC
		LIMIT 200`, baseWhere)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		if isUndefinedTable(err) {
			return []*FiredAlert{}, 0, nil // table not yet created — return empty gracefully
		}
		return nil, 0, err
	}
	defer rows.Close()

	var alerts []*FiredAlert
	for rows.Next() {
		a := &FiredAlert{}
		var ackedAt, resolvedAt sql.NullString
		if err := rows.Scan(
			&a.ID, &a.InterfaceID, &a.InterfaceName, &a.AlertType,
			&a.Severity, &a.Message, &a.Acknowledged,
			&a.AcknowledgedBy, &ackedAt,
			&a.AutoResolved, &resolvedAt,
			&a.CreatedAt, &a.ElapsedSeconds,
		); err != nil {
			return nil, 0, err
		}
		if ackedAt.Valid {
			a.AcknowledgedAt = &ackedAt.String
		}
		if resolvedAt.Valid {
			a.ResolvedAt = &resolvedAt.String
		}
		alerts = append(alerts, a)
	}
	return alerts, len(alerts), nil
}

// isUndefinedTable returns true when a PostgreSQL error indicates the queried
// relation (table) does not exist yet (SQLSTATE 42P01).  Used to return
// graceful empty results before migrations are applied.
func isUndefinedTable(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "42P01"
	}
	return false
}

// AcknowledgeFiredAlert marks a monitoring_alert as acknowledged.
func (s *AlertRuleService) AcknowledgeFiredAlert(ctx context.Context, id, acknowledgedBy string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE monitoring_alerts SET
		    acknowledged=TRUE, acknowledged_by=$1, acknowledged_at=NOW(), updated_at=NOW()
		WHERE id=$2`, acknowledgedBy, id)
	return err
}

// ResolveFiredAlert marks a monitoring_alert as manually resolved.
func (s *AlertRuleService) ResolveFiredAlert(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE monitoring_alerts SET
		    auto_resolved=TRUE, resolved_at=NOW(), updated_at=NOW()
		WHERE id=$1`, id)
	return err
}

// AcknowledgeAll acknowledges all unacknowledged alerts (optionally scoped to one interface).
func (s *AlertRuleService) AcknowledgeAll(ctx context.Context, interfaceID, acknowledgedBy string) (int64, error) {
	var q string
	var args []interface{}
	if interfaceID != "" {
		q = `UPDATE monitoring_alerts SET acknowledged=TRUE, acknowledged_by=$1, acknowledged_at=NOW(), updated_at=NOW()
		     WHERE acknowledged=FALSE AND auto_resolved=FALSE AND interface_id=$2`
		args = []interface{}{acknowledgedBy, interfaceID}
	} else {
		q = `UPDATE monitoring_alerts SET acknowledged=TRUE, acknowledged_by=$1, acknowledged_at=NOW(), updated_at=NOW()
		     WHERE acknowledged=FALSE AND auto_resolved=FALSE`
		args = []interface{}{acknowledgedBy}
	}
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ListAlertHistory returns all monitoring_alerts (including resolved) for the
// History tab, newest first, capped at 500 rows.
func (s *AlertRuleService) ListAlertHistory(ctx context.Context) ([]*FiredAlert, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ma.id, ma.interface_id, COALESCE(i.name,'System'), ma.alert_type,
		       ma.severity, ma.message, ma.acknowledged,
		       COALESCE(ma.acknowledged_by,''),
		       to_char(ma.acknowledged_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       ma.auto_resolved,
		       to_char(ma.resolved_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       to_char(ma.created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       EXTRACT(EPOCH FROM (NOW() - ma.created_at))::bigint
		FROM monitoring_alerts ma
		LEFT JOIN interfaces i ON i.id = ma.interface_id
		ORDER BY ma.created_at DESC
		LIMIT 500`)
	if err != nil {
		if isUndefinedTable(err) {
			return []*FiredAlert{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	var alerts []*FiredAlert
	for rows.Next() {
		a := &FiredAlert{}
		var ackedAt, resolvedAt sql.NullString
		if err := rows.Scan(
			&a.ID, &a.InterfaceID, &a.InterfaceName, &a.AlertType,
			&a.Severity, &a.Message, &a.Acknowledged,
			&a.AcknowledgedBy, &ackedAt,
			&a.AutoResolved, &resolvedAt,
			&a.CreatedAt, &a.ElapsedSeconds,
		); err != nil {
			return nil, err
		}
		if ackedAt.Valid {
			a.AcknowledgedAt = &ackedAt.String
		}
		if resolvedAt.Valid {
			a.ResolvedAt = &resolvedAt.String
		}
		alerts = append(alerts, a)
	}
	return alerts, nil
}

// ─── Notification Dispatch ─────────────────────────────────────────────────────

// DispatchAlert sends notifications for a newly fired alert to all channels
// configured for the matching rule. Dispatch is best-effort; failures are logged
// but do not block the calling evaluation loop.
func (s *AlertRuleService) DispatchAlert(ctx context.Context, alert *FiredAlert, ruleID string) {
	if s.notificationSvc == nil {
		return
	}
	channels, err := s.GetRuleChannels(ctx, ruleID)
	if err != nil || len(channels) == 0 {
		return
	}

	for _, assignment := range channels {
		// Apply severity filter
		if len(assignment.SeverityFilter) > 0 && !containsString(assignment.SeverityFilter, alert.Severity) {
			continue
		}

		ch, err := s.GetChannel(ctx, assignment.ChannelID)
		if err != nil || ch == nil || !ch.IsEnabled {
			continue
		}

		go func(channel *AlertChannel) {
			dispatchCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = s.notificationSvc.DispatchToChannel(dispatchCtx, channel, alert)
		}(ch)
	}
}

// ─── IsSilenced ────────────────────────────────────────────────────────────────

// IsSilenced checks whether an active silence suppresses the given interface + rule combination.
func (s *AlertRuleService) IsSilenced(ctx context.Context, interfaceID *string, ruleID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM alert_silences
		WHERE NOW() BETWEEN starts_at AND ends_at
		  AND (interface_id IS NULL OR interface_id = $1)
		  AND (alert_rule_id IS NULL OR alert_rule_id = $2)`,
		interfaceID, ruleID,
	).Scan(&count)
	return count > 0, err
}

// ─── Helpers ───────────────────────────────────────────────────────────────────

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func joinStrings(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ","
		}
		result += `"` + s + `"`
	}
	return result
}

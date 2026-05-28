// services/ai/conversation_memory.go
// Persists multi-turn conversation history in ai_conversations so the LLM
// can answer follow-up questions with full context from earlier in the session.
package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ConversationMemory reads and writes conversation history using ai_conversations.
type ConversationMemory struct {
	db *sql.DB
}

// NewConversationMemory creates a new ConversationMemory.
func NewConversationMemory(db *sql.DB) *ConversationMemory {
	return &ConversationMemory{db: db}
}

// SaveTurn persists a single conversation turn (user or assistant) to the DB.
// userID may be empty (anonymous / system).
func (m *ConversationMemory) SaveTurn(ctx context.Context, sessionID, userID, role, content string, metadata map[string]interface{}) error {
	if m.db == nil {
		return nil
	}
	metaJSON, _ := json.Marshal(metadata)
	var uid interface{}
	if userID != "" {
		uid = userID
	}
	_, err := m.db.ExecContext(ctx, `
		INSERT INTO ai_conversations (id, session_id, user_id, role, content, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, uuid.New().String(), sessionID, uid, role, content, metaJSON, time.Now())
	return err
}

// LoadHistory retrieves the last `limit` turns for a session, oldest-first
// (so they read naturally as a conversation when injected into a prompt).
func (m *ConversationMemory) LoadHistory(ctx context.Context, sessionID string, limit int) ([]ConversationTurn, error) {
	if m.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	// Inner query: newest N; outer query: re-order oldest-first
	rows, err := m.db.QueryContext(ctx, `
		SELECT role, content FROM (
			SELECT role, content, created_at
			FROM   ai_conversations
			WHERE  session_id = $1
			ORDER  BY created_at DESC
			LIMIT  $2
		) recent
		ORDER BY created_at ASC
	`, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("load history for session %s: %w", sessionID, err)
	}
	defer rows.Close()

	var turns []ConversationTurn
	for rows.Next() {
		var t ConversationTurn
		if err := rows.Scan(&t.Role, &t.Content); err != nil {
			continue
		}
		turns = append(turns, t)
	}
	return turns, nil
}

// FormatHistory formats stored turns as a readable markdown block for prompt injection.
func (m *ConversationMemory) FormatHistory(turns []ConversationTurn) string {
	if len(turns) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("### Conversation History\n\n")
	for _, t := range turns {
		role := t.Role
		if len(role) > 0 {
			role = strings.ToUpper(role[:1]) + role[1:]
		}
		// Truncate turns to keep prompt within CPU inference budget (~6 input tokens/sec on CPU).
		content := t.Content
		if len(content) > 400 {
			content = content[:400] + "…"
		}
		sb.WriteString(fmt.Sprintf("**%s**: %s\n\n", role, content))
	}
	sb.WriteString("---\n\n")
	return sb.String()
}

// HistoryMessage is a conversation turn with timestamp — returned by the history endpoint.
type HistoryMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// LoadHistoryWithTimestamps returns the last `limit` turns for a session with timestamps.
// userID must match the turn's stored user_id — prevents cross-user IDOR.
// Returned oldest-first so the UI can render them in order.
func (m *ConversationMemory) LoadHistoryWithTimestamps(ctx context.Context, sessionID, userID string, limit int) ([]HistoryMessage, error) {
	if m.db == nil {
		return nil, nil
	}
	if userID == "" {
		return nil, fmt.Errorf("user_id required to load conversation history")
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := m.db.QueryContext(ctx, `
		SELECT role, content, created_at FROM (
			SELECT role, content, created_at
			FROM   ai_conversations
			WHERE  session_id = $1 AND user_id = $3
			ORDER  BY created_at DESC
			LIMIT  $2
		) recent
		ORDER BY created_at ASC
	`, sessionID, limit, userID)
	if err != nil {
		return nil, fmt.Errorf("load history for session %s: %w", sessionID, err)
	}
	defer rows.Close()

	var msgs []HistoryMessage
	for rows.Next() {
		var h HistoryMessage
		if err := rows.Scan(&h.Role, &h.Content, &h.CreatedAt); err != nil {
			continue
		}
		msgs = append(msgs, h)
	}
	return msgs, nil
}

// PruneOldSessions deletes conversation history older than `days` days.
// Safe to call periodically from a maintenance goroutine.
func (m *ConversationMemory) PruneOldSessions(ctx context.Context, days int) (int64, error) {
	result, err := m.db.ExecContext(ctx,
		`DELETE FROM ai_conversations WHERE created_at < NOW() - INTERVAL '1 day' * $1`,
		days)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

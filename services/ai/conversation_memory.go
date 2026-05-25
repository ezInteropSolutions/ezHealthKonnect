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
		// Truncate very long turns to keep prompt size manageable
		content := t.Content
		if len(content) > 800 {
			content = content[:800] + "…"
		}
		sb.WriteString(fmt.Sprintf("**%s**: %s\n\n", role, content))
	}
	sb.WriteString("---\n\n")
	return sb.String()
}

// PruneOldSessions deletes conversation history older than `days` days.
// Safe to call periodically from a maintenance goroutine.
func (m *ConversationMemory) PruneOldSessions(ctx context.Context, days int) (int64, error) {
	result, err := m.db.ExecContext(ctx,
		`DELETE FROM ai_conversations WHERE created_at < NOW() - ($1 || ' days')::INTERVAL`,
		days)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

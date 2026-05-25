package models

import (
	"encoding/json"
	"time"
)

// GitRepository represents a connected remote git repository.
// Sensitive fields (credentials) are never serialized to API responses.
type GitRepository struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	RemoteURL       string          `json:"remote_url"`
	Branch          string          `json:"branch"`
	LocalPath       string          `json:"local_path"` // resolved at runtime; never empty after load
	AuthType        string          `json:"auth_type"`  // "token" | "basic"
	Credentials     json.RawMessage `json:"-"`          // encrypted blob — never sent to client
	AutoPush        bool            `json:"auto_push"`
	IsActive        bool            `json:"is_active"`
	LastSyncAt      *time.Time      `json:"last_sync_at"`
	LastSyncStatus  string          `json:"last_sync_status"`
	LastSyncError   string          `json:"last_sync_error"`
	CreatedByUserID *string         `json:"created_by_user_id"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// GitCredentials is the plaintext shape stored inside the encrypted JSONB blob.
// The "token" key matches sensitiveKeySubstrings → auto-masked by CredentialStore on read.
type GitCredentials struct {
	Username string `json:"username"`
	Token    string `json:"token"`
}

// GitSyncLogEntry records one git operation in the audit trail.
type GitSyncLogEntry struct {
	ID            string          `json:"id"`
	RepoID        string          `json:"repo_id"`
	Operation     string          `json:"operation"`
	InterfaceID   *string         `json:"interface_id"`
	Branch        string          `json:"branch"`
	CommitSHA     string          `json:"commit_sha"`
	CommitMessage string          `json:"commit_message"`
	FilesChanged  json.RawMessage `json:"files_changed"`
	Status        string          `json:"status"`
	ErrorMessage  string          `json:"error_message"`
	DurationMs    int             `json:"duration_ms"`
	TriggeredBy   *string         `json:"triggered_by"`
	CreatedAt     time.Time       `json:"created_at"`
}

// GitStatusFile describes one file in the git working-tree status.
type GitStatusFile struct {
	Path     string `json:"path"`
	Staging  string `json:"staging"`  // " ", "A", "M", "D", "R", "?"
	Worktree string `json:"worktree"` // " ", "M", "D", "?"
}

// GitCommitSummary is one entry in the git log.
type GitCommitSummary struct {
	SHA       string    `json:"sha"`
	ShortSHA  string    `json:"short_sha"`
	Message   string    `json:"message"`
	Author    string    `json:"author"`
	Email     string    `json:"email"`
	Timestamp time.Time `json:"timestamp"`
}

// ── Bundle types ──────────────────────────────────────────────────────────────

// InterfaceBundle is the portable, git-friendly export for one interface.
// Sensitive values in connectivity config are replaced with {{ENV_VAR}} placeholders.
type InterfaceBundle struct {
	BundleVersion string                 `json:"bundle_version"` // "1.0"
	ExportedAt    time.Time              `json:"exported_at"`
	Metadata      InterfaceBundleMetadata `json:"metadata"`
	Interface     map[string]interface{} `json:"interface"`
	Pipelines     []PipelineBundle       `json:"pipelines"`
	Connectivity  map[string]interface{} `json:"connectivity"`
}

// PipelineBundle holds one pipeline and all its steps.
type PipelineBundle struct {
	Pipeline map[string]interface{}   `json:"pipeline"`
	Steps    []map[string]interface{} `json:"steps"`
}

// InterfaceBundleMetadata carries provenance information about the export.
type InterfaceBundleMetadata struct {
	InterfaceID   string `json:"interface_id"`
	InterfaceName string `json:"interface_name"`
	ExportedBy    string `json:"exported_by"`
	AppVersion    string `json:"app_version"`
}

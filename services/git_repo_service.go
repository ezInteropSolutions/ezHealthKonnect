package services

// ================================================================
// GitRepoService — CRUD for git_repositories + high-level workflows
//
// Orchestrates: CredentialStore (encryption), GitService (go-git),
// GitSerializer (bundle export), and audit logging to git_sync_log.
// ================================================================

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"ezhealthkonnect/models"
)

// GitRepoService manages git repository records and git workflows.
type GitRepoService struct {
	db         *sql.DB
	credStore  *CredentialStore
	gitSvc     *GitService
	serializer *GitSerializer
}

// NewGitRepoService creates a GitRepoService wired to all dependencies.
func NewGitRepoService(db *sql.DB, credStore *CredentialStore) *GitRepoService {
	return &GitRepoService{
		db:         db,
		credStore:  credStore,
		gitSvc:     NewGitService(),
		serializer: NewGitSerializer(db, credStore),
	}
}

// ── Request / Response types ──────────────────────────────────────────────────

// CreateGitRepoRequest is the request body for POST /api/git.
type CreateGitRepoRequest struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	RemoteURL   string              `json:"remote_url"`
	Branch      string              `json:"branch"`
	LocalPath   string              `json:"local_path"`
	AuthType    string              `json:"auth_type"`
	Credentials models.GitCredentials `json:"credentials"`
	AutoPush    bool                `json:"auto_push"`
}

// UpdateGitRepoRequest is the request body for PUT /api/git/:id.
// Pointer fields: nil means "no change".
type UpdateGitRepoRequest struct {
	Name        *string               `json:"name"`
	Description *string               `json:"description"`
	RemoteURL   *string               `json:"remote_url"`
	Branch      *string               `json:"branch"`
	LocalPath   *string               `json:"local_path"`
	AuthType    *string               `json:"auth_type"`
	Credentials *models.GitCredentials `json:"credentials"`
	AutoPush    *bool                 `json:"auto_push"`
	IsActive    *bool                 `json:"is_active"`
}

// CommitInterfaceRequest is the body for POST /api/git/:id/commit-interface.
type CommitInterfaceRequest struct {
	InterfaceID   string `json:"interface_id"`
	CommitMessage string `json:"commit_message"`
	AuthorName    string `json:"author_name"`
	AuthorEmail   string `json:"author_email"`
}

// ── CRUD ──────────────────────────────────────────────────────────────────────

// List returns active git repositories owned by the given user.
// userID scopes results so each user only sees their own repos.
func (s *GitRepoService) List(ctx context.Context, userID *string) ([]*models.GitRepository, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, remote_url, branch,
		       COALESCE(local_path,'') AS local_path, auth_type,
		       credentials, auto_push, is_active,
		       last_sync_at, COALESCE(last_sync_status,'') AS last_sync_status,
		       COALESCE(last_sync_error,'') AS last_sync_error,
		       created_by_user_id, created_at, updated_at
		FROM git_repositories
		WHERE is_active = true
		  AND ($1::uuid IS NULL OR created_by_user_id = $1::uuid)
		ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []*models.GitRepository
	for rows.Next() {
		r := &models.GitRepository{}
		if err := s.scanRepo(rows, r); err != nil {
			continue
		}
		r.LocalPath = s.gitSvc.ResolvedLocalPath(r)
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

// Get returns one repository by ID, scoped to the owning user.
// Pass userID=nil to bypass ownership check (internal calls like CommitInterface).
func (s *GitRepoService) Get(ctx context.Context, id string, userID *string) (*models.GitRepository, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, remote_url, branch,
		       COALESCE(local_path,'') AS local_path, auth_type,
		       credentials, auto_push, is_active,
		       last_sync_at, COALESCE(last_sync_status,'') AS last_sync_status,
		       COALESCE(last_sync_error,'') AS last_sync_error,
		       created_by_user_id, created_at, updated_at
		FROM git_repositories
		WHERE id = $1
		  AND ($2::uuid IS NULL OR created_by_user_id = $2::uuid)`, id, userID)

	r := &models.GitRepository{}
	var credsRaw []byte
	var lastSyncAt sql.NullTime
	var localPath sql.NullString
	var createdBy sql.NullString

	err := row.Scan(
		&r.ID, &r.Name, &r.Description, &r.RemoteURL, &r.Branch,
		&localPath, &r.AuthType,
		&credsRaw, &r.AutoPush, &r.IsActive,
		&lastSyncAt, &r.LastSyncStatus, &r.LastSyncError,
		&createdBy, &r.CreatedAt, &r.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Credentials = credsRaw
	if lastSyncAt.Valid {
		t := lastSyncAt.Time
		r.LastSyncAt = &t
	}
	if localPath.Valid {
		r.LocalPath = localPath.String
	}
	if createdBy.Valid {
		r.CreatedByUserID = &createdBy.String
	}
	r.LocalPath = s.gitSvc.ResolvedLocalPath(r)
	return r, nil
}

// Create inserts a new repository with encrypted credentials.
func (s *GitRepoService) Create(ctx context.Context, req CreateGitRepoRequest, createdBy *string) (*models.GitRepository, error) {
	if req.Branch == "" {
		req.Branch = "main"
	}
	if req.AuthType == "" {
		req.AuthType = "token"
	}

	credBytes, err := s.encryptCreds(req.Credentials)
	if err != nil {
		return nil, fmt.Errorf("encrypt credentials: %w", err)
	}

	var id string
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO git_repositories
		       (name, description, remote_url, branch, local_path,
		        auth_type, credentials, auto_push, created_by_user_id)
		VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9)
		RETURNING id`,
		req.Name, req.Description, req.RemoteURL, req.Branch, req.LocalPath,
		req.AuthType, credBytes, req.AutoPush, createdBy,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("insert git_repository: %w", err)
	}
	return s.Get(ctx, id, nil)
}

// Update patches an existing repository. Only non-nil fields are changed.
func (s *GitRepoService) Update(ctx context.Context, id string, req UpdateGitRepoRequest) (*models.GitRepository, error) {
	existing, err := s.Get(ctx, id, nil)
	if err != nil || existing == nil {
		return nil, fmt.Errorf("not found")
	}

	// Apply patches
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.RemoteURL != nil {
		existing.RemoteURL = *req.RemoteURL
	}
	if req.Branch != nil {
		existing.Branch = *req.Branch
	}
	if req.AuthType != nil {
		existing.AuthType = *req.AuthType
	}
	if req.AutoPush != nil {
		existing.AutoPush = *req.AutoPush
	}
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}

	credBytes := existing.Credentials // keep existing unless new creds provided
	if req.Credentials != nil && req.Credentials.Token != "" {
		credBytes, err = s.encryptCreds(*req.Credentials)
		if err != nil {
			return nil, fmt.Errorf("encrypt credentials: %w", err)
		}
	}

	var localPath *string
	if req.LocalPath != nil {
		localPath = req.LocalPath
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE git_repositories
		SET name=$1, description=$2, remote_url=$3, branch=$4,
		    local_path=COALESCE($5, local_path),
		    auth_type=$6, credentials=$7, auto_push=$8, is_active=$9,
		    updated_at=NOW()
		WHERE id=$10`,
		existing.Name, existing.Description, existing.RemoteURL, existing.Branch,
		localPath, existing.AuthType, credBytes, existing.AutoPush, existing.IsActive, id,
	)
	if err != nil {
		return nil, fmt.Errorf("update git_repository: %w", err)
	}
	return s.Get(ctx, id, nil)
}

// Delete soft-deletes by setting is_active = false.
func (s *GitRepoService) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE git_repositories SET is_active=false, updated_at=NOW() WHERE id=$1`, id)
	return err
}

// ── Credential helpers ────────────────────────────────────────────────────────

func (s *GitRepoService) encryptCreds(creds models.GitCredentials) ([]byte, error) {
	plain, err := json.Marshal(creds)
	if err != nil {
		return nil, err
	}
	if s.credStore == nil {
		return plain, nil
	}
	return s.credStore.EncryptConfig(plain)
}

func (s *GitRepoService) decryptCreds(repo *models.GitRepository) (*models.GitCredentials, error) {
	raw := repo.Credentials
	if s.credStore != nil {
		if dec, err := s.credStore.DecryptConfig(raw); err == nil {
			raw = dec
		}
	}
	var creds models.GitCredentials
	if err := json.Unmarshal(raw, &creds); err != nil {
		return nil, fmt.Errorf("unmarshal credentials: %w", err)
	}
	return &creds, nil
}

// ── High-level git operations ─────────────────────────────────────────────────

// TestConnection verifies that the remote URL and credentials are reachable.
func (s *GitRepoService) TestConnection(ctx context.Context, id string) error {
	repo, err := s.Get(ctx, id, nil)
	if err != nil || repo == nil {
		return fmt.Errorf("repository not found")
	}
	creds, err := s.decryptCreds(repo)
	if err != nil {
		return err
	}
	return s.gitSvc.TestConnection(ctx, repo, creds)
}

// Pull fetches and fast-forwards the local clone from origin.
func (s *GitRepoService) Pull(ctx context.Context, id string, triggeredBy *string) (bool, error) {
	repo, err := s.Get(ctx, id, nil)
	if err != nil || repo == nil {
		return false, fmt.Errorf("repository not found")
	}
	creds, err := s.decryptCreds(repo)
	if err != nil {
		return false, err
	}

	start := time.Now()

	// Ensure clone exists first
	if _, err := s.gitSvc.EnsureCloned(ctx, repo, creds); err != nil {
		return false, fmt.Errorf("clone: %w", err)
	}

	changed, pullErr := s.gitSvc.Pull(ctx, repo, creds)
	dur := int(time.Since(start).Milliseconds())

	status := "success"
	errMsg := ""
	if pullErr != nil {
		status = "error"
		errMsg = pullErr.Error()
	}

	s.writeSyncLog(ctx, &models.GitSyncLogEntry{
		RepoID:      id,
		Operation:   "pull",
		Branch:      repo.Branch,
		Status:      status,
		ErrorMessage: errMsg,
		DurationMs:  dur,
		TriggeredBy: triggeredBy,
	})
	s.updateLastSync(ctx, id, status, errMsg)

	return changed, pullErr
}

// GetStatus returns the working-tree status for the local clone.
func (s *GitRepoService) GetStatus(ctx context.Context, id string) ([]models.GitStatusFile, error) {
	repo, err := s.Get(ctx, id, nil)
	if err != nil || repo == nil {
		return nil, fmt.Errorf("repository not found")
	}
	return s.gitSvc.Status(ctx, repo)
}

// GetLog returns recent commit history.
func (s *GitRepoService) GetLog(ctx context.Context, id string, limit int) ([]models.GitCommitSummary, error) {
	repo, err := s.Get(ctx, id, nil)
	if err != nil || repo == nil {
		return nil, fmt.Errorf("repository not found")
	}
	if limit <= 0 {
		limit = 20
	}
	return s.gitSvc.Log(ctx, repo, limit)
}

// CurrentBranch returns the currently checked-out branch name.
func (s *GitRepoService) CurrentBranch(ctx context.Context, id string) (string, error) {
	repo, err := s.Get(ctx, id, nil)
	if err != nil || repo == nil {
		return "", fmt.Errorf("repository not found")
	}
	return s.gitSvc.CurrentBranch(ctx, repo)
}

// ListBranches returns all local branch names.
func (s *GitRepoService) ListBranches(ctx context.Context, id string) ([]string, error) {
	repo, err := s.Get(ctx, id, nil)
	if err != nil || repo == nil {
		return nil, fmt.Errorf("repository not found")
	}
	return s.gitSvc.ListBranches(ctx, repo)
}

// CreateBranch creates a new local branch from HEAD.
func (s *GitRepoService) CreateBranch(ctx context.Context, id string, branchName string, triggeredBy *string) error {
	repo, err := s.Get(ctx, id, nil)
	if err != nil || repo == nil {
		return fmt.Errorf("repository not found")
	}
	err = s.gitSvc.CreateBranch(ctx, repo, branchName)
	status := "success"
	errMsg := ""
	if err != nil {
		status = "error"
		errMsg = err.Error()
	}
	s.writeSyncLog(ctx, &models.GitSyncLogEntry{
		RepoID:       id,
		Operation:    "branch_create",
		Branch:       branchName,
		Status:       status,
		ErrorMessage: errMsg,
		TriggeredBy:  triggeredBy,
	})
	return err
}

// Checkout switches the local clone to the named branch.
func (s *GitRepoService) Checkout(ctx context.Context, id string, branchName string, triggeredBy *string) error {
	repo, err := s.Get(ctx, id, nil)
	if err != nil || repo == nil {
		return fmt.Errorf("repository not found")
	}
	err = s.gitSvc.Checkout(ctx, repo, branchName)
	status := "success"
	errMsg := ""
	if err != nil {
		status = "error"
		errMsg = err.Error()
	}
	s.writeSyncLog(ctx, &models.GitSyncLogEntry{
		RepoID:       id,
		Operation:    "checkout",
		Branch:       branchName,
		Status:       status,
		ErrorMessage: errMsg,
		TriggeredBy:  triggeredBy,
	})
	return err
}

// CommitInterface exports the interface bundle, writes it to the repo, commits,
// and optionally pushes. Returns the commit SHA.
func (s *GitRepoService) CommitInterface(
	ctx context.Context,
	repoID string,
	req CommitInterfaceRequest,
	triggeredBy *string,
) (string, error) {
	repo, err := s.Get(ctx, repoID, nil)
	if err != nil || repo == nil {
		return "", fmt.Errorf("repository not found")
	}
	creds, err := s.decryptCreds(repo)
	if err != nil {
		return "", err
	}

	start := time.Now()

	// Ensure clone is up to date
	if _, err := s.gitSvc.EnsureCloned(ctx, repo, creds); err != nil {
		return "", fmt.Errorf("clone: %w", err)
	}
	if _, err := s.gitSvc.Pull(ctx, repo, creds); err != nil {
		// Non-fatal: might be a brand-new repo with nothing to pull
		_ = err
	}

	// Serialize interface to bundle
	authorName := req.AuthorName
	if authorName == "" {
		authorName = "ezHealthKonnect"
	}
	authorEmail := req.AuthorEmail
	if authorEmail == "" {
		authorEmail = "git@ezhealthkonnect"
	}

	bundle, err := s.serializer.SerializeInterface(ctx, req.InterfaceID, authorEmail)
	if err != nil {
		return "", fmt.Errorf("serialize interface: %w", err)
	}
	if bundle == nil {
		return "", fmt.Errorf("interface %s not found", req.InterfaceID)
	}

	// Marshal to indented JSON (readable diffs)
	bundleJSON, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal bundle: %w", err)
	}

	// Also write/update .ezhealthkonnect/config.json
	repoMeta := map[string]interface{}{
		"app":          "ezHealthKonnect",
		"app_version":  "2.0",
		"last_export":  time.Now().UTC(),
		"repo_id":      repoID,
	}
	metaJSON, _ := json.MarshalIndent(repoMeta, "", "  ")

	slug := MakeInterfaceSlug(bundle.Metadata.InterfaceName)
	bundlePath := fmt.Sprintf("interfaces/%s/bundle.json", slug)
	metaPath := ".ezhealthkonnect/config.json"

	commitMsg := req.CommitMessage
	if commitMsg == "" {
		commitMsg = fmt.Sprintf("Update %s interface bundle", bundle.Metadata.InterfaceName)
	}

	sha, commitErr := s.gitSvc.CommitFiles(ctx, repo, map[string][]byte{
		bundlePath: bundleJSON,
		metaPath:   metaJSON,
	}, commitMsg, authorName, authorEmail)

	dur := int(time.Since(start).Milliseconds())
	status := "success"
	errMsg := ""
	if commitErr != nil {
		status = "error"
		errMsg = commitErr.Error()
	}

	filesJSON, _ := json.Marshal([]string{bundlePath, metaPath})
	s.writeSyncLog(ctx, &models.GitSyncLogEntry{
		RepoID:        repoID,
		Operation:     "commit",
		InterfaceID:   &req.InterfaceID,
		Branch:        repo.Branch,
		CommitSHA:     sha,
		CommitMessage: commitMsg,
		FilesChanged:  filesJSON,
		Status:        status,
		ErrorMessage:  errMsg,
		DurationMs:    dur,
		TriggeredBy:   triggeredBy,
	})
	s.updateLastSync(ctx, repoID, status, errMsg)

	if commitErr != nil {
		return "", commitErr
	}

	// Push if auto_push or explicitly requested
	if repo.AutoPush {
		_, pushErr := s.gitSvc.Push(ctx, repo, creds)
		pushStatus := "success"
		pushErrMsg := ""
		if pushErr != nil {
			pushStatus = "error"
			pushErrMsg = pushErr.Error()
		}
		s.writeSyncLog(ctx, &models.GitSyncLogEntry{
			RepoID:       repoID,
			Operation:    "push",
			Branch:       repo.Branch,
			CommitSHA:    sha,
			Status:       pushStatus,
			ErrorMessage: pushErrMsg,
			TriggeredBy:  triggeredBy,
		})
	}

	return sha, nil
}

// CommitInterfacesRequest is the body for the batch commit endpoint.
type CommitInterfacesRequest struct {
	InterfaceIDs  []string `json:"interface_ids"`
	CommitMessage string   `json:"commit_message"`
	AuthorName    string   `json:"author_name"`
	AuthorEmail   string   `json:"author_email"`
	Push          bool     `json:"push"` // explicit push override (ignores auto_push)
}

// CommitInterfacesResult holds the per-interface result of a batch commit.
type CommitInterfaceResult struct {
	InterfaceID   string `json:"interface_id"`
	InterfaceName string `json:"interface_name"`
	FilePath      string `json:"file_path"`
	Skipped       bool   `json:"skipped"`
	Error         string `json:"error,omitempty"`
}

// CommitInterfaces serializes multiple interfaces into a single git commit.
// All selected interfaces are written to the worktree and staged before
// the commit is created, so they appear as one atomic change in the log.
func (s *GitRepoService) CommitInterfaces(
	ctx context.Context,
	repoID string,
	req CommitInterfacesRequest,
	triggeredBy *string,
) (string, []CommitInterfaceResult, error) {
	if len(req.InterfaceIDs) == 0 {
		return "", nil, fmt.Errorf("at least one interface_id is required")
	}

	repo, err := s.Get(ctx, repoID, nil)
	if err != nil || repo == nil {
		return "", nil, fmt.Errorf("repository not found")
	}
	creds, err := s.decryptCreds(repo)
	if err != nil {
		return "", nil, err
	}

	authorName := req.AuthorName
	if authorName == "" {
		authorName = "ezHealthKonnect"
	}
	authorEmail := req.AuthorEmail
	if authorEmail == "" {
		authorEmail = "git@ezhealthkonnect"
	}

	start := time.Now()

	// Ensure clone exists and is up to date
	if _, err := s.gitSvc.EnsureCloned(ctx, repo, creds); err != nil {
		return "", nil, fmt.Errorf("clone: %w", err)
	}
	_, _ = s.gitSvc.Pull(ctx, repo, creds) // non-fatal on empty/new repo

	// Serialize every requested interface
	files := map[string][]byte{}
	var results []CommitInterfaceResult
	var fileList []string

	for _, ifaceID := range req.InterfaceIDs {
		bundle, err := s.serializer.SerializeInterface(ctx, ifaceID, authorEmail)
		if err != nil || bundle == nil {
			results = append(results, CommitInterfaceResult{
				InterfaceID: ifaceID,
				Skipped:     true,
				Error:       "interface not found or serialization failed",
			})
			continue
		}

		bundleJSON, err := json.MarshalIndent(bundle, "", "  ")
		if err != nil {
			results = append(results, CommitInterfaceResult{
				InterfaceID:   ifaceID,
				InterfaceName: bundle.Metadata.InterfaceName,
				Skipped:       true,
				Error:         "marshal failed: " + err.Error(),
			})
			continue
		}

		slug := MakeInterfaceSlug(bundle.Metadata.InterfaceName)
		bundlePath := fmt.Sprintf("interfaces/%s/bundle.json", slug)
		files[bundlePath] = bundleJSON
		fileList = append(fileList, bundlePath)

		results = append(results, CommitInterfaceResult{
			InterfaceID:   ifaceID,
			InterfaceName: bundle.Metadata.InterfaceName,
			FilePath:      bundlePath,
		})
	}

	if len(files) == 0 {
		return "", results, fmt.Errorf("no valid interfaces to commit")
	}

	// Repo metadata file
	repoMeta := map[string]interface{}{
		"app":         "ezHealthKonnect",
		"app_version": "2.0",
		"last_export": time.Now().UTC(),
		"repo_id":     repoID,
	}
	metaJSON, _ := json.MarshalIndent(repoMeta, "", "  ")
	files[".ezhealthkonnect/config.json"] = metaJSON

	commitMsg := req.CommitMessage
	if commitMsg == "" {
		commitMsg = fmt.Sprintf("Update %d interface bundle(s)", len(files)-1)
	}

	sha, commitErr := s.gitSvc.CommitFiles(ctx, repo, files, commitMsg, authorName, authorEmail)

	dur := int(time.Since(start).Milliseconds())
	status := "success"
	errMsg := ""
	if commitErr != nil {
		status = "error"
		errMsg = commitErr.Error()
	}

	allFiles := append(fileList, ".ezhealthkonnect/config.json")
	filesJSON, _ := json.Marshal(allFiles)
	s.writeSyncLog(ctx, &models.GitSyncLogEntry{
		RepoID:        repoID,
		Operation:     "commit",
		Branch:        repo.Branch,
		CommitSHA:     sha,
		CommitMessage: commitMsg,
		FilesChanged:  filesJSON,
		Status:        status,
		ErrorMessage:  errMsg,
		DurationMs:    dur,
		TriggeredBy:   triggeredBy,
	})
	s.updateLastSync(ctx, repoID, status, errMsg)

	if commitErr != nil {
		return "", results, commitErr
	}

	// Push if auto_push or explicitly requested
	if repo.AutoPush || req.Push {
		_, pushErr := s.gitSvc.Push(ctx, repo, creds)
		pushStatus := "success"
		pushErrMsg := ""
		if pushErr != nil {
			pushStatus = "error"
			pushErrMsg = pushErr.Error()
		}
		s.writeSyncLog(ctx, &models.GitSyncLogEntry{
			RepoID:       repoID,
			Operation:    "push",
			Branch:       repo.Branch,
			CommitSHA:    sha,
			Status:       pushStatus,
			ErrorMessage: pushErrMsg,
			TriggeredBy:  triggeredBy,
		})
	}

	return sha, results, nil
}

// ExportPreview serializes the interface bundle without writing to git.
func (s *GitRepoService) ExportPreview(
	ctx context.Context,
	interfaceID string,
	exportedBy string,
) (*models.InterfaceBundle, error) {
	return s.serializer.SerializeInterface(ctx, interfaceID, exportedBy)
}

// Push pushes committed local changes to the remote origin.
// Returns true if new commits were actually pushed (false = already up to date).
func (s *GitRepoService) Push(ctx context.Context, id string, triggeredBy *string) (bool, error) {
	repo, err := s.Get(ctx, id, nil)
	if err != nil || repo == nil {
		return false, fmt.Errorf("repository not found")
	}
	creds, err := s.decryptCreds(repo)
	if err != nil {
		return false, err
	}

	start := time.Now()
	pushed, pushErr := s.gitSvc.Push(ctx, repo, creds)
	dur := int(time.Since(start).Milliseconds())

	status := "success"
	errMsg := ""
	if pushErr != nil {
		status = "error"
		errMsg = pushErr.Error()
	}

	s.writeSyncLog(ctx, &models.GitSyncLogEntry{
		RepoID:       id,
		Operation:    "push",
		Branch:       repo.Branch,
		Status:       status,
		ErrorMessage: errMsg,
		DurationMs:   dur,
		TriggeredBy:  triggeredBy,
	})
	s.updateLastSync(ctx, id, status, errMsg)
	return pushed, pushErr
}

// InterfaceDiffResult holds the current vs committed state of an interface.
type InterfaceDiffResult struct {
	InterfaceID  string                 `json:"interface_id"`
	HasChanges   bool                   `json:"has_changes"`
	NotCommitted bool                   `json:"not_committed"` // true if file not in git HEAD
	Current      *models.InterfaceBundle `json:"current"`
	Committed    *models.InterfaceBundle `json:"committed"`     // nil when NotCommitted
}

// InterfaceDiff returns the current serialized bundle and the last committed
// bundle side by side. If the interface has never been committed, Committed is
// nil and NotCommitted is true.
func (s *GitRepoService) InterfaceDiff(
	ctx context.Context,
	repoID, interfaceID, exportedBy string,
) (*InterfaceDiffResult, error) {
	repo, err := s.Get(ctx, repoID, nil)
	if err != nil || repo == nil {
		return nil, fmt.Errorf("repository not found")
	}
	creds, err := s.decryptCreds(repo)
	if err != nil {
		return nil, err
	}

	// Ensure clone exists (read-only — no pull needed)
	if _, err := s.gitSvc.EnsureCloned(ctx, repo, creds); err != nil {
		return nil, fmt.Errorf("clone: %w", err)
	}

	// Serialize current state from DB
	current, err := s.serializer.SerializeInterface(ctx, interfaceID, exportedBy)
	if err != nil {
		return nil, fmt.Errorf("serialize: %w", err)
	}
	if current == nil {
		return nil, fmt.Errorf("interface %s not found", interfaceID)
	}

	slug := MakeInterfaceSlug(current.Metadata.InterfaceName)
	bundlePath := fmt.Sprintf("interfaces/%s/bundle.json", slug)

	// Read committed version from git HEAD
	committedBytes, err := s.gitSvc.ReadFileAtHEAD(ctx, repo, bundlePath)
	if err != nil {
		return nil, fmt.Errorf("read HEAD: %w", err)
	}

	result := &InterfaceDiffResult{
		InterfaceID: interfaceID,
		Current:     current,
	}

	if committedBytes == nil {
		// Never committed to git
		result.NotCommitted = true
		result.HasChanges = true
		return result, nil
	}

	var committed models.InterfaceBundle
	if err := json.Unmarshal(committedBytes, &committed); err != nil {
		return nil, fmt.Errorf("parse committed bundle: %w", err)
	}
	result.Committed = &committed

	// Detect changes: compare interface + pipelines sections as JSON
	curJSON, _ := json.Marshal(map[string]interface{}{
		"interface":    current.Interface,
		"pipelines":    current.Pipelines,
		"connectivity": current.Connectivity,
	})
	comJSON, _ := json.Marshal(map[string]interface{}{
		"interface":    committed.Interface,
		"pipelines":    committed.Pipelines,
		"connectivity": committed.Connectivity,
	})
	result.HasChanges = string(curJSON) != string(comJSON)

	return result, nil
}

// GetSyncLog returns recent sync log entries for a repo.
func (s *GitRepoService) GetSyncLog(ctx context.Context, repoID string, limit int) ([]*models.GitSyncLogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, repo_id, operation, interface_id, branch,
		       commit_sha, commit_message, files_changed,
		       status, error_message, duration_ms, triggered_by, created_at
		FROM git_sync_log
		WHERE repo_id = $1
		ORDER BY created_at DESC
		LIMIT $2`, repoID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*models.GitSyncLogEntry
	for rows.Next() {
		e := &models.GitSyncLogEntry{}
		var interfaceID, triggeredBy sql.NullString
		var filesChanged []byte
		if err := rows.Scan(
			&e.ID, &e.RepoID, &e.Operation, &interfaceID, &e.Branch,
			&e.CommitSHA, &e.CommitMessage, &filesChanged,
			&e.Status, &e.ErrorMessage, &e.DurationMs, &triggeredBy, &e.CreatedAt,
		); err != nil {
			continue
		}
		if interfaceID.Valid {
			e.InterfaceID = &interfaceID.String
		}
		if triggeredBy.Valid {
			e.TriggeredBy = &triggeredBy.String
		}
		e.FilesChanged = filesChanged
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func (s *GitRepoService) writeSyncLog(ctx context.Context, entry *models.GitSyncLogEntry) {
	filesChanged := entry.FilesChanged
	if filesChanged == nil {
		filesChanged = []byte("[]")
	}
	interfaceID := (*string)(nil)
	if entry.InterfaceID != nil {
		interfaceID = entry.InterfaceID
	}
	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO git_sync_log
		       (repo_id, operation, interface_id, branch, commit_sha,
		        commit_message, files_changed, status, error_message,
		        duration_ms, triggered_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		entry.RepoID, entry.Operation, interfaceID, entry.Branch,
		entry.CommitSHA, entry.CommitMessage, filesChanged,
		entry.Status, entry.ErrorMessage, entry.DurationMs, entry.TriggeredBy,
	)
}

func (s *GitRepoService) updateLastSync(ctx context.Context, id, status, errMsg string) {
	_, _ = s.db.ExecContext(ctx, `
		UPDATE git_repositories
		SET last_sync_at=NOW(), last_sync_status=$1, last_sync_error=$2, updated_at=NOW()
		WHERE id=$3`, status, errMsg, id)
}

// scanRepo scans a db row into a GitRepository struct.
// Used by List() which returns *sql.Rows.
func (s *GitRepoService) scanRepo(rows *sql.Rows, r *models.GitRepository) error {
	var credsRaw []byte
	var lastSyncAt sql.NullTime
	var localPath sql.NullString
	var createdBy sql.NullString

	err := rows.Scan(
		&r.ID, &r.Name, &r.Description, &r.RemoteURL, &r.Branch,
		&localPath, &r.AuthType,
		&credsRaw, &r.AutoPush, &r.IsActive,
		&lastSyncAt, &r.LastSyncStatus, &r.LastSyncError,
		&createdBy, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return err
	}
	r.Credentials = credsRaw
	if lastSyncAt.Valid {
		t := lastSyncAt.Time
		r.LastSyncAt = &t
	}
	if localPath.Valid {
		r.LocalPath = localPath.String
	}
	if createdBy.Valid {
		r.CreatedByUserID = &createdBy.String
	}
	return nil
}

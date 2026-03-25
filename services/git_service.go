package services

// ================================================================
// GitService — low-level go-git operations
//
// Wraps github.com/go-git/go-git/v5 with:
//   - Per-repository mutex (sync.Map) for thread-safe filesystem ops
//   - Context-aware cancellation on all blocking operations
//   - Configurable base directory (GIT_REPOS_BASE_DIR env var)
//
// All exported methods acquire the per-repo lock for their duration.
// Different repos run concurrently; the same repo is serialised.
// ================================================================

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/models"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gogithttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
)

// GitService manages local git repository clones and remote operations.
type GitService struct {
	locks       sync.Map // key: repoID (string) → *sync.Mutex
	baseDir     string   // root for auto-resolved local clone paths
}

// NewGitService creates a GitService. The base directory for clones is read from
// the GIT_REPOS_BASE_DIR env var, defaulting to /data/git-repos on Linux or
// %TEMP%/ezhk-git-repos on Windows.
func NewGitService() *GitService {
	base := os.Getenv("GIT_REPOS_BASE_DIR")
	if base == "" {
		if runtime.GOOS == "windows" {
			base = filepath.Join(os.TempDir(), "ezhk-git-repos")
		} else {
			base = "/data/git-repos"
		}
	}
	return &GitService{baseDir: base}
}

// ── internal helpers ──────────────────────────────────────────────────────────

// acquireRepoLock fetches (or lazily creates) the per-repo mutex and locks it.
// Returns an unlock function; always defer it.
func (gs *GitService) acquireRepoLock(repoID string) func() {
	v, _ := gs.locks.LoadOrStore(repoID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// resolvedLocalPath returns the filesystem path where this repo is (or will be) cloned.
func (gs *GitService) resolvedLocalPath(repo *models.GitRepository) string {
	if repo.LocalPath != "" {
		return repo.LocalPath
	}
	return filepath.Join(gs.baseDir, repo.ID)
}

// authFromCredentials builds the go-git BasicAuth transport adapter.
func (gs *GitService) authFromCredentials(creds *models.GitCredentials) *gogithttp.BasicAuth {
	username := creds.Username
	if username == "" {
		username = "git" // GitHub accepts any non-empty username with a PAT
	}
	return &gogithttp.BasicAuth{
		Username: username,
		Password: creds.Token,
	}
}

// ── Repository lifecycle ──────────────────────────────────────────────────────

// EnsureCloned clones the repository if the local path doesn't exist, or opens it
// if it does. Returns the *git.Repository ready for further operations.
func (gs *GitService) EnsureCloned(
	ctx context.Context,
	repo *models.GitRepository,
	creds *models.GitCredentials,
) (*git.Repository, error) {
	unlock := gs.acquireRepoLock(repo.ID)
	defer unlock()

	localPath := gs.resolvedLocalPath(repo)

	// Check if already cloned
	if _, err := os.Stat(filepath.Join(localPath, ".git")); err == nil {
		return git.PlainOpen(localPath)
	}

	if err := os.MkdirAll(localPath, 0o755); err != nil {
		return nil, fmt.Errorf("create clone dir: %w", err)
	}

	cloneOpts := &git.CloneOptions{
		URL:           repo.RemoteURL,
		ReferenceName: plumbing.NewBranchReferenceName(repo.Branch),
		SingleBranch:  true,
		Depth:         0, // full history
		Auth:          gs.authFromCredentials(creds),
		Progress:      nil,
	}

	r, err := git.PlainCloneContext(ctx, localPath, false, cloneOpts)
	if err != nil {
		return nil, fmt.Errorf("clone %s: %w", repo.RemoteURL, err)
	}
	return r, nil
}

// Pull fetches and fast-forwards the configured branch from origin.
// Returns true if any new commits were pulled.
// Opens the existing clone (must have been cloned first via EnsureCloned).
func (gs *GitService) Pull(
	ctx context.Context,
	repo *models.GitRepository,
	creds *models.GitCredentials,
) (bool, error) {
	unlock := gs.acquireRepoLock(repo.ID)
	defer unlock()

	localPath := gs.resolvedLocalPath(repo)
	r, err := git.PlainOpen(localPath)
	if err != nil {
		return false, fmt.Errorf("open repo: %w", err)
	}

	wt, err := r.Worktree()
	if err != nil {
		return false, fmt.Errorf("worktree: %w", err)
	}

	err = wt.PullContext(ctx, &git.PullOptions{
		RemoteName:    "origin",
		ReferenceName: plumbing.NewBranchReferenceName(repo.Branch),
		Auth:          gs.authFromCredentials(creds),
		Force:         false,
	})
	if err == git.NoErrAlreadyUpToDate {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("pull: %w", err)
	}
	return true, nil
}

// Push pushes the current HEAD to origin.
// Returns (true, nil) when new commits were pushed, (false, nil) when already up to date.
func (gs *GitService) Push(
	ctx context.Context,
	repo *models.GitRepository,
	creds *models.GitCredentials,
) (bool, error) {
	unlock := gs.acquireRepoLock(repo.ID)
	defer unlock()

	localPath := gs.resolvedLocalPath(repo)
	r, err := git.PlainOpen(localPath)
	if err != nil {
		return false, fmt.Errorf("open repo: %w", err)
	}

	refSpec := config.RefSpec(
		"refs/heads/" + repo.Branch + ":refs/heads/" + repo.Branch,
	)
	err = r.PushContext(ctx, &git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{refSpec},
		Auth:       gs.authFromCredentials(creds),
	})
	if err == git.NoErrAlreadyUpToDate {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ReadFileAtHEAD reads the content of relPath from the HEAD commit.
// Returns (nil, nil) if the file does not exist in HEAD (never committed).
func (gs *GitService) ReadFileAtHEAD(
	ctx context.Context,
	repo *models.GitRepository,
	relPath string,
) ([]byte, error) {
	unlock := gs.acquireRepoLock(repo.ID)
	defer unlock()

	localPath := gs.resolvedLocalPath(repo)
	r, err := git.PlainOpen(localPath)
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	ref, err := r.Head()
	if err != nil {
		// No commits yet
		return nil, nil
	}

	commit, err := r.CommitObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("commit object: %w", err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("tree: %w", err)
	}

	file, err := tree.File(relPath)
	if err != nil {
		// File not in HEAD — never committed
		return nil, nil
	}

	contents, err := file.Contents()
	if err != nil {
		return nil, fmt.Errorf("read contents: %w", err)
	}
	return []byte(contents), nil
}

// ── File operations ───────────────────────────────────────────────────────────

// WriteAndStage writes content to a relative file path inside the worktree and
// stages it with `git add`. Parent directories are created as needed.
// NOTE: caller must already hold the repo lock (called from Commit which does).
func (gs *GitService) writeAndStageNoLock(
	localPath string,
	relPath string,
	content []byte,
) error {
	absPath := filepath.Join(localPath, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Errorf("mkdirall for %s: %w", relPath, err)
	}
	if err := os.WriteFile(absPath, content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", relPath, err)
	}
	return nil
}

// CommitFiles writes multiple files, stages them all, and creates a commit.
// Returns the commit SHA. Acquires the per-repo lock for the full duration.
func (gs *GitService) CommitFiles(
	ctx context.Context,
	repo *models.GitRepository,
	files map[string][]byte, // relPath → content
	message string,
	authorName string,
	authorEmail string,
) (string, error) {
	unlock := gs.acquireRepoLock(repo.ID)
	defer unlock()

	localPath := gs.resolvedLocalPath(repo)
	r, err := git.PlainOpen(localPath)
	if err != nil {
		return "", fmt.Errorf("open repo: %w", err)
	}

	wt, err := r.Worktree()
	if err != nil {
		return "", fmt.Errorf("worktree: %w", err)
	}

	// Write + stage each file
	for relPath, content := range files {
		if err := gs.writeAndStageNoLock(localPath, relPath, content); err != nil {
			return "", err
		}
		// go-git Add uses forward-slash paths
		if _, err := wt.Add(filepath.ToSlash(relPath)); err != nil {
			return "", fmt.Errorf("git add %s: %w", relPath, err)
		}
	}

	hash, err := wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return hash.String(), nil
}

// ── Status & history ──────────────────────────────────────────────────────────

// Status returns the working-tree status for the local clone.
func (gs *GitService) Status(
	ctx context.Context,
	repo *models.GitRepository,
) ([]models.GitStatusFile, error) {
	unlock := gs.acquireRepoLock(repo.ID)
	defer unlock()

	localPath := gs.resolvedLocalPath(repo)
	r, err := git.PlainOpen(localPath)
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	wt, err := r.Worktree()
	if err != nil {
		return nil, fmt.Errorf("worktree: %w", err)
	}

	s, err := wt.Status()
	if err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}

	var result []models.GitStatusFile
	for path, fs := range s {
		result = append(result, models.GitStatusFile{
			Path:     path,
			Staging:  string(fs.Staging),
			Worktree: string(fs.Worktree),
		})
	}
	return result, nil
}

// Log returns up to limit recent commits on the current branch.
func (gs *GitService) Log(
	ctx context.Context,
	repo *models.GitRepository,
	limit int,
) ([]models.GitCommitSummary, error) {
	unlock := gs.acquireRepoLock(repo.ID)
	defer unlock()

	localPath := gs.resolvedLocalPath(repo)
	r, err := git.PlainOpen(localPath)
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	ref, err := r.Head()
	if err != nil {
		return nil, fmt.Errorf("HEAD: %w", err)
	}

	iter, err := r.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		return nil, fmt.Errorf("log: %w", err)
	}
	defer iter.Close()

	var commits []models.GitCommitSummary
	for i := 0; i < limit; i++ {
		c, err := iter.Next()
		if err != nil {
			break
		}
		sha := c.Hash.String()
		commits = append(commits, models.GitCommitSummary{
			SHA:       sha,
			ShortSHA:  sha[:7],
			Message:   strings.TrimSpace(c.Message),
			Author:    c.Author.Name,
			Email:     c.Author.Email,
			Timestamp: c.Author.When,
		})
	}
	return commits, nil
}

// CurrentBranch returns the name of the currently checked-out local branch.
func (gs *GitService) CurrentBranch(
	ctx context.Context,
	repo *models.GitRepository,
) (string, error) {
	unlock := gs.acquireRepoLock(repo.ID)
	defer unlock()

	localPath := gs.resolvedLocalPath(repo)
	r, err := git.PlainOpen(localPath)
	if err != nil {
		return "", fmt.Errorf("open repo: %w", err)
	}

	ref, err := r.Head()
	if err != nil {
		return "", fmt.Errorf("HEAD: %w", err)
	}
	if ref.Name().IsBranch() {
		return ref.Name().Short(), nil
	}
	return ref.Hash().String()[:7], nil // detached HEAD
}

// ── Branch management ─────────────────────────────────────────────────────────

// ListBranches returns all local branch names.
func (gs *GitService) ListBranches(
	ctx context.Context,
	repo *models.GitRepository,
) ([]string, error) {
	unlock := gs.acquireRepoLock(repo.ID)
	defer unlock()

	localPath := gs.resolvedLocalPath(repo)
	r, err := git.PlainOpen(localPath)
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	iter, err := r.Branches()
	if err != nil {
		return nil, fmt.Errorf("branches: %w", err)
	}
	defer iter.Close()

	var branches []string
	_ = iter.ForEach(func(ref *plumbing.Reference) error {
		branches = append(branches, ref.Name().Short())
		return nil
	})
	return branches, nil
}

// CreateBranch creates a new local branch pointing at the current HEAD.
func (gs *GitService) CreateBranch(
	ctx context.Context,
	repo *models.GitRepository,
	branchName string,
) error {
	unlock := gs.acquireRepoLock(repo.ID)
	defer unlock()

	localPath := gs.resolvedLocalPath(repo)
	r, err := git.PlainOpen(localPath)
	if err != nil {
		return fmt.Errorf("open repo: %w", err)
	}

	head, err := r.Head()
	if err != nil {
		return fmt.Errorf("HEAD: %w", err)
	}

	ref := plumbing.NewHashReference(
		plumbing.NewBranchReferenceName(branchName),
		head.Hash(),
	)
	return r.Storer.SetReference(ref)
}

// Checkout switches the worktree to the named local branch.
func (gs *GitService) Checkout(
	ctx context.Context,
	repo *models.GitRepository,
	branchName string,
) error {
	unlock := gs.acquireRepoLock(repo.ID)
	defer unlock()

	localPath := gs.resolvedLocalPath(repo)
	r, err := git.PlainOpen(localPath)
	if err != nil {
		return fmt.Errorf("open repo: %w", err)
	}

	wt, err := r.Worktree()
	if err != nil {
		return fmt.Errorf("worktree: %w", err)
	}

	return wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
		Create: false,
	})
}

// ── Connection testing ────────────────────────────────────────────────────────

// TestConnection performs a lightweight ls-remote to verify that the URL and
// credentials are valid without cloning the full repository.
func (gs *GitService) TestConnection(
	ctx context.Context,
	repo *models.GitRepository,
	creds *models.GitCredentials,
) error {
	rem := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{repo.RemoteURL},
	})

	_, err := rem.ListContext(ctx, &git.ListOptions{
		Auth: gs.authFromCredentials(creds),
	})
	if err != nil {
		return fmt.Errorf("cannot reach %s: %w", repo.RemoteURL, err)
	}
	return nil
}

// ResolvedLocalPath exposes the computed local path for external use.
func (gs *GitService) ResolvedLocalPath(repo *models.GitRepository) string {
	return gs.resolvedLocalPath(repo)
}

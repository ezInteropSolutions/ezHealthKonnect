package services

// ================================================================
// git_integration_test.go — Unit tests for GitSerializer and GitService
//
// Surfaces tested (no real network, no real DB):
//   1. MakeInterfaceSlug
//   2. makeEnvVarName
//   3. placeholderSensitiveValues
//   4. GitService.ResolvedLocalPath
//   5. GitService.acquireRepoLock (concurrency)
//   6. GitService.CommitFiles (filesystem-only go-git)
//   7. GitService.Status after CommitFiles
//   8. GitService.Log after CommitFiles
//   9. GitService.Branch operations
// ================================================================

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ezhealthkonnect/models"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// initLocalRepo creates a new git repo in dir with a single initial commit on
// "main" so that HEAD is valid and branch operations work correctly.
func initLocalRepo(t *testing.T, dir string) *git.Repository {
	t.Helper()

	r, err := git.PlainInitWithOptions(dir, &git.PlainInitOptions{
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")},
	})
	if err != nil {
		t.Fatalf("PlainInitWithOptions: %v", err)
	}

	wt, err := r.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	// Write an initial file so the first commit has content.
	initFile := filepath.Join(dir, "README.md")
	if err := os.WriteFile(initFile, []byte("# init\n"), 0o644); err != nil {
		t.Fatalf("WriteFile README: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("git add README: %v", err)
	}

	_, err = wt.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test Author",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	return r
}

// repoModel returns a minimal *models.GitRepository pointing at localPath.
func repoModel(id, localPath string) *models.GitRepository {
	return &models.GitRepository{
		ID:        id,
		LocalPath: localPath,
		Branch:    "main",
	}
}

// isHexSHA returns true when s is a 40-character lowercase hex string.
func isHexSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	matched, _ := regexp.MatchString(`^[0-9a-f]{40}$`, s)
	return matched
}

// ── 1. MakeInterfaceSlug ─────────────────────────────────────────────────────

func TestMakeInterfaceSlug(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Epic ADT Inbound", "epic-adt-inbound"},
		{"Epic ADT Inbound v2", "epic-adt-inbound-v2"},
		{"  My Interface!!  ", "my-interface"},
		{"123 Numbers Only", "123-numbers-only"},
		{"", "interface"},
		{"---", "interface"},
		{"UPPER CASE", "upper-case"},
		{"already-slug", "already-slug"},
		{"special@#$%chars", "special-chars"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(fmt.Sprintf("input=%q", tc.input), func(t *testing.T) {
			got := MakeInterfaceSlug(tc.input)
			if got != tc.want {
				t.Errorf("MakeInterfaceSlug(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ── 2. makeEnvVarName ────────────────────────────────────────────────────────

func TestMakeEnvVarName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"password", "PASSWORD"},
		{"accessKeyId", "ACCESS_KEY_ID"},
		{"db_password", "DB_PASSWORD"},
		{"apiKey", "API_KEY"},
		{"secretAccessKey", "SECRET_ACCESS_KEY"},
		{"token", "TOKEN"},
		{"mySecretValue", "MY_SECRET_VALUE"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			got := makeEnvVarName(tc.input)
			if got != tc.want {
				t.Errorf("makeEnvVarName(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ── 3. placeholderSensitiveValues ────────────────────────────────────────────

func TestPlaceholderSensitiveValues(t *testing.T) {
	// GitSerializer with nil db and nil credStore is safe for unit tests that
	// only exercise placeholderSensitiveValues (no DB calls made).
	gs := &GitSerializer{}

	t.Run("password key is replaced", func(t *testing.T) {
		in := map[string]interface{}{"password": "hunter2"}
		out := gs.placeholderSensitiveValues(in)
		got, ok := out["password"].(string)
		if !ok {
			t.Fatalf("password key missing or not string: %v", out["password"])
		}
		if got != "{{PASSWORD}}" {
			t.Errorf("got %q; want {{PASSWORD}}", got)
		}
	})

	t.Run("token key is replaced", func(t *testing.T) {
		in := map[string]interface{}{"token": "ghp_abc123"}
		out := gs.placeholderSensitiveValues(in)
		got, _ := out["token"].(string)
		if got != "{{TOKEN}}" {
			t.Errorf("got %q; want {{TOKEN}}", got)
		}
	})

	t.Run("host unchanged, password replaced", func(t *testing.T) {
		in := map[string]interface{}{
			"host":     "localhost",
			"password": "s3cr3t",
		}
		out := gs.placeholderSensitiveValues(in)
		if host, _ := out["host"].(string); host != "localhost" {
			t.Errorf("host = %q; want localhost", host)
		}
		if pw, _ := out["password"].(string); pw != "{{PASSWORD}}" {
			t.Errorf("password = %q; want {{PASSWORD}}", pw)
		}
	})

	t.Run("api_key replaced", func(t *testing.T) {
		in := map[string]interface{}{"api_key": "xyz"}
		out := gs.placeholderSensitiveValues(in)
		got, _ := out["api_key"].(string)
		if got != "{{API_KEY}}" {
			t.Errorf("got %q; want {{API_KEY}}", got)
		}
	})

	t.Run("nested map: db.password replaced, db.host unchanged", func(t *testing.T) {
		in := map[string]interface{}{
			"db": map[string]interface{}{
				"password": "x",
				"host":     "h",
			},
		}
		out := gs.placeholderSensitiveValues(in)

		dbMap, ok := out["db"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected nested map under 'db', got %T", out["db"])
		}
		if pw, _ := dbMap["password"].(string); pw != "{{PASSWORD}}" {
			t.Errorf("db.password = %q; want {{PASSWORD}}", pw)
		}
		if host, _ := dbMap["host"].(string); host != "h" {
			t.Errorf("db.host = %q; want h", host)
		}
	})

	t.Run("username is NOT sensitive — value unchanged", func(t *testing.T) {
		in := map[string]interface{}{"username": "admin"}
		out := gs.placeholderSensitiveValues(in)
		if got, _ := out["username"].(string); got != "admin" {
			t.Errorf("username = %q; want admin", got)
		}
	})

	t.Run("nil map returns nil", func(t *testing.T) {
		out := gs.placeholderSensitiveValues(nil)
		if out != nil {
			t.Errorf("expected nil, got %v", out)
		}
	})
}

// ── 4. GitService.ResolvedLocalPath ──────────────────────────────────────────

func TestResolvedLocalPath(t *testing.T) {
	t.Run("custom LocalPath is returned as-is", func(t *testing.T) {
		gs := &GitService{baseDir: "/some/base"}
		repo := &models.GitRepository{ID: "repo-1", LocalPath: "/custom/path"}
		got := gs.ResolvedLocalPath(repo)
		if got != "/custom/path" {
			t.Errorf("got %q; want /custom/path", got)
		}
	})

	t.Run("empty LocalPath returns baseDir+ID", func(t *testing.T) {
		baseDir := t.TempDir()
		// Point the env var so NewGitService picks up our baseDir.
		t.Setenv("GIT_REPOS_BASE_DIR", baseDir)

		gs := NewGitService()
		repo := &models.GitRepository{ID: "my-repo-id", LocalPath: ""}
		want := filepath.Join(baseDir, "my-repo-id")
		got := gs.ResolvedLocalPath(repo)
		if got != want {
			t.Errorf("got %q; want %q", got, want)
		}
	})
}

// ── 5. GitService.acquireRepoLock (concurrency) ───────────────────────────────

func TestAcquireRepoLock_SameRepo_Serialized(t *testing.T) {
	gs := &GitService{}
	repoID := "concurrent-repo"

	var (
		counter int64 // incremented inside the critical section
		wg      sync.WaitGroup
		overlap int64 // > 0 means two goroutines were inside simultaneously
	)

	work := func() {
		defer wg.Done()
		unlock := gs.acquireRepoLock(repoID)
		defer unlock()

		// Mark entry
		if atomic.AddInt64(&counter, 1) > 1 {
			atomic.StoreInt64(&overlap, 1)
		}
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt64(&counter, -1)
	}

	wg.Add(2)
	go work()
	go work()
	wg.Wait()

	if atomic.LoadInt64(&overlap) != 0 {
		t.Error("two goroutines were inside the critical section simultaneously (same repoID)")
	}
}

func TestAcquireRepoLock_DifferentRepos_Concurrent(t *testing.T) {
	gs := &GitService{}

	var wg sync.WaitGroup
	start := time.Now()

	// Both goroutines sleep 20ms inside their respective locks.
	// If serialized they would take >= 40ms; if concurrent ~20ms.
	for _, id := range []string{"repo-A", "repo-B"} {
		id := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := gs.acquireRepoLock(id)
			defer unlock()
			time.Sleep(20 * time.Millisecond)
		}()
	}
	wg.Wait()

	elapsed := time.Since(start)
	// Allow generous headroom: must finish in under 35ms (not 40ms).
	if elapsed >= 35*time.Millisecond {
		t.Errorf("different repos appear to be serialized: elapsed=%v (expected < 35ms)", elapsed)
	}
}

// ── 6. GitService.CommitFiles ─────────────────────────────────────────────────

func TestCommitFiles(t *testing.T) {
	ctx := context.Background()

	t.Run("single file produces non-empty 40-char hex SHA", func(t *testing.T) {
		dir := t.TempDir()
		initLocalRepo(t, dir)

		gs := &GitService{baseDir: dir}
		repo := repoModel("r1", dir)

		sha, err := gs.CommitFiles(ctx, repo,
			map[string][]byte{"hello.txt": []byte("hello world")},
			"add hello",
			"Test Author", "test@example.com",
		)
		if err != nil {
			t.Fatalf("CommitFiles: %v", err)
		}
		if !isHexSHA(sha) {
			t.Errorf("SHA %q is not a 40-char hex string", sha)
		}
	})

	t.Run("two files in one call — both staged and SHA non-empty", func(t *testing.T) {
		dir := t.TempDir()
		initLocalRepo(t, dir)

		gs := &GitService{baseDir: dir}
		repo := repoModel("r2", dir)

		files := map[string][]byte{
			"a.txt": []byte("aaa"),
			"b.txt": []byte("bbb"),
		}
		sha, err := gs.CommitFiles(ctx, repo, files, "add two files",
			"Test Author", "test@example.com")
		if err != nil {
			t.Fatalf("CommitFiles: %v", err)
		}
		if !isHexSHA(sha) {
			t.Errorf("SHA %q is not a 40-char hex string", sha)
		}

		// Verify both files exist in the worktree.
		for name := range files {
			if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
				t.Errorf("expected file %q to exist: %v", name, err)
			}
		}
	})

	t.Run("empty commit message still succeeds", func(t *testing.T) {
		dir := t.TempDir()
		initLocalRepo(t, dir)

		gs := &GitService{baseDir: dir}
		repo := repoModel("r3", dir)

		sha, err := gs.CommitFiles(ctx, repo,
			map[string][]byte{"empty-msg.txt": []byte("data")},
			"", // empty message
			"Test Author", "test@example.com",
		)
		if err != nil {
			t.Fatalf("CommitFiles with empty message: %v", err)
		}
		if !isHexSHA(sha) {
			t.Errorf("SHA %q is not a 40-char hex string", sha)
		}
	})

	t.Run("subdirectory path — directories created automatically", func(t *testing.T) {
		dir := t.TempDir()
		initLocalRepo(t, dir)

		gs := &GitService{baseDir: dir}
		repo := repoModel("r4", dir)

		relPath := "interfaces/my-iface/bundle.json"
		sha, err := gs.CommitFiles(ctx, repo,
			map[string][]byte{relPath: []byte(`{"version":"1.0"}`)},
			"add bundle",
			"Test Author", "test@example.com",
		)
		if err != nil {
			t.Fatalf("CommitFiles with subdir: %v", err)
		}
		if !isHexSHA(sha) {
			t.Errorf("SHA %q is not a 40-char hex string", sha)
		}

		absPath := filepath.Join(dir, filepath.FromSlash(relPath))
		if _, err := os.Stat(absPath); err != nil {
			t.Errorf("expected %q to exist: %v", absPath, err)
		}
	})
}

// ── 7. GitService.Status ──────────────────────────────────────────────────────

func TestStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("after commit status is clean", func(t *testing.T) {
		dir := t.TempDir()
		initLocalRepo(t, dir)

		gs := &GitService{baseDir: dir}
		repo := repoModel("s1", dir)

		// Commit one file.
		_, err := gs.CommitFiles(ctx, repo,
			map[string][]byte{"committed.txt": []byte("committed content")},
			"add committed.txt",
			"Test Author", "test@example.com",
		)
		if err != nil {
			t.Fatalf("CommitFiles: %v", err)
		}

		files, err := gs.Status(ctx, repo)
		if err != nil {
			t.Fatalf("Status: %v", err)
		}

		// Any non-clean entry (ignoring Untracked status " ") is an error.
		for _, f := range files {
			if f.Staging != " " && f.Staging != "" {
				t.Errorf("unexpected staging status for %q: %q", f.Path, f.Staging)
			}
			if f.Worktree != " " && f.Worktree != "" {
				t.Errorf("unexpected worktree status for %q: %q", f.Path, f.Worktree)
			}
		}
	})

	t.Run("untracked file appears with worktree='?'", func(t *testing.T) {
		dir := t.TempDir()
		initLocalRepo(t, dir)

		gs := &GitService{baseDir: dir}
		repo := repoModel("s2", dir)

		// Write a file without staging it.
		untrackedPath := filepath.Join(dir, "untracked.txt")
		if err := os.WriteFile(untrackedPath, []byte("not staged"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		files, err := gs.Status(ctx, repo)
		if err != nil {
			t.Fatalf("Status: %v", err)
		}

		found := false
		for _, f := range files {
			if f.Path == "untracked.txt" {
				found = true
				if f.Worktree != "?" {
					t.Errorf("untracked.txt worktree = %q; want '?'", f.Worktree)
				}
			}
		}
		if !found {
			t.Error("untracked.txt not found in status output")
		}
	})
}

// ── 8. GitService.Log ─────────────────────────────────────────────────────────

func TestLog(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	initLocalRepo(t, dir)

	gs := &GitService{baseDir: dir}
	repo := repoModel("log-repo", dir)

	messages := []string{"first commit", "second commit", "third commit"}
	for _, msg := range messages {
		_, err := gs.CommitFiles(ctx, repo,
			map[string][]byte{msg + ".txt": []byte(msg)},
			msg,
			"Test Author", "test@example.com",
		)
		if err != nil {
			t.Fatalf("CommitFiles %q: %v", msg, err)
		}
	}

	t.Run("limit=2 returns 2 entries in reverse-chronological order", func(t *testing.T) {
		commits, err := gs.Log(ctx, repo, 2)
		if err != nil {
			t.Fatalf("Log: %v", err)
		}
		if len(commits) != 2 {
			t.Fatalf("expected 2 commits, got %d", len(commits))
		}
		// Most recent first.
		if commits[0].Message != "third commit" {
			t.Errorf("commits[0].Message = %q; want 'third commit'", commits[0].Message)
		}
		if commits[1].Message != "second commit" {
			t.Errorf("commits[1].Message = %q; want 'second commit'", commits[1].Message)
		}
	})

	t.Run("limit=10 returns all 4 commits when only 4 exist (3 + initial)", func(t *testing.T) {
		commits, err := gs.Log(ctx, repo, 10)
		if err != nil {
			t.Fatalf("Log: %v", err)
		}
		// 3 test commits + 1 initial commit from initLocalRepo.
		if len(commits) != 4 {
			t.Errorf("expected 4 commits, got %d", len(commits))
		}
	})

	t.Run("each returned commit has a valid 40-char SHA", func(t *testing.T) {
		commits, err := gs.Log(ctx, repo, 10)
		if err != nil {
			t.Fatalf("Log: %v", err)
		}
		for i, c := range commits {
			if !isHexSHA(c.SHA) {
				t.Errorf("commits[%d].SHA %q is not a 40-char hex string", i, c.SHA)
			}
		}
	})
}

// ── 9. Branch operations ──────────────────────────────────────────────────────

func TestBranchOperations(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	initLocalRepo(t, dir)

	gs := &GitService{baseDir: dir}
	repo := repoModel("branch-repo", dir)

	t.Run("CurrentBranch on fresh repo is 'main'", func(t *testing.T) {
		branch, err := gs.CurrentBranch(ctx, repo)
		if err != nil {
			t.Fatalf("CurrentBranch: %v", err)
		}
		if branch != "main" {
			t.Errorf("CurrentBranch = %q; want 'main'", branch)
		}
	})

	t.Run("CreateBranch 'feature/test' succeeds", func(t *testing.T) {
		if err := gs.CreateBranch(ctx, repo, "feature/test"); err != nil {
			t.Fatalf("CreateBranch: %v", err)
		}
	})

	t.Run("ListBranches returns both 'main' and 'feature/test'", func(t *testing.T) {
		branches, err := gs.ListBranches(ctx, repo)
		if err != nil {
			t.Fatalf("ListBranches: %v", err)
		}

		sort.Strings(branches)
		want := []string{"feature/test", "main"}

		if len(branches) != len(want) {
			t.Fatalf("ListBranches = %v; want %v", branches, want)
		}
		for i, b := range branches {
			if b != want[i] {
				t.Errorf("branches[%d] = %q; want %q", i, b, want[i])
			}
		}
	})

	t.Run("Checkout 'feature/test' succeeds", func(t *testing.T) {
		if err := gs.Checkout(ctx, repo, "feature/test"); err != nil {
			t.Fatalf("Checkout: %v", err)
		}
	})

	t.Run("CurrentBranch after Checkout is 'feature/test'", func(t *testing.T) {
		branch, err := gs.CurrentBranch(ctx, repo)
		if err != nil {
			t.Fatalf("CurrentBranch: %v", err)
		}
		if branch != "feature/test" {
			t.Errorf("CurrentBranch = %q; want 'feature/test'", branch)
		}
	})
}

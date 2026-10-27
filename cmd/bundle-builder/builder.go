package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FileLister lists repository files eligible for the bundle.
type FileLister interface {
	List() ([]string, error) // slash-separated paths relative to the repo root
}

// GitLister lists git-tracked files, so nothing untracked (a local .env, logs,
// test output) can ever reach a customer.
type GitLister struct{ RepoRoot string }

// List implements FileLister.
func (g GitLister) List() ([]string, error) {
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = g.RepoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var files []string
	for _, f := range strings.Split(string(out), "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}

// Fetcher returns a local copy of a runtime artifact whose SHA-256 matches.
type Fetcher interface {
	Fetch(a RuntimeArtifact) (string, error)
}

// HTTPFetcher downloads into a cache directory and verifies the pinned hash.
type HTTPFetcher struct {
	CacheDir string
	Client   *http.Client
}

// Fetch implements Fetcher.
func (f HTTPFetcher) Fetch(a RuntimeArtifact) (string, error) {
	if err := os.MkdirAll(f.CacheDir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(f.CacheDir, a.SHA256+"-"+path.Base(a.URL))
	if sum, err := fileSHA256(dest); err == nil && sum == a.SHA256 {
		return dest, nil
	}
	client := f.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	resp, err := client.Get(a.URL)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", a.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", a.URL, resp.StatusCode)
	}
	tmp := dest + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), resp.Body); err != nil {
		out.Close()
		os.Remove(tmp)
		return "", fmt.Errorf("download %s: %w", a.URL, err)
	}
	out.Close()
	if got := hex.EncodeToString(h.Sum(nil)); got != a.SHA256 {
		os.Remove(tmp)
		return "", fmt.Errorf("%s: SHA-256 %s does not match the pinned %s — refusing to bundle it", a.URL, got, a.SHA256)
	}
	return dest, os.Rename(tmp, dest)
}

// DepsInstaller installs production Node dependencies into a directory.
type DepsInstaller interface {
	Install(dir, goos, goarch string) error
}

// NpmInstaller runs `npm ci` for the target platform (optional native
// accelerators are chosen for the target, not the build host). Install
// scripts are not run: no production dependency needs them.
type NpmInstaller struct{}

// Install implements DepsInstaller.
func (NpmInstaller) Install(dir, goos, goarch string) error {
	npmOS := map[string]string{"windows": "win32"}[goos]
	if npmOS == "" {
		npmOS = goos
	}
	npmCPU := map[string]string{"amd64": "x64"}[goarch]
	if npmCPU == "" {
		npmCPU = goarch
	}
	npm := "npm"
	if os.PathSeparator == '\\' {
		npm = "npm.cmd"
	}
	cmd := exec.Command(npm, "ci", "--omit=dev", "--ignore-scripts", "--no-audit", "--no-fund",
		"--os="+npmOS, "--cpu="+npmCPU)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

// BundleInfo is written into the bundle as bundle-manifest.json; the
// installer uses it to know which top-level entries the application owns.
type BundleInfo struct {
	Version     string   `json:"version"`
	OS          string   `json:"os"`
	Arch        string   `json:"arch"`
	NodeVersion string   `json:"nodeVersion"`
	Entries     []string `json:"entries"`
	Protected   []string `json:"protected"`
	BuiltAt     string   `json:"builtAt"`
}

// Builder assembles the Standalone application bundle.
type Builder struct {
	RepoRoot string
	Manifest *Manifest
	Lister   FileLister
	Fetcher  Fetcher
	Deps     DepsInstaller
	Log      func(format string, args ...any)
}

// Options for one build.
type Options struct {
	OS, Arch, Version string
	GoAPIPath         string
	Out               string
}

// Build stages the bundle and writes it as a zip with a single root folder
// ("ezhealthkonnect/"). Returns the zip's SHA-256.
func (b *Builder) Build(opts Options) (string, error) {
	target := opts.OS + "/" + opts.Arch
	node, ok := b.Manifest.Runtimes["node"].Targets[target]
	if !ok {
		return "", fmt.Errorf("no Node.js runtime pinned for %s", target)
	}

	stageRoot, err := os.MkdirTemp("", "ezhk-bundle-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stageRoot)
	stage := filepath.Join(stageRoot, "ezhealthkonnect")

	// 1. Tracked application files.
	files, err := b.Lister.List()
	if err != nil {
		return "", err
	}
	count := 0
	for _, f := range files {
		if !b.Manifest.Includes(f) {
			continue
		}
		if err := copyFile(filepath.Join(b.RepoRoot, filepath.FromSlash(f)), filepath.Join(stage, filepath.FromSlash(f)), 0o644); err != nil {
			return "", err
		}
		count++
	}
	if err := b.requireListedPathsPresent(files); err != nil {
		return "", err
	}
	b.Log("staged %d tracked files", count)

	// 2. Production Node dependencies for the target platform.
	if err := b.Deps.Install(stage, opts.OS, opts.Arch); err != nil {
		return "", fmt.Errorf("install node dependencies: %w", err)
	}

	// 3. Go backend.
	goAPIName := "go-api"
	if opts.OS == "windows" {
		goAPIName = "go-api.exe"
	}
	if err := copyFile(opts.GoAPIPath, filepath.Join(stage, goAPIName), 0o755); err != nil {
		return "", fmt.Errorf("copy go-api: %w", err)
	}

	// 4. Bundled runtimes (verified against pinned hashes).
	nodeArchive, err := b.Fetcher.Fetch(node)
	if err != nil {
		return "", err
	}
	nodeDir := filepath.Join(stage, "runtime", "node")
	if strings.HasSuffix(node.URL, ".zip") {
		err = extractZipStripRoot(nodeArchive, nodeDir)
	} else {
		err = extractTarGzStripRoot(nodeArchive, nodeDir)
	}
	if err != nil {
		return "", fmt.Errorf("extract node runtime: %w", err)
	}
	if opts.OS == "windows" {
		winsw, ok := b.Manifest.Runtimes["winsw"].Targets[target]
		if !ok {
			return "", fmt.Errorf("no WinSW pinned for %s", target)
		}
		p, err := b.Fetcher.Fetch(winsw)
		if err != nil {
			return "", err
		}
		if err := copyFile(p, filepath.Join(stage, "runtime", "winsw", "WinSW-x64.exe"), 0o755); err != nil {
			return "", err
		}
		for dst, src := range b.Manifest.WindowsFiles {
			if err := copyFile(filepath.Join(b.RepoRoot, filepath.FromSlash(src)), filepath.Join(stage, filepath.FromSlash(dst)), 0o644); err != nil {
				return "", err
			}
		}
	}

	// 5. Bundle info for the installer.
	entries, err := topLevel(stage)
	if err != nil {
		return "", err
	}
	entries = append(entries, "bundle-manifest.json")
	sort.Strings(entries)
	for _, e := range entries {
		for _, p := range b.Manifest.Protected {
			if e == p {
				return "", fmt.Errorf("bundle would contain protected entry %q", e)
			}
		}
	}
	info := BundleInfo{Version: opts.Version, OS: opts.OS, Arch: opts.Arch, NodeVersion: b.Manifest.Runtimes["node"].Version,
		Entries: entries, Protected: b.Manifest.Protected, BuiltAt: time.Now().UTC().Format(time.RFC3339)}
	infoJSON, _ := json.MarshalIndent(info, "", "  ")
	if err := os.WriteFile(filepath.Join(stage, "bundle-manifest.json"), infoJSON, 0o644); err != nil {
		return "", err
	}

	// 6. Zip.
	if err := writeZip(stageRoot, "ezhealthkonnect", opts.Out); err != nil {
		return "", err
	}
	return fileSHA256(opts.Out)
}

// requireListedPathsPresent fails when a manifest file or dir matches no
// tracked file — a renamed directory must not silently drop out of the bundle.
func (b *Builder) requireListedPathsPresent(tracked []string) error {
	for _, f := range b.Manifest.Files {
		found := false
		for _, t := range tracked {
			if t == f {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("manifest file %q is not tracked in git", f)
		}
	}
	for _, d := range b.Manifest.Dirs {
		found := false
		for _, t := range tracked {
			if strings.HasPrefix(t, d+"/") && b.Manifest.Includes(t) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("manifest dir %q contributes no files", d)
		}
	}
	return nil
}

func topLevel(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out, nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func fileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// safeJoin joins an archive member name under dest. A pinned runtime never
// contains absolute or ".." paths, so one is treated as tampering and fails
// the build rather than being "cleaned" into place.
func safeJoin(dest, name string) (string, error) {
	n := strings.ReplaceAll(name, "\\", "/")
	if n == "" || strings.HasPrefix(n, "/") || filepath.IsAbs(n) {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	for _, seg := range strings.Split(n, "/") {
		if seg == ".." {
			return "", fmt.Errorf("unsafe archive path %q", name)
		}
	}
	return filepath.Join(dest, filepath.FromSlash(path.Clean(n))), nil
}

func stripRoot(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.Index(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return ""
}

func extractTarGzStripRoot(archive, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := stripRoot(hdr.Name)
		if name == "" {
			continue
		}
		target, err := safeJoin(dest, name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		case tar.TypeSymlink:
			// Node's tarball links bin/npm -> ../lib/node_modules/npm/bin/npm-cli.js.
			// The bundle only needs bin/node; skip links (zip cannot hold them portably).
		}
	}
}

func extractZipStripRoot(archive, dest string) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		name := stripRoot(f.Name)
		if name == "" {
			continue
		}
		target, err := safeJoin(dest, name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() || strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// writeZip zips root/<name> into out, keeping file modes (executables stay
// executable when the Linux installer extracts them).
func writeZip(root, name, out string) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	base := filepath.Join(root, name)
	walkErr := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = rel
		if d.IsDir() {
			hdr.Name += "/"
			_, err := zw.CreateHeader(hdr)
			return err
		}
		hdr.Method = zip.Deflate
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		src, err := os.Open(p)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(w, src)
		return err
	})
	if walkErr != nil {
		zw.Close()
		f.Close()
		return walkErr
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

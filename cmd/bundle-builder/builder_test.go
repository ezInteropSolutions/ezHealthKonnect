package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const repoRoot = "../.."

func realManifest(t *testing.T) *Manifest {
	t.Helper()
	m, err := LoadManifest(filepath.Join(repoRoot, "packaging", "bundle-manifest.json"))
	if err != nil {
		t.Fatalf("real manifest: %v", err)
	}
	return m
}

// The Docker image is the runtime layout that is exercised by every CI run.
// Every path its runtime stage copies must also ship in the Standalone bundle
// — the v26.10 bundle once silently omitted edi/, ncpdp/, ncpdptelecom/,
// uscdi/ and astm/.
func TestManifest_CoversEveryDockerfileRuntimeCopy(t *testing.T) {
	m := realManifest(t)
	f, err := os.Open(filepath.Join(repoRoot, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var copies []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "FROM ") {
			copies = nil // only the final (runtime) stage counts
			continue
		}
		if !strings.HasPrefix(line, "COPY ") || strings.Contains(line, "--from=") {
			continue
		}
		fields := strings.Fields(line)
		for _, src := range fields[1 : len(fields)-1] {
			copies = append(copies, strings.TrimSuffix(src, "/"))
		}
	}
	if len(copies) < 15 {
		t.Fatalf("parsed only %d runtime COPY sources from the Dockerfile", len(copies))
	}
	tracked, err := GitLister{RepoRoot: repoRoot}.List()
	if err != nil {
		t.Skipf("git not available: %v", err)
	}
	// A Dockerfile path is covered if the manifest ships it, or if it holds no
	// runtime files at all (Go source only — compiled into go-api).
	runtimeFiles := func(src string) []string {
		var out []string
		for _, f := range tracked {
			if (f == src || strings.HasPrefix(f, src+"/")) && !m.Excluded(f) {
				out = append(out, f)
			}
		}
		return out
	}
	notNeeded := map[string]bool{"docker-entrypoint.sh": true} // Docker-only startup script
	for _, src := range copies {
		if notNeeded[src] {
			continue
		}
		covered := false
		for _, f := range m.Files {
			if f == src {
				covered = true
			}
		}
		for _, d := range m.Dirs {
			if d == src {
				covered = true
			}
		}
		if !covered {
			if rf := runtimeFiles(src); len(rf) > 0 {
				t.Errorf("Dockerfile ships %q at runtime but packaging/bundle-manifest.json does not (e.g. %s)", src, rf[0])
			}
		}
	}
}

func TestManifest_RealManifestIsBuildable(t *testing.T) {
	m := realManifest(t)
	files, err := GitLister{RepoRoot: repoRoot}.List()
	if err != nil {
		t.Skipf("git not available: %v", err)
	}
	b := &Builder{Manifest: m}
	if err := b.requireListedPathsPresent(files); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range files {
		if m.Includes(f) {
			n++
			if strings.HasSuffix(f, ".go") || strings.Contains(f, "/testdata/") || strings.HasPrefix(f, ".env") {
				t.Errorf("%s must not ship", f)
			}
		}
	}
	if n < 500 {
		t.Errorf("only %d files would ship — manifest looks wrong", n)
	}
	for _, src := range m.WindowsFiles {
		if _, err := os.Stat(filepath.Join(repoRoot, src)); err != nil {
			t.Errorf("windows file %s: %v", src, err)
		}
	}
	for name, rt := range m.Runtimes {
		for target, a := range rt.Targets {
			if len(a.SHA256) != 64 || !strings.HasPrefix(a.URL, "https://") {
				t.Errorf("runtime %s %s must be an https URL with a pinned SHA-256", name, target)
			}
		}
	}
	pg, ok := m.InstallerDownloads["postgresql-windows-amd64"]
	if !ok || len(pg.SHA256) != 64 || !strings.HasPrefix(pg.URL, "https://") || !strings.Contains(pg.URL, pg.Version) {
		t.Errorf("the Windows PostgreSQL installer must be pinned (https URL matching its version + SHA-256): %+v", pg)
	}
}

func TestManifest_Includes(t *testing.T) {
	m := &Manifest{
		Files:     []string{"app.js"},
		Dirs:      []string{"services", "edi"},
		Exclude:   ExcludeRules{Suffixes: []string{".go", ".test.js"}, DirNames: []string{"testdata"}, Files: []string{"go.mod"}},
		Protected: []string{".env"},
	}
	cases := map[string]bool{
		"app.js":                            true,
		"services/x.js":                     true,
		"services/x.go":                     false,
		"services/x.test.js":                false,
		"edi/schemas/x12/manifest.json":     true,
		"edi/testdata/real_samples/835.txt": false,
		"edi/go.mod":                        false,
		"README.md":                         false,
		"servicesX/y.js":                    false,
	}
	for p, want := range cases {
		if got := m.Includes(p); got != want {
			t.Errorf("Includes(%q) = %v, want %v", p, got, want)
		}
	}
	bad := &Manifest{Dirs: []string{"logs"}, Protected: []string{"logs"}}
	if err := bad.Validate(); err == nil {
		t.Error("a manifest shipping a protected entry must be rejected")
	}
}

type fakeLister []string

func (f fakeLister) List() ([]string, error) { return f, nil }

type fakeFetcher map[string]string // url -> local path

func (f fakeFetcher) Fetch(a RuntimeArtifact) (string, error) { return f[a.URL], nil }

type fakeDeps struct{}

func (fakeDeps) Install(dir, goos, goarch string) error {
	return os.WriteFile(filepath.Join(mustMkdir(filepath.Join(dir, "node_modules", "express")), "index.js"), []byte("x"), 0o644)
}

func mustMkdir(p string) string { os.MkdirAll(p, 0o755); return p }

func writeTestFile(t *testing.T, p, content string) {
	t.Helper()
	mustMkdir(filepath.Dir(p))
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeNodeTarGz(t *testing.T, p string, extra ...string) {
	t.Helper()
	f, _ := os.Create(p)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	add := func(name string, mode int64, body string) {
		tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.WriteHeader(&tar.Header{Name: "node-v1/bin/", Mode: 0o755, Typeflag: tar.TypeDir})
	add("node-v1/bin/node", 0o755, "ELF")
	add("node-v1/LICENSE", 0o644, "MIT")
	tw.WriteHeader(&tar.Header{Name: "node-v1/bin/npm", Linkname: "../lib/npm-cli.js", Typeflag: tar.TypeSymlink})
	for _, e := range extra {
		add(e, 0o644, "hostile")
	}
	tw.Close()
	gz.Close()
	f.Close()
}

func TestBuild_ProducesInstallableBundle(t *testing.T) {
	repo := t.TempDir()
	writeTestFile(t, filepath.Join(repo, "app.js"), "app")
	writeTestFile(t, filepath.Join(repo, "services", "svc.js"), "svc")
	writeTestFile(t, filepath.Join(repo, "services", "svc.go"), "package x")
	writeTestFile(t, filepath.Join(repo, "edi", "testdata", "sample.txt"), "fixture")
	writeTestFile(t, filepath.Join(repo, "edi", "schemas", "m.json"), "{}")
	writeTestFile(t, filepath.Join(repo, ".env"), "SECRET=1") // untracked: never listed
	goAPI := filepath.Join(t.TempDir(), "go-api")
	writeTestFile(t, goAPI, "binary")
	nodeTar := filepath.Join(t.TempDir(), "node.tar.gz")
	makeNodeTarGz(t, nodeTar)

	m := &Manifest{
		Files:     []string{"app.js"},
		Dirs:      []string{"services", "edi"},
		Exclude:   ExcludeRules{Suffixes: []string{".go"}, DirNames: []string{"testdata"}},
		Protected: []string{".env", "logs", "storage"},
		Runtimes: map[string]Runtime{"node": {Version: "22.0.0", Targets: map[string]RuntimeArtifact{
			"linux/amd64": {URL: "https://example/node.tar.gz", SHA256: strings.Repeat("a", 64)}}}},
	}
	out := filepath.Join(t.TempDir(), "bundle.zip")
	b := &Builder{RepoRoot: repo, Manifest: m,
		Lister:  fakeLister{"app.js", "services/svc.js", "services/svc.go", "edi/testdata/sample.txt", "edi/schemas/m.json"},
		Fetcher: fakeFetcher{"https://example/node.tar.gz": nodeTar},
		Deps:    fakeDeps{}, Log: t.Logf}
	sum, err := b.Build(Options{OS: "linux", Arch: "amd64", Version: "v9.9", GoAPIPath: goAPI, Out: out})
	if err != nil {
		t.Fatal(err)
	}
	if len(sum) != 64 {
		t.Fatalf("sha256 = %q", sum)
	}

	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	names := map[string]*zip.File{}
	for _, f := range zr.File {
		names[f.Name] = f
	}
	for _, want := range []string{"ezhealthkonnect/app.js", "ezhealthkonnect/services/svc.js", "ezhealthkonnect/edi/schemas/m.json",
		"ezhealthkonnect/go-api", "ezhealthkonnect/node_modules/express/index.js", "ezhealthkonnect/runtime/node/bin/node",
		"ezhealthkonnect/bundle-manifest.json"} {
		if names[want] == nil {
			t.Errorf("bundle is missing %s", want)
		}
	}
	for name := range names {
		if strings.HasSuffix(name, ".go") || strings.Contains(name, "testdata") || strings.Contains(name, ".env") ||
			strings.Contains(name, "escape") || strings.HasSuffix(name, "bin/npm") {
			t.Errorf("bundle must not contain %s", name)
		}
	}
	if runtime.GOOS != "windows" {
		if mode := names["ezhealthkonnect/go-api"].Mode(); mode&0o111 == 0 {
			t.Errorf("go-api must stay executable, mode %v", mode)
		}
		if mode := names["ezhealthkonnect/runtime/node/bin/node"].Mode(); mode&0o111 == 0 {
			t.Errorf("node must stay executable, mode %v", mode)
		}
	}
	rc, _ := names["ezhealthkonnect/bundle-manifest.json"].Open()
	var info BundleInfo
	json.NewDecoder(rc).Decode(&info) //nolint:errcheck
	rc.Close()
	if info.Version != "v9.9" || strings.Join(info.Entries, ",") != "app.js,bundle-manifest.json,edi,go-api,node_modules,runtime,services" {
		t.Errorf("bundle info = %+v", info)
	}
}

func TestBuild_FailsWhenAListedDirShipsNothing(t *testing.T) {
	m := &Manifest{Files: []string{"app.js"}, Dirs: []string{"renamed"}, Protected: []string{".env"}}
	b := &Builder{Manifest: m}
	if err := b.requireListedPathsPresent([]string{"app.js", "other/x.js"}); err == nil {
		t.Fatal("a manifest dir with no tracked files must fail the build")
	}
}

func TestHTTPFetcher_VerifiesPinnedHash(t *testing.T) {
	body := []byte("runtime bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write(body) }))
	defer srv.Close()
	sum := sha256.Sum256(body)
	good := hex.EncodeToString(sum[:])
	f := HTTPFetcher{CacheDir: t.TempDir()}

	p, err := f.Fetch(RuntimeArtifact{URL: srv.URL + "/rt.bin", SHA256: good})
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(p); string(data) != string(body) {
		t.Fatal("wrong content")
	}
	if _, err := f.Fetch(RuntimeArtifact{URL: srv.URL + "/other.bin", SHA256: strings.Repeat("0", 64)}); err == nil ||
		!strings.Contains(err.Error(), "does not match") {
		t.Fatalf("a tampered download must be refused, got %v", err)
	}
	entries, _ := os.ReadDir(f.CacheDir)
	for _, e := range entries {
		if strings.Contains(e.Name(), "other.bin") {
			t.Fatal("a rejected download must not stay in the cache")
		}
	}
}

func TestExtract_RejectsPathTraversal(t *testing.T) {
	for _, hostile := range []string{"node-v1/../../escape", "node-v1/a/../../../escape"} {
		p := filepath.Join(t.TempDir(), "node.tar.gz")
		makeNodeTarGz(t, p, hostile)
		dest := filepath.Join(t.TempDir(), "out")
		if err := extractTarGzStripRoot(p, dest); err == nil || !strings.Contains(err.Error(), "unsafe archive path") {
			t.Errorf("%s: expected refusal, got %v", hostile, err)
		}
	}
}

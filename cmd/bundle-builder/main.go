// Command bundle-builder assembles the Standalone application bundle that the
// installer ships: git-tracked runtime files from packaging/bundle-manifest.json,
// production node_modules for the target platform, the go-api binary, and the
// pinned, checksum-verified Node.js (and, for Windows, WinSW) runtimes.
//
//	go run ./cmd/bundle-builder -os linux -arch amd64 -version v26.10 \
//	    -go-api dist/go-api -out installer/assets/bundle-linux-amd64.zip
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func main() {
	goos := flag.String("os", "", "target OS: linux | windows")
	arch := flag.String("arch", "amd64", "target architecture: amd64 | arm64")
	version := flag.String("version", "", "release version recorded in the bundle (e.g. v26.10)")
	goAPI := flag.String("go-api", "", "path to the go-api binary built for the target")
	out := flag.String("out", "", "output zip path")
	repo := flag.String("repo", ".", "repository root")
	manifestPath := flag.String("manifest", "packaging/bundle-manifest.json", "bundle manifest")
	cache := flag.String("cache", filepath.Join(os.TempDir(), "ezhk-bundle-cache"), "download cache for runtimes")
	flag.Parse()

	if *goos == "" || *version == "" || *goAPI == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	m, err := LoadManifest(filepath.Join(*repo, *manifestPath))
	if err != nil {
		log.Fatalf("bundle-builder: %v", err)
	}
	b := &Builder{
		RepoRoot: *repo,
		Manifest: m,
		Lister:   GitLister{RepoRoot: *repo},
		Fetcher:  HTTPFetcher{CacheDir: *cache},
		Deps:     NpmInstaller{},
		Log:      log.Printf,
	}
	sum, err := b.Build(Options{OS: *goos, Arch: *arch, Version: *version, GoAPIPath: *goAPI, Out: *out})
	if err != nil {
		log.Fatalf("bundle-builder: %v", err)
	}
	fmt.Printf("%s  %s\n", sum, *out)
}

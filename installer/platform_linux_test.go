//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The layout of /etc/yum.repos.d/pgdg-redhat-all.repo from pgdg-redhat-repo 42.0-70.
const pgdgRepoSample = `[pgdg-common]
name=PostgreSQL common RPMs for RHEL / Rocky Linux / AlmaLinux $releasever_major.$releasever_minor - $basearch
baseurl=https://download.postgresql.org/pub/repos/yum/common/redhat/rhel-$releasever_major.$releasever_minor-$basearch
enabled=1

[pgdg17]
name=PostgreSQL 17
baseurl=https://download.postgresql.org/pub/repos/yum/17/redhat/rhel-$releasever_major.$releasever_minor-$basearch
enabled=1

[pgdg18]
name=PostgreSQL 18
baseurl=https://download.postgresql.org/pub/repos/yum/18/redhat/rhel-$releasever_major.$releasever_minor-$basearch
enabled=1

[pgdg17-source]
name=PostgreSQL 17 source
enabled=0
`

func TestIsVersionRepo(t *testing.T) {
	for s, want := range map[string]bool{"pgdg17": true, "pgdg-common": false, "pgdg17-source": false, "pgdg": false} {
		if isVersionRepo(s) != want {
			t.Errorf("isVersionRepo(%q) != %v", s, want)
		}
	}
}

func TestConfigurePgdgReposContent(t *testing.T) {
	// configurePgdgRepos edits /etc; exercise its line logic on a copy.
	dir := t.TempDir()
	p := filepath.Join(dir, "pgdg-redhat-all.repo")
	os.WriteFile(p, []byte(pgdgRepoSample), 0o644)
	got := rewriteRepoForTest(t, p, 17)
	if !strings.Contains(got, "[pgdg18]\nname=PostgreSQL 18\nbaseurl=https://download.postgresql.org/pub/repos/yum/18/redhat/rhel-$releasever_major-$basearch\nenabled=0") {
		t.Errorf("pgdg18 must be disabled:\n%s", got)
	}
	if !strings.Contains(got, "[pgdg17]\nname=PostgreSQL 17\nbaseurl=https://download.postgresql.org/pub/repos/yum/17/redhat/rhel-$releasever_major-$basearch\nenabled=1") {
		t.Errorf("pgdg17 must stay enabled on the major-only path:\n%s", got)
	}
	for _, l := range strings.Split(got, "\n") {
		if strings.HasPrefix(l, "baseurl") && strings.Contains(l, "$releasever_minor") {
			t.Errorf("minor-version baseurl left: %s", l)
		}
	}
}

func rewriteRepoForTest(t *testing.T, p string, major int) string {
	t.Helper()
	if err := rewritePgdgRepoFile(p, major); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	return string(b)
}

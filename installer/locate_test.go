package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// installed makes dir look like an ezHealthKonnect installation.
func installed(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	s := &InstallState{Version: "v26.10", DBMode: "external", Service: true}
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestFindInstall_FirstCandidateWithAnInstallRecordWins(t *testing.T) {
	root := t.TempDir()
	stale := filepath.Join(root, "stale") // e.g. an old registry value: folder without a record
	_ = os.MkdirAll(stale, 0o755)
	real := installed(t, filepath.Join(root, "D-drive", "ezhk"))
	fallback := filepath.Join(root, "default")

	if got := findInstall([]string{"", stale, real}, fallback); got != real {
		t.Errorf("got %q, want the folder that holds an install record (%q)", got, real)
	}
	if got := findInstall([]string{stale, ""}, fallback); got != fallback {
		t.Errorf("with no installation found got %q, want the default %q", got, fallback)
	}
}

func TestInstallRootFor_WalksUpFromTheServiceProgram(t *testing.T) {
	dir := installed(t, filepath.Join(t.TempDir(), "My Apps", "ezhk"))
	svc := filepath.Join(dir, "tools", "ezhealthkonnect.exe")
	if got := installRootFor(svc); got != dir {
		t.Errorf("service program %q: got %q, want %q", svc, got, dir)
	}
	if got := installRootFor(dir); got != dir {
		t.Errorf("the install folder itself: got %q, want %q", got, dir)
	}
	if got := installRootFor(filepath.Join(t.TempDir(), "unrelated", "x.exe")); got != "" {
		t.Errorf("an unrelated path must not be treated as an install, got %q", got)
	}
}

func TestProgramPath(t *testing.T) {
	cases := map[string]string{
		`"C:\Program Files\ezhk\tools\ezhealthkonnect.exe"`: `C:\Program Files\ezhk\tools\ezhealthkonnect.exe`,
		`"C:\ezhk\tools\ezhealthkonnect.exe" --service`:     `C:\ezhk\tools\ezhealthkonnect.exe`,
		`C:\ezhk\tools\ezhealthkonnect.exe`:                 `C:\ezhk\tools\ezhealthkonnect.exe`,
		`C:\ezhk\tools\ezhealthkonnect.exe arg1 "arg two"`:  `C:\ezhk\tools\ezhealthkonnect.exe`,
		`  "D:\x y\tools\ezhealthkonnect.exe"  `:            `D:\x y\tools\ezhealthkonnect.exe`,
		`C:\no extension here`:                              `C:\no extension here`,
		`"C:\ezhk\tools\ezhealthkonnect.exe`:                `C:\ezhk\tools\ezhealthkonnect.exe`,
		`/opt/ezhk/run.sh --flag`:                           `/opt/ezhk/run.sh`,
	}
	for in, want := range cases {
		if got := programPath(in); got != want {
			t.Errorf("programPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseRegValue(t *testing.T) {
	out := "\r\nHKEY_LOCAL_MACHINE\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\ezHealthKonnect\r\n" +
		"    InstallLocation    REG_SZ    D:\\Apps\\ez health\r\n\r\n"
	if got := parseRegValue(out, "InstallLocation"); got != `D:\Apps\ez health` {
		t.Errorf("got %q", got)
	}
	if got := parseRegValue(out, "DisplayName"); got != "" {
		t.Errorf("a value that is not there must be empty, got %q", got)
	}
	exp := "    ImagePath    REG_EXPAND_SZ    \"C:\\ezhk\\tools\\ezhealthkonnect.exe\"\r\n"
	if got := parseRegValue(exp, "ImagePath"); got != `"C:\ezhk\tools\ezhealthkonnect.exe"` {
		t.Errorf("expandable string: got %q", got)
	}
}

func TestParseUnitValue(t *testing.T) {
	unit := "[Unit]\nDescription=ezHealthKonnect\n\n[Service]\nUser=ezhealthkonnect\nWorkingDirectory=/srv/ezhk\nExecStart=/srv/ezhk/start.sh\n"
	if got := parseUnitValue(unit, "WorkingDirectory"); got != "/srv/ezhk" {
		t.Errorf("got %q", got)
	}
	if got := parseUnitValue(unit, "Environment"); got != "" {
		t.Errorf("got %q for a missing key", got)
	}
}

// The reported bug: an install in a folder other than the default was
// invisible, so the wizard offered a fresh install and no Uninstall.
func TestUIServer_StateFindsAnInstallOutsideTheDefaultFolder(t *testing.T) {
	custom := installed(t, filepath.Join(t.TempDir(), "custom"))
	web := fstest.MapFS{"index.html": {Data: []byte("<html>")}}
	s := NewUIServer(&fakePlatform{elevated: true}, nil, web, 41234)
	host := "127.0.0.1:41234"

	state := func() StateResponse {
		req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
		req.Host = host
		req.Header.Set("X-Installer-Token", s.Token)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		var st StateResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
			t.Fatalf("state: %v (%s)", err, rec.Body.String())
		}
		return st
	}

	if st := state(); st.Installed {
		t.Fatalf("without a locator only the default folder is checked, but got installed=%v at %q", st.Installed, st.InstallDir)
	}
	s.Locate = func() []string { return []string{custom} }
	st := state()
	if !st.Installed || st.InstallDir != custom {
		t.Errorf("installed=%v installDir=%q, want the custom install at %q", st.Installed, st.InstallDir, custom)
	}
}

// Uninstall used to remove whatever folder it was pointed at.
func TestUninstall_RefusesAFolderThatIsNotAnInstallation(t *testing.T) {
	h := newHarness(t)
	victim := filepath.Join(t.TempDir(), "Documents")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(victim, "important.txt")
	if err := os.WriteFile(keep, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := h.installer("").Uninstall(victim, false)
	if err == nil || !strings.Contains(err.Error(), "nothing was removed") {
		t.Fatalf("uninstall of a non-installation: err = %v, want a refusal", err)
	}
	if got := readFile(t, keep); got != "mine" {
		t.Errorf("a file in the folder was changed: %q", got)
	}
}

func TestUninstall_StillRemovesARealInstallation(t *testing.T) {
	h := newHarness(t)
	h.install(t, "v26.10", map[string]string{"app.txt": "x"})
	if err := h.installer("").Uninstall(h.dir, false); err != nil {
		t.Fatalf("uninstall: %v\n%s", err, h.console)
	}
	if _, err := os.Stat(h.dir); !os.IsNotExist(err) {
		t.Errorf("install folder still there after uninstall (stat err = %v)", err)
	}
}

// A failed install must still be listed in Add/Remove Programs so it can be removed.
func TestInstall_RegistersBeforeTheStepsThatCanFail(t *testing.T) {
	h := newHarness(t)
	h.plat.startErr = errors.New("service did not start")
	_, err := h.installer(makeBundle(t, "v26.10", nil)).Install(Config{InstallDir: h.dir})
	if err == nil {
		t.Fatal("expected the install to fail when the service cannot start")
	}
	calls := h.plat.callString()
	reg, start := strings.Index(calls, "register"), strings.Index(calls, "start")
	if reg < 0 || reg > start {
		t.Errorf("a failed install must already be registered before the service starts: %s", calls)
	}
}

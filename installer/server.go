package main

// server.go — the installer's local web UI backend.
//
// It listens on 127.0.0.1 only, and every /api request must carry the random
// per-run token that is only in the URL the installer opens. Without it, any
// web page open in the same browser could drive the installer (including
// uninstall, which drops the database) — a plain text/plain POST needs no
// CORS preflight. The Host and Origin headers are checked too (DNS rebinding).

import (
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// UIServer serves the installer web UI and its API.
type UIServer struct {
	Platform Platform
	Bundle   BundleSource
	Web      fs.FS
	Token    string
	Port     int
	Done     chan struct{}

	// Locate returns where an existing installation may be (see locate.go);
	// nil means only the default folder is checked.
	Locate func() []string

	hub      *Hub
	mu       sync.Mutex
	busy     bool
	doneOnce sync.Once

	// Wizard activity, for exiting once the wizard window is gone (see Idle).
	lastSeen time.Time // last authorized API request; zero until the wizard connects
	streams  int       // open progress streams
}

// NewUIServer creates a server with a fresh random token.
func NewUIServer(p Platform, b BundleSource, web fs.FS, port int) *UIServer {
	return &UIServer{Platform: p, Bundle: b, Web: web, Port: port, Token: hex.EncodeToString(randomBytes(32)),
		Done: make(chan struct{}), hub: NewHub()}
}

// URL is what the installer opens in the browser.
func (s *UIServer) URL(extraQuery string) string {
	u := fmt.Sprintf("http://127.0.0.1:%d/?t=%s", s.Port, s.Token)
	if extraQuery != "" {
		u += "&" + extraQuery
	}
	return u
}

// Handler returns the HTTP handler.
func (s *UIServer) Handler() http.Handler {
	mux := http.NewServeMux()
	api := map[string]http.HandlerFunc{
		"/api/state":     s.handleState,
		"/api/defaults":  s.handleDefaults,
		"/api/checks":    s.handleChecks,
		"/api/install":   s.handleInstall,
		"/api/upgrade":   s.handleUpgrade,
		"/api/rollback":  s.handleRollback,
		"/api/uninstall": s.handleUninstall,
		"/api/progress":  s.handleProgress,
		"/api/browse":    s.handleBrowse,
		"/api/close":     s.handleClose,
		"/api/ping":      s.handlePing,
	}
	for p, h := range api {
		mux.Handle(p, s.guard(h))
	}
	mux.Handle("/", s.hostOnly(http.FileServer(http.FS(s.Web))))
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		next.ServeHTTP(w, r)
	})
}

func (s *UIServer) allowedHost(host string) bool {
	return host == fmt.Sprintf("127.0.0.1:%d", s.Port) || host == fmt.Sprintf("localhost:%d", s.Port)
}

func (s *UIServer) hostOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.allowedHost(r.Host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// guard enforces host, origin, token and content type on API requests.
func (s *UIServer) guard(next http.HandlerFunc) http.Handler {
	return s.hostOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && !s.allowedHost(strings.TrimPrefix(origin, "http://")) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		token := r.Header.Get("X-Installer-Token")
		if token == "" && r.Method == http.MethodGet {
			token = r.URL.Query().Get("t") // EventSource cannot send headers
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.Token)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		s.touch()
		if r.Method == http.MethodPost {
			mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if mt != "application/json" {
				http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		}
		next(w, r)
	}))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return false
	}
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// ── state & defaults ──────────────────────────────────────────────────────────

// StateResponse describes this machine for the UI.
type StateResponse struct {
	Platform       string   `json:"platform"`
	Installed      bool     `json:"installed"`
	InstallDir     string   `json:"installDir"`
	Version        string   `json:"version,omitempty"`
	AppPort        string   `json:"appPort,omitempty"`
	ServiceRunning bool     `json:"serviceRunning"`
	BundleVersion  string   `json:"bundleVersion"`
	UpgradeAction  string   `json:"upgradeAction,omitempty"` // "upgrade" | "repair" | "older"
	RollbackTo     string   `json:"rollbackTo,omitempty"`
	Backups        []string `json:"backups,omitempty"`
}

func (s *UIServer) installedDir(r *http.Request) string {
	if d := r.URL.Query().Get("dir"); d != "" {
		return filepath.Clean(d)
	}
	if s.Locate != nil {
		return findInstall(s.Locate(), s.Platform.DefaultInstallDir())
	}
	return s.Platform.DefaultInstallDir()
}

func (s *UIServer) handleState(w http.ResponseWriter, r *http.Request) {
	dir := s.installedDir(r)
	resp := StateResponse{Platform: s.Platform.Name(), InstallDir: dir, BundleVersion: bundleVersion(s.Bundle)}
	if st, found, err := LoadState(dir); err == nil && found {
		resp.Installed = true
		resp.Version = st.Version
		if env, err := ReadEnvFile(filepath.Join(dir, ".env")); err == nil {
			resp.AppPort = env.Get("PORT")
		}
		resp.ServiceRunning = resp.AppPort != "" && healthyQuick(resp.AppPort)
		switch c := compareVersions(resp.BundleVersion, st.Version); {
		case c > 0:
			resp.UpgradeAction = "upgrade"
		case c == 0:
			resp.UpgradeAction = "repair"
		default:
			resp.UpgradeAction = "older"
		}
		if _, bm, err := latestBackup(dir); err == nil && bm.ToVersion == st.Version {
			resp.RollbackTo = bm.FromVersion
		}
	}
	writeJSON(w, resp)
}

func healthyQuick(port string) bool {
	conn, err := net.Dial("tcp", "127.0.0.1:"+port)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// bundleVersion reads the version from the bundle without extracting it all.
func bundleVersion(b BundleSource) string {
	if b == nil {
		return ""
	}
	info, err := peekBundleInfo(b)
	if err != nil {
		return ""
	}
	return info.Version
}

func (s *UIServer) handleDefaults(w http.ResponseWriter, _ *http.Request) {
	c := Config{}
	c.Normalize()
	writeJSON(w, map[string]any{"config": c, "platform": s.Platform.Name(), "postgresToInstall": PostgresMajorToInstall})
}

func (s *UIServer) handleChecks(w http.ResponseWriter, r *http.Request) {
	var c Config
	if !decode(w, r, &c) {
		return
	}
	c.Normalize()
	writeJSON(w, s.Platform.Prerequisites(&c))
}

// ── long-running operations ───────────────────────────────────────────────────

func (s *UIServer) start(w http.ResponseWriter, op func(rep Reporter) (any, error)) {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		http.Error(w, "another operation is already running", http.StatusConflict)
		return
	}
	s.busy = true
	s.hub.Reset()
	s.mu.Unlock()

	rep := HubReporter{Hub: s.hub, Console: os.Stdout}
	go func() {
		defer func() {
			s.mu.Lock()
			s.busy = false
			s.mu.Unlock()
		}()
		result, err := op(rep)
		if err != nil {
			rep.Error("%v", err)
			s.hub.Finish("failed", err.Error())
			return
		}
		b, _ := json.Marshal(result)
		s.hub.Finish("done", string(b))
	}()
	w.WriteHeader(http.StatusAccepted)
}

func (s *UIServer) handleInstall(w http.ResponseWriter, r *http.Request) {
	var c Config
	if !decode(w, r, &c) {
		return
	}
	s.start(w, func(rep Reporter) (any, error) {
		return NewInstaller(s.Platform, s.Bundle, rep).Install(c)
	})
}

type dirRequest struct {
	InstallDir   string `json:"installDir"`
	Confirm      bool   `json:"confirm"`
	KeepDatabase bool   `json:"keepDatabase"`
}

func (s *UIServer) handleUpgrade(w http.ResponseWriter, r *http.Request) {
	var req dirRequest
	if !decode(w, r, &req) {
		return
	}
	s.start(w, func(rep Reporter) (any, error) {
		return NewInstaller(s.Platform, s.Bundle, rep).Upgrade(req.InstallDir)
	})
}

func (s *UIServer) handleRollback(w http.ResponseWriter, r *http.Request) {
	var req dirRequest
	if !decode(w, r, &req) {
		return
	}
	if !req.Confirm {
		http.Error(w, "rollback must be confirmed", http.StatusBadRequest)
		return
	}
	s.start(w, func(rep Reporter) (any, error) {
		return map[string]bool{"ok": true}, NewInstaller(s.Platform, s.Bundle, rep).Rollback(req.InstallDir)
	})
}

func (s *UIServer) handleUninstall(w http.ResponseWriter, r *http.Request) {
	var req dirRequest
	if !decode(w, r, &req) {
		return
	}
	if !req.Confirm {
		http.Error(w, "uninstall must be confirmed", http.StatusBadRequest)
		return
	}
	s.start(w, func(rep Reporter) (any, error) {
		return map[string]bool{"ok": true}, NewInstaller(s.Platform, s.Bundle, rep).Uninstall(req.InstallDir, req.KeepDatabase)
	})
}

func (s *UIServer) handleProgress(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Connection", "keep-alive")
	past, ch := s.hub.Subscribe()
	defer s.hub.Unsubscribe(ch)
	s.mu.Lock()
	s.streams++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.streams--
		s.lastSeen = time.Now()
		s.mu.Unlock()
	}()
	for _, e := range past {
		fmt.Fprintf(w, "data: %s\n\n", marshalEvent(e))
	}
	flusher.Flush()
	for {
		select {
		case e, open := <-ch:
			if !open {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", marshalEvent(e))
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// handleBrowse lists subdirectories for the folder picker.
func (s *UIServer) handleBrowse(w http.ResponseWriter, r *http.Request) {
	type entry struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	p := r.URL.Query().Get("path")
	if p == "" {
		if runtime.GOOS == "windows" {
			p = `C:\`
		} else {
			p = "/"
		}
	}
	p = filepath.Clean(p)
	res := struct {
		Path   string  `json:"path"`
		Parent string  `json:"parent,omitempty"`
		Dirs   []entry `json:"dirs"`
	}{Path: p, Dirs: []entry{}}
	if parent := filepath.Dir(p); parent != p {
		res.Parent = parent
	}
	if entries, err := os.ReadDir(p); err == nil {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && e.Name() != "$Recycle.Bin" && e.Name() != "System Volume Information" {
				res.Dirs = append(res.Dirs, entry{Name: e.Name(), Path: filepath.Join(p, e.Name())})
			}
		}
	}
	sort.Slice(res.Dirs, func(i, j int) bool { return strings.ToLower(res.Dirs[i].Name) < strings.ToLower(res.Dirs[j].Name) })
	writeJSON(w, res)
}

// ── wizard activity ───────────────────────────────────────────────────────────

func (s *UIServer) touch() {
	s.mu.Lock()
	s.lastSeen = time.Now()
	s.mu.Unlock()
}

// handlePing is the wizard page's heartbeat (it calls it every few seconds).
func (s *UIServer) handlePing(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// Idle reports whether the wizard has gone away: it connected at least once,
// then nothing has been heard from it for longer than limit, no progress
// stream is open and no operation is running. An install, upgrade or
// uninstall always finishes first, even if the window was closed.
func (s *UIServer) Idle(now time.Time, limit time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.lastSeen.IsZero() && !s.busy && s.streams == 0 && now.Sub(s.lastSeen) > limit
}

// ExitWhenIdle closes Done once the wizard has been gone for limit, so a
// setup process whose window was closed (and whose console may be hidden)
// does not keep running in the background.
func (s *UIServer) ExitWhenIdle(limit, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-s.Done:
			return
		case now := <-t.C:
			if s.Idle(now, limit) {
				s.doneOnce.Do(func() { close(s.Done) })
				return
			}
		}
	}
}

func (s *UIServer) handleClose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	s.doneOnce.Do(func() { close(s.Done) })
}

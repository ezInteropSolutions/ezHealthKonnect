package main

import (
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

//go:embed web
var webFS embed.FS

// version is injected at build time by the release workflow:
// go build -ldflags "-X main.version=v1.2.3"
var version string

// done is closed when the user clicks Finish / Close in the wizard.
var done = make(chan struct{})

func main() {
	// --uninstall flag: invoked by Add/Remove Programs — run headless uninstall
	// then open the browser on the progress page.
	for _, arg := range os.Args[1:] {
		if strings.EqualFold(arg, "--uninstall") {
			runHeadlessUninstall()
			return
		}
	}

	port := freePort()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	url := "http://" + addr

	mux := http.NewServeMux()
	registerHandlers(mux)

	// Serve embedded web assets
	webRoot, _ := fs.Sub(webFS, "web")
	mux.Handle("/", http.FileServer(http.FS(webRoot)))

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, "installer server error:", err)
			os.Exit(1)
		}
	}()

	// Give the server a moment to bind before opening the browser.
	time.Sleep(300 * time.Millisecond)

	fmt.Println("ezHealthKonnect Installer")
	fmt.Println("Opening browser at", url)
	fmt.Println("If the browser did not open, navigate to:", url)
	openBrowser(url)

	<-done
	fmt.Println("Installer finished.")
}

// runHeadlessUninstall opens the installer UI pre-navigated to the uninstall
// confirmation page, so the user sees progress (same as clicking Uninstall in the app).
func runHeadlessUninstall() {
	port := freePort()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	url := "http://" + addr + "/?uninstall=1"

	mux := http.NewServeMux()
	registerHandlers(mux)
	webRoot, _ := fs.Sub(webFS, "web")
	mux.Handle("/", http.FileServer(http.FS(webRoot)))

	srv := &http.Server{Addr: addr, Handler: mux}
	go srv.ListenAndServe() //nolint:errcheck
	time.Sleep(300 * time.Millisecond)
	openBrowser(url)
	<-done
}

func freePort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 7788
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// openBrowser opens the installer URL.
// On Windows we try to launch Edge or Chrome in --app mode (no browser chrome,
// fixed window size) so the installer looks like a native desktop wizard.
// Falls back to the system default browser if neither is found.
func openBrowser(url string) {
	if runtime.GOOS == "windows" {
		if openAppMode(url) {
			return
		}
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start() //nolint:errcheck
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start() //nolint:errcheck
}

// openAppMode tries to launch Edge or Chrome as a frameless app window.
// Returns true if a browser was launched successfully.
func openAppMode(url string) bool {
	appArgs := []string{
		"--app=" + url,
		"--window-size=980,680",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-extensions",
	}

	// Search paths for Edge then Chrome
	candidates := []string{}

	// Edge — installed per-machine and per-user
	for _, base := range []string{
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
		os.Getenv("LOCALAPPDATA"),
	} {
		if base != "" {
			candidates = append(candidates,
				filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"),
			)
		}
	}
	// Chrome — installed per-machine and per-user
	for _, base := range []string{
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
		os.Getenv("LOCALAPPDATA"),
	} {
		if base != "" {
			candidates = append(candidates,
				filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"),
			)
		}
	}

	for _, exe := range candidates {
		if _, err := os.Stat(exe); err != nil {
			continue
		}
		args := append([]string{exe}, appArgs...)
		cmd := exec.Command(args[0], args[1:]...)
		if cmd.Start() == nil {
			return true
		}
	}
	return false
}

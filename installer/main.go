package main

import (
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
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

func freePort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 7788
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start() //nolint:errcheck — best-effort
}

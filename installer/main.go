package main

// ezHealthKonnect installer (Standalone edition: Windows and Linux services).
//
// With no arguments it opens the setup wizard in a browser. Headless
// commands support servers without a desktop, scripted deployment and CI:
//
//	ezhealthkonnect-setup install   --config answers.json [--bundle app.zip]
//	ezhealthkonnect-setup upgrade   [--dir DIR] [--bundle app.zip]
//	ezhealthkonnect-setup rollback  [--dir DIR] --yes
//	ezhealthkonnect-setup uninstall [--dir DIR] --yes [--keep-database]
//	ezhealthkonnect-setup status    [--dir DIR]
//	ezhealthkonnect-setup version

import (
	"embed"
	"encoding/json"
	"flag"
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

// version is injected at build time: -ldflags "-X main.version=v26.10".
var version string

func main() {
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		os.Exit(runCommand(args[0], args[1:]))
	}
	uninstall := false
	for _, a := range args {
		if strings.EqualFold(a, "--uninstall") {
			uninstall = true
		}
	}
	os.Exit(runGUI(uninstall))
}

// bundleSource picks the application bundle: an explicit --bundle file, the
// bundle compiled into this installer, or a zip next to the installer.
func bundleSource(explicit string) BundleSource {
	if explicit != "" {
		return FileBundle{File: explicit}
	}
	if b := embeddedBundle(); len(b) > 0 {
		return EmbeddedBundle{Data: b}
	}
	if exe, err := os.Executable(); err == nil {
		side := filepath.Join(filepath.Dir(exe), fmt.Sprintf("ezhealthkonnect-%s-%s.zip", goosName(), runtime.GOARCH))
		if _, err := os.Stat(side); err == nil {
			return FileBundle{File: side}
		}
	}
	return nil
}

func runCommand(cmd string, args []string) int {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	p := newPlatform()
	dir := fs.String("dir", findInstall(installCandidates(), p.DefaultInstallDir()), "installation directory (default: the existing installation, found from the service)")
	bundle := fs.String("bundle", "", "application bundle zip (default: the one included in this installer)")
	configPath := fs.String("config", "", "answer file (JSON) for install")
	yes := fs.Bool("yes", false, "confirm a destructive operation")
	keepDB := fs.Bool("keep-database", false, "uninstall: keep the database")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rep := ConsoleReporter{Out: os.Stdout}
	needBundle := cmd == "install" || cmd == "upgrade"
	src := bundleSource(*bundle)
	if needBundle && src == nil {
		fmt.Fprintln(os.Stderr, "no application bundle: this installer does not include one — pass --bundle")
		return 2
	}
	in := NewInstaller(p, src, rep)

	var err error
	switch cmd {
	case "install":
		if *configPath == "" {
			fmt.Fprintln(os.Stderr, "install needs --config answers.json (see INSTALL.md)")
			return 2
		}
		var cfg Config
		data, rerr := os.ReadFile(*configPath)
		if rerr == nil {
			rerr = json.Unmarshal(data, &cfg)
		}
		if rerr != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", *configPath, rerr)
			return 2
		}
		var res *InstallResult
		if res, err = in.Install(cfg); err == nil {
			fmt.Printf("\nInstalled %s: %s\nCreate the first administrator (one-time link, keep it private):\n  %s\n", res.Version, res.AppURL, res.SetupURL)
		}
	case "upgrade":
		var res *InstallResult
		if res, err = in.Upgrade(*dir); err == nil {
			fmt.Printf("\nUpgraded to %s: %s\n", res.Version, res.AppURL)
		}
	case "rollback":
		if !*yes {
			fmt.Fprintln(os.Stderr, "rollback restores the database as it was before the last upgrade; data received since then is lost. Re-run with --yes to confirm.")
			return 2
		}
		err = in.Rollback(*dir)
	case "uninstall":
		if !*yes {
			fmt.Fprintln(os.Stderr, "uninstall removes the application and (unless --keep-database) its database. Re-run with --yes to confirm.")
			return 2
		}
		err = in.Uninstall(*dir, *keepDB)
	case "status":
		st, found, serr := LoadState(*dir)
		if serr != nil {
			err = serr
			break
		}
		if !found {
			fmt.Printf("ezHealthKonnect is not installed in %s\n", *dir)
			return 1
		}
		out, _ := json.MarshalIndent(st, "", "  ")
		fmt.Println(string(out))
	case "version":
		bv := ""
		if src != nil {
			bv = bundleVersion(src)
		}
		fmt.Printf("installer %s, application %s\n", version, bv)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q (install, upgrade, rollback, uninstall, status, version)\n", cmd)
		return 2
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nERROR: %v\n", err)
		return 1
	}
	return 0
}

func runGUI(uninstall bool) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot start the setup wizard:", err)
		return 1
	}
	port := l.Addr().(*net.TCPAddr).Port
	web, _ := fs.Sub(webFS, "web")
	srv := NewUIServer(newPlatform(), bundleSource(""), web, port)
	srv.Locate = installCandidates
	httpSrv := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go httpSrv.Serve(l) //nolint:errcheck

	extra := ""
	if uninstall {
		extra = "action=uninstall"
	}
	url := srv.URL(extra)
	fmt.Println("ezHealthKonnect Setup")
	fmt.Println("Open this address in a browser on this machine to continue:")
	fmt.Println("  " + url)
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		fmt.Println("\nNo desktop detected. Either forward the port over SSH:")
		fmt.Printf("  ssh -L %d:127.0.0.1:%d <this-server>   then open the address above locally\n", port, port)
		fmt.Println("or install without a browser:  sudo ./ezHealthKonnect-Setup-Linux-x64 install --config answers.json")
	} else if openBrowser(url) {
		// The wizard is open in its own window: drop the console window that
		// Windows opened for a double-clicked setup.exe (no-op from a terminal).
		releaseOwnConsole()
	}
	// Exit once the wizard window has been closed and nothing is running.
	go srv.ExitWhenIdle(wizardIdleLimit, 10*time.Second)
	<-srv.Done
	time.Sleep(300 * time.Millisecond)
	return 0
}

// wizardIdleLimit is how long setup waits after the wizard stops responding
// (window closed) before it exits. The page pings every 15 seconds.
const wizardIdleLimit = 2 * time.Minute

// openBrowser opens the wizard; on Windows as a frameless Edge/Chrome window.
// It reports whether that dedicated window was started (a plain default-
// browser hand-off cannot be confirmed).
func openBrowser(url string) bool {
	if runtime.GOOS == "windows" {
		for _, base := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), os.Getenv("LOCALAPPDATA")} {
			for _, rel := range []string{`Microsoft\Edge\Application\msedge.exe`, `Google\Chrome\Application\chrome.exe`} {
				exe := filepath.Join(base, rel)
				if base == "" {
					continue
				}
				if _, err := os.Stat(exe); err == nil {
					if exec.Command(exe, "--app="+url, "--window-size=1000,720", "--no-first-run").Start() == nil {
						return true
					}
				}
			}
		}
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start() //nolint:errcheck
		return false
	}
	exec.Command("xdg-open", url).Start() //nolint:errcheck
	return false
}

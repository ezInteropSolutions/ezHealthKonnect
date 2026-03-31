// services/schema/schema_package_manager.go
// SchemaPackageManager — download, install, and remove HL7/FHIR schema packs.
// Schema packs are hosted as GitHub Release assets on the public schema repo.
// The app ships with NO schemas bundled; users install only what they need.

package schema

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// schemaRegistryURL is the public JSON catalog listing all available schema packs.
// Hosted on GitHub Pages from the ezinteropsolutions/ezhealthkonnect-schemas repo.
const schemaRegistryURL = "https://ezInteropSolutions.github.io/ezhealthkonnect-schemas/catalog.json"

// PackageType categorises a schema pack.
type PackageType string

const (
	PackageTypeFHIR PackageType = "fhir" // FHIR R4, R5, R6 — JSON-based
	PackageTypeHL7  PackageType = "hl7"  // HL7 v2.x pipe-delimited — JSON dictionary
	PackageTypeCDA  PackageType = "cda"  // HL7 CDA / C-CDA / CCD — XSD-based
	PackageTypeXDS  PackageType = "xds"  // IHE XDS.b document sharing profiles
)

// InstallStatus is the current state of a schema pack on disk.
type InstallStatus string

const (
	StatusInstalled    InstallStatus = "installed"
	StatusNotInstalled InstallStatus = "not_installed"
	StatusDownloading  InstallStatus = "downloading"
)

// SchemaPackage describes one downloadable schema pack.
type SchemaPackage struct {
	ID          string      `json:"id"`           // e.g. "fhir-r4-base"
	Name        string      `json:"name"`         // e.g. "FHIR R4 Base Resources"
	Type        PackageType `json:"type"`         // "fhir" | "hl7"
	Version     string      `json:"version"`      // e.g. "4.0.1"
	Profile     string      `json:"profile"`      // e.g. "base", "us-core", "v2.5.1"
	Description string      `json:"description"`
	SizeBytes   int64       `json:"size_bytes"`
	DownloadURL string      `json:"download_url"` // GitHub Release asset URL
	InstallPath string      `json:"install_path"` // relative to schema root, e.g. "fhir/R4/resources"
	FileCount   int         `json:"file_count"`
	// Runtime fields (not in catalog JSON)
	Status        InstallStatus `json:"status"`
	InstalledAt   *time.Time    `json:"installed_at,omitempty"`
	InstalledSize int64         `json:"installed_size,omitempty"`
}

// DownloadProgress reports progress for an active download.
type DownloadProgress struct {
	PackageID   string  `json:"package_id"`
	BytesTotal  int64   `json:"bytes_total"`
	BytesDone   int64   `json:"bytes_done"`
	Percent     float64 `json:"percent"`
	Status      string  `json:"status"` // "downloading" | "extracting" | "done" | "error"
	ErrorMsg    string  `json:"error,omitempty"`
}

// SchemaPackageManager manages schema packs: catalog, install, remove.
type SchemaPackageManager struct {
	schemaRoot   string // absolute path to schemas/ directory
	distPath     string // optional local path containing pre-built *.zip files (dev mode)
	mu           sync.RWMutex
	progress     map[string]*DownloadProgress // keyed by package ID
	httpClient   *http.Client

	// Catalog cache — avoids re-reading disk + remote on every request
	cachedCatalog    []*SchemaPackage
	cacheExpiry      time.Time
	cacheDirSizes    map[string]int64 // package ID → installed size in bytes (lazy, cached)
}

// NewSchemaPackageManager creates a manager rooted at schemaRoot.
// distPath (optional) points to a local directory with pre-built <id>.zip files;
// when set, installs read from disk instead of downloading from GitHub.
const catalogCacheTTL = 60 * time.Second

func NewSchemaPackageManager(schemaRoot, distPath string) *SchemaPackageManager {
	return &SchemaPackageManager{
		schemaRoot:    schemaRoot,
		distPath:      distPath,
		progress:      make(map[string]*DownloadProgress),
		cacheDirSizes: make(map[string]int64),
		httpClient:    &http.Client{Timeout: 0}, // no timeout — large downloads
	}
}

// ----------------------------------------------------------------------------
// Catalog
// ----------------------------------------------------------------------------

// GetCatalog fetches the catalog and merges install status from disk.
// Priority: local catalog.json (distPath) → remote GitHub Pages → built-in fallback.
// Results are cached for catalogCacheTTL to avoid repeated disk/network I/O.
func (m *SchemaPackageManager) GetCatalog() ([]*SchemaPackage, error) {
	m.mu.RLock()
	if m.cachedCatalog != nil && time.Now().Before(m.cacheExpiry) {
		// Return a copy of the cached catalog with fresh install status
		// (install status is cheap — just os.Stat, no dirSize walk)
		cached := cloneCatalog(m.cachedCatalog)
		m.mu.RUnlock()
		m.mergeInstallStatus(cached, false)
		return cached, nil
	}
	m.mu.RUnlock()

	// Cache miss — fetch from source
	packages, err := m.fetchLocalCatalog()
	if err != nil {
		packages, err = m.fetchRemoteCatalog()
		if err != nil {
			packages = builtinCatalog()
		}
	}

	// Merge status including expensive dirSize (first load only)
	m.mergeInstallStatus(packages, true)

	m.mu.Lock()
	m.cachedCatalog = packages
	m.cacheExpiry = time.Now().Add(catalogCacheTTL)
	m.mu.Unlock()

	return cloneCatalog(packages), nil
}

// InvalidateCatalogCache forces the next GetCatalog call to re-fetch from source.
// Called after Install or Remove to reflect the new state immediately.
func (m *SchemaPackageManager) InvalidateCatalogCache() {
	m.mu.Lock()
	m.cachedCatalog = nil
	m.cacheExpiry = time.Time{}
	m.mu.Unlock()
}

// cloneCatalog returns a shallow copy of each package so callers can mutate
// Status/InstalledSize without affecting the cached originals.
func cloneCatalog(src []*SchemaPackage) []*SchemaPackage {
	out := make([]*SchemaPackage, len(src))
	for i, p := range src {
		cp := *p
		out[i] = &cp
	}
	return out
}

// fetchLocalCatalog reads catalog.json from the distPath directory (dev mode).
func (m *SchemaPackageManager) fetchLocalCatalog() ([]*SchemaPackage, error) {
	if m.distPath == "" {
		return nil, fmt.Errorf("no local dist path configured")
	}
	catalogPath := filepath.Join(m.distPath, "catalog.json")
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		return nil, fmt.Errorf("local catalog not found at %s: %w", catalogPath, err)
	}
	var packages []*SchemaPackage
	if err := json.Unmarshal(data, &packages); err != nil {
		return nil, fmt.Errorf("local catalog parse: %w", err)
	}
	return packages, nil
}

// GetInstalled returns only the packages currently installed on disk.
func (m *SchemaPackageManager) GetInstalled() ([]*SchemaPackage, error) {
	all, err := m.GetCatalog()
	if err != nil {
		return nil, err
	}
	var installed []*SchemaPackage
	for _, p := range all {
		if p.Status == StatusInstalled {
			installed = append(installed, p)
		}
	}
	return installed, nil
}

// fetchRemoteCatalog downloads catalog.json from the schema registry.
func (m *SchemaPackageManager) fetchRemoteCatalog() ([]*SchemaPackage, error) {
	resp, err := m.httpClient.Get(schemaRegistryURL)
	if err != nil {
		return nil, fmt.Errorf("schema catalog fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("schema catalog fetch: HTTP %d", resp.StatusCode)
	}

	var packages []*SchemaPackage
	if err := json.NewDecoder(resp.Body).Decode(&packages); err != nil {
		return nil, fmt.Errorf("schema catalog decode: %w", err)
	}
	return packages, nil
}

// mergeInstallStatus checks disk for each package and updates Status/InstalledAt.
// computeSize=true runs the expensive filepath.Walk and caches the result.
// computeSize=false uses the cached value (or 0 if not yet computed).
func (m *SchemaPackageManager) mergeInstallStatus(packages []*SchemaPackage, computeSize bool) {
	m.mu.RLock()
	progressSnapshot := make(map[string]*DownloadProgress, len(m.progress))
	for k, v := range m.progress {
		cp := *v
		progressSnapshot[k] = &cp
	}
	sizeCache := make(map[string]int64, len(m.cacheDirSizes))
	for k, v := range m.cacheDirSizes {
		sizeCache[k] = v
	}
	m.mu.RUnlock()

	for _, p := range packages {
		dir := filepath.Join(m.schemaRoot, filepath.FromSlash(p.InstallPath))
		info, err := os.Stat(dir)
		if err == nil && info.IsDir() {
			entries, _ := os.ReadDir(dir)
			if len(entries) > 0 {
				p.Status = StatusInstalled
				modTime := info.ModTime()
				p.InstalledAt = &modTime

				if computeSize {
					sz := dirSize(dir)
					m.mu.Lock()
					m.cacheDirSizes[p.ID] = sz
					m.mu.Unlock()
					p.InstalledSize = sz
				} else {
					p.InstalledSize = sizeCache[p.ID]
				}
				continue
			}
		}
		// Check if currently downloading
		if prog, ok := progressSnapshot[p.ID]; ok && prog.Status == "downloading" {
			p.Status = StatusDownloading
		} else {
			p.Status = StatusNotInstalled
		}
	}
}

// ----------------------------------------------------------------------------
// Install
// ----------------------------------------------------------------------------

// Install downloads and extracts a schema pack asynchronously.
// Progress can be polled via GetProgress(packageID).
func (m *SchemaPackageManager) Install(pkg *SchemaPackage) error {
	m.mu.Lock()
	if prog, ok := m.progress[pkg.ID]; ok && prog.Status == "downloading" {
		m.mu.Unlock()
		return fmt.Errorf("package %q is already downloading", pkg.ID)
	}
	m.progress[pkg.ID] = &DownloadProgress{
		PackageID:  pkg.ID,
		BytesTotal: pkg.SizeBytes,
		Status:     "downloading",
	}
	m.mu.Unlock()

	go m.installAsync(pkg)
	return nil
}

func (m *SchemaPackageManager) installAsync(pkg *SchemaPackage) {
	if err := m.doInstall(pkg); err != nil {
		m.setProgress(pkg.ID, func(p *DownloadProgress) {
			p.Status = "error"
			p.ErrorMsg = err.Error()
		})
	}
	// Invalidate catalog cache so next GetCatalog reflects installed state
	m.InvalidateCatalogCache()
}

func (m *SchemaPackageManager) doInstall(pkg *SchemaPackage) error {
	var zipData []byte

	// Prefer local dist file (dev mode) — skip network entirely
	// Check root first, then dist/ subdirectory (schema repo layout)
	if m.distPath != "" {
		localZip := filepath.Join(m.distPath, pkg.ID+".zip")
		if _, err := os.Stat(localZip); os.IsNotExist(err) {
			localZip = filepath.Join(m.distPath, "dist", pkg.ID+".zip")
		}
		if data, err := os.ReadFile(localZip); err == nil {
			zipData = data
			m.setProgress(pkg.ID, func(p *DownloadProgress) {
				p.BytesTotal = int64(len(data))
				p.BytesDone  = int64(len(data))
				p.Percent    = 100
			})
		}
	}

	// Fall back to remote download when local file not available
	if zipData == nil {
		resp, err := m.httpClient.Get(pkg.DownloadURL)
		if err != nil {
			return fmt.Errorf("download %q: %w", pkg.ID, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("download %q: HTTP %d", pkg.ID, resp.StatusCode)
		}

		total := resp.ContentLength
		m.setProgress(pkg.ID, func(p *DownloadProgress) {
			p.BytesTotal = total
		})

		buf := &bytes.Buffer{}
		counter := &progressReader{
			reader: resp.Body,
			onRead: func(n int64) {
				m.setProgress(pkg.ID, func(p *DownloadProgress) {
					p.BytesDone += n
					if p.BytesTotal > 0 {
						p.Percent = float64(p.BytesDone) / float64(p.BytesTotal) * 100
					}
				})
			},
		}
		if _, err := io.Copy(buf, counter); err != nil {
			return fmt.Errorf("download %q: read body: %w", pkg.ID, err)
		}
		zipData = buf.Bytes()
	}

	// Switch to extracting phase
	m.setProgress(pkg.ID, func(p *DownloadProgress) {
		p.Status = "extracting"
		p.Percent = 100
	})

	// Extract zip to install path
	destDir := filepath.Join(m.schemaRoot, filepath.FromSlash(pkg.InstallPath))
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("install %q: create dir: %w", pkg.ID, err)
	}
	if err := extractZip(zipData, destDir); err != nil {
		return fmt.Errorf("install %q: extract: %w", pkg.ID, err)
	}

	m.setProgress(pkg.ID, func(p *DownloadProgress) {
		p.Status = "done"
	})
	return nil
}

// ----------------------------------------------------------------------------
// Remove
// ----------------------------------------------------------------------------

// Remove deletes all files in a package's install path.
func (m *SchemaPackageManager) Remove(pkg *SchemaPackage) error {
	dir := filepath.Join(m.schemaRoot, filepath.FromSlash(pkg.InstallPath))
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove schema %q: %w", pkg.ID, err)
	}
	// Clear cached dir size for this package and invalidate catalog cache
	m.mu.Lock()
	delete(m.cacheDirSizes, pkg.ID)
	m.mu.Unlock()
	m.InvalidateCatalogCache()
	return nil
}

// ----------------------------------------------------------------------------
// Progress
// ----------------------------------------------------------------------------

// GetProgress returns the download progress for a package, or nil if not downloading.
func (m *SchemaPackageManager) GetProgress(packageID string) *DownloadProgress {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p := m.progress[packageID]
	if p == nil {
		return nil
	}
	copy := *p
	return &copy
}

func (m *SchemaPackageManager) setProgress(id string, fn func(*DownloadProgress)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.progress[id]; ok {
		fn(p)
	}
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

type progressReader struct {
	reader io.Reader
	onRead func(int64)
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.onRead(int64(n))
	}
	return n, err
}

func extractZip(data []byte, destDir string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		// Strip top-level directory from zip (if present)
		name := f.Name
		destPath := filepath.Join(destDir, filepath.FromSlash(name))

		// Prevent zip-slip
		if !isInsideDir(destDir, destPath) {
			return fmt.Errorf("zip-slip detected: %q", f.Name)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(destPath, 0755)
			continue
		}
		if err := extractZipFile(f, destPath); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(f *zip.File, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}

func isInsideDir(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && len(rel) >= 2 && rel[:2] != ".."
}

func dirSize(dir string) int64 {
	var size int64
	filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

// ----------------------------------------------------------------------------
// Built-in catalog (fallback when offline)
// ----------------------------------------------------------------------------

func builtinCatalog() []*SchemaPackage {
	return []*SchemaPackage{
		{
			ID:          "fhir-r4-base",
			Name:        "FHIR R4 — Base Resources",
			Type:        PackageTypeFHIR,
			Version:     "4.0.1",
			Profile:     "base",
			Description: "All 150 base FHIR R4 resource definitions. Required for any HL7→FHIR transformation.",
			SizeBytes:   18 * 1024 * 1024, // ~18 MB compressed
			DownloadURL: "https://github.com/ezinteropsolutions/ezhealthkonnect-schemas/releases/download/v1.0.0/fhir-r4-base.zip",
			InstallPath: "fhir/R4/resources",
			FileCount:   150,
		},
		{
			ID:          "fhir-r4-us-core",
			Name:        "FHIR R4 — US Core Profiles",
			Type:        PackageTypeFHIR,
			Version:     "6.1.0",
			Profile:     "us-core",
			Description: "US Core Implementation Guide profiles (ONC requirement for US-based deployments).",
			SizeBytes:   3 * 1024 * 1024, // ~3 MB compressed
			DownloadURL: "https://github.com/ezinteropsolutions/ezhealthkonnect-schemas/releases/download/v1.0.0/fhir-r4-us-core.zip",
			InstallPath: "fhir/R4/profiles/us-core",
			FileCount:   26,
		},
		{
			ID:          "fhir-r5-base",
			Name:        "FHIR R5 — Base Resources",
			Type:        PackageTypeFHIR,
			Version:     "5.0.0",
			Profile:     "base",
			Description: "FHIR R5 base resources. Only needed if your destination system requires R5.",
			SizeBytes:   22 * 1024 * 1024,
			DownloadURL: "https://github.com/ezinteropsolutions/ezhealthkonnect-schemas/releases/download/v1.0.0/fhir-r5-base.zip",
			InstallPath: "fhir/R5/resources",
			FileCount:   162,
		},
		{
			ID:          "hl7-v2-25",
			Name:        "HL7 v2.5 / v2.5.1 Dictionary",
			Type:        PackageTypeHL7,
			Version:     "2.5.1",
			Profile:     "v2.5.1",
			Description: "HL7 v2.5 and v2.5.1 segment and field definitions. Most common version in production.",
			SizeBytes:   8 * 1024 * 1024,
			DownloadURL: "https://github.com/ezinteropsolutions/ezhealthkonnect-schemas/releases/download/v1.0.0/hl7-v2-25.zip",
			InstallPath: "hl7/v2.5",
			FileCount:   45,
		},
		{
			ID:          "hl7-v2-27",
			Name:        "HL7 v2.7 / v2.8 Dictionary",
			Type:        PackageTypeHL7,
			Version:     "2.8",
			Profile:     "v2.8",
			Description: "HL7 v2.7 and v2.8 segment and field definitions.",
			SizeBytes:   10 * 1024 * 1024,
			DownloadURL: "https://github.com/ezinteropsolutions/ezhealthkonnect-schemas/releases/download/v1.0.0/hl7-v2-27.zip",
			InstallPath: "hl7/v2.7",
			FileCount:   52,
		},
		{
			ID:          "hl7-v2-23",
			Name:        "HL7 v2.3 / v2.4 Dictionary",
			Type:        PackageTypeHL7,
			Version:     "2.4",
			Profile:     "v2.4",
			Description: "HL7 v2.3 and v2.4 segment definitions. For legacy systems still on older HL7 versions.",
			SizeBytes:   6 * 1024 * 1024,
			DownloadURL: "https://github.com/ezinteropsolutions/ezhealthkonnect-schemas/releases/download/v1.0.0/hl7-v2-23.zip",
			InstallPath: "hl7/v2.3",
			FileCount:   38,
		},
	}
}

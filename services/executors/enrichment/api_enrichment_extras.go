package enrichment

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ezhealthkonnect/models"
	httpservice "ezhealthkonnect/services/http"
)

// applyHeaderOutputs copies the named response headers into the step's
// output, for a value the API returns in a header rather than the body (an
// asynchronous call's status address, in Content-Location). A header the
// response lacks is left out.
func applyHeaderOutputs(out map[string]interface{}, outputs map[string]string, resp *httpservice.Response) {
	for name, header := range outputs {
		if v := resp.Headers.Get(header); v != "" {
			out[name] = v
		}
	}
}

// saveResponseBody writes a response's body to a file instead of keeping it
// in the step's output and logs, for a download too large to carry in a
// message. The directory is created if needed; the file name is cut to its
// last element, so a name from the response or another system can never
// choose where it is written. The file appears all at once (written beside
// it, then renamed). It returns the path written and its size.
func saveResponseBody(cfg *models.APISaveToConfig, directory, fileName string, body []byte) (string, int, error) {
	name := filepath.Base(strings.ReplaceAll(strings.TrimSpace(fileName), "\\", "/"))
	if name == "" || name == "." || name == ".." || name == "/" {
		return "", 0, fmt.Errorf("saveTo: no usable file name (got %q)", fileName)
	}
	if strings.TrimSpace(directory) == "" {
		return "", 0, fmt.Errorf("saveTo: no directory")
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return "", 0, fmt.Errorf("saveTo: cannot create %s: %w", directory, err)
	}
	final := filepath.Join(directory, name)
	if !cfg.Overwrite {
		if _, err := os.Stat(final); err == nil {
			return "", 0, fmt.Errorf("saveTo: %s already exists", final)
		}
	}
	tmp, err := os.CreateTemp(directory, ".part-*")
	if err != nil {
		return "", 0, fmt.Errorf("saveTo: cannot write in %s: %w", directory, err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return "", 0, fmt.Errorf("saveTo: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", 0, fmt.Errorf("saveTo: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o640); err != nil {
		return "", 0, fmt.Errorf("saveTo: %w", err)
	}
	if err := os.Rename(tmp.Name(), final); err != nil {
		return "", 0, fmt.Errorf("saveTo: %w", err)
	}
	return final, len(body), nil
}

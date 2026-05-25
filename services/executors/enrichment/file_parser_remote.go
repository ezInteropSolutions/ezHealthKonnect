package enrichment

// ===============================================================
// FILE PARSER — REMOTE FILE RESOLUTION
// ===============================================================
// Resolves a URI stored in a pipeline field to raw file content.
// Scheme dispatch:
//   s3://bucket/key        → AWS S3 (credentials from interface connectivity config,
//                             or default AWS credential chain: env vars / IAM role)
//   https://...            → HTTP GET
//   http://...             → HTTP GET
//   file:///path or /path  → local filesystem (same as local_path mode)
//
// Credential strategy for S3 (containerisation-friendly):
//   1. Look up the interface's source_config in interface_connectivity table
//   2. If access_key_id + secret_access_key are present → use them explicitly
//   3. Otherwise fall back to the standard AWS credential chain
//      (AWS_ACCESS_KEY_ID env var, ~/.aws/credentials, ECS task role, EKS IRSA, etc.)
// ===============================================================

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	ezhmodels "ezhealthkonnect/models"
)

// resolveRemoteFile resolves a URI (stored in a pipeline field) to raw file bytes.
// Dispatches to the appropriate resolver based on URI scheme.
func (e *FileParserExecutor) resolveRemoteFile(
	ctx context.Context,
	uri string,
	config *ezhmodels.FileParserConfig,
	interfaceID string,
) (string, error) {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return "", fmt.Errorf("URI field is empty — the previous step must set a non-empty file URI")
	}

	log.Printf("   🌐 Resolving remote URI: %s", maskCredentialsInURI(uri))

	switch {
	case strings.HasPrefix(uri, "s3://"):
		return e.resolveS3File(ctx, uri, interfaceID)
	case strings.HasPrefix(uri, "https://") || strings.HasPrefix(uri, "http://"):
		return e.resolveHTTPFile(ctx, uri)
	case strings.HasPrefix(uri, "file://"):
		localPath := strings.TrimPrefix(uri, "file://")
		return e.resolveLocalFile(localPath, config)
	default:
		// Bare path (absolute Unix /path/to/file or Windows C:\path) — treat as local
		return e.resolveLocalFile(uri, config)
	}
}

// ── S3 ──────────────────────────────────────────────────────────────────────

// resolveS3File downloads a single S3 object and returns its contents as a string.
func (e *FileParserExecutor) resolveS3File(
	ctx context.Context,
	s3URI string,
	interfaceID string,
) (string, error) {
	bucket, key, err := parseS3URI(s3URI)
	if err != nil {
		return "", err
	}

	log.Printf("   ☁️  S3 download: bucket=%s  key=%s", bucket, key)

	client, err := e.buildS3Client(ctx, interfaceID)
	if err != nil {
		return "", fmt.Errorf("failed to build S3 client: %w", err)
	}

	dlCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	result, err := client.GetObject(dlCtx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return "", fmt.Errorf("S3 GetObject(bucket=%s key=%s) failed: %w", bucket, key, err)
	}
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read S3 object body: %w", err)
	}

	log.Printf("   ✅ S3 download complete: %d bytes", len(data))
	return string(data), nil
}

// buildS3Client creates an S3 client, preferring explicit credentials from the
// interface connectivity config. Falls back to the default AWS credential chain.
func (e *FileParserExecutor) buildS3Client(ctx context.Context, interfaceID string) (*s3.Client, error) {
	connCfg, region := e.loadS3ConnectivityConfig(ctx, interfaceID)

	var opts []func(*awsconfig.LoadOptions) error

	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
		log.Printf("   🔑 S3 region from connectivity config: %s", region)
	}

	if keyID, secret, ok := extractAWSCredentials(connCfg); ok {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(keyID, secret, ""),
		))
		log.Printf("   🔑 S3 using explicit credentials from interface connectivity config")
	} else {
		log.Printf("   🔑 S3 using default AWS credential chain (env vars / IAM role)")
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return s3.NewFromConfig(cfg), nil
}

// loadS3ConnectivityConfig fetches the interface's source connectivity config from the
// database and returns the parsed map along with the AWS region (if set).
func (e *FileParserExecutor) loadS3ConnectivityConfig(
	ctx context.Context,
	interfaceID string,
) (cfgMap map[string]interface{}, region string) {
	if e.db == nil || interfaceID == "" {
		return nil, ""
	}

	// Query the interface's enabled source connectivity config.
	// We don't filter by connector type — the executor trusts the pipeline designer
	// to point a field_as_path step at an interface configured with an S3 connector.
	query := `
		SELECT ic.source_config
		FROM   interface_connectivity ic
		WHERE  ic.interface_id   = $1
		  AND  ic.source_enabled = true
		LIMIT  1
	`

	var raw []byte
	if err := e.db.QueryRowContext(ctx, query, interfaceID).Scan(&raw); err != nil {
		if err != sql.ErrNoRows {
			log.Printf("   ⚠️  Could not load connectivity config for interface %s: %v", interfaceID, err)
		}
		return nil, ""
	}

	if len(raw) == 0 {
		return nil, ""
	}

	// Decrypt the config if a credential store decrypt function was provided
	if e.configDecrypt != nil {
		decrypted, err := e.configDecrypt(raw)
		if err != nil {
			log.Printf("   ⚠️  Failed to decrypt connectivity config for interface %s: %v", interfaceID, err)
			return nil, ""
		}
		raw = decrypted
	}

	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		log.Printf("   ⚠️  Failed to parse connectivity config JSON: %v", err)
		return nil, ""
	}

	region = firstStringValue(m, "region", "aws_region")
	return m, region
}

// extractAWSCredentials returns the access key ID and secret found in a connectivity config map.
func extractAWSCredentials(m map[string]interface{}) (keyID, secret string, ok bool) {
	if m == nil {
		return "", "", false
	}
	keyID = firstStringValue(m, "access_key_id", "aws_access_key_id", "accessKeyId")
	secret = firstStringValue(m, "secret_access_key", "aws_secret_access_key", "secretAccessKey")
	if keyID != "" && secret != "" {
		return keyID, secret, true
	}
	return "", "", false
}

// parseS3URI splits s3://bucket/key into (bucket, key, error).
func parseS3URI(uri string) (bucket, key string, err error) {
	path := strings.TrimPrefix(uri, "s3://")
	idx := strings.IndexByte(path, '/')
	if idx < 1 {
		return "", "", fmt.Errorf("invalid S3 URI (expected s3://bucket/key): %s", uri)
	}
	bucket = path[:idx]
	key = path[idx+1:]
	if key == "" {
		return "", "", fmt.Errorf("S3 URI has no object key (expected s3://bucket/key): %s", uri)
	}
	return bucket, key, nil
}

// ── HTTP / HTTPS ─────────────────────────────────────────────────────────────

// resolveHTTPFile performs a simple authenticated-free GET and returns the body.
func (e *FileParserExecutor) resolveHTTPFile(ctx context.Context, url string) (string, error) {
	log.Printf("   🌐 HTTP download: %s", url)

	dlCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to build HTTP request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP GET failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP GET returned %d for %s", resp.StatusCode, url)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read HTTP response: %w", err)
	}

	log.Printf("   ✅ HTTP download complete: %d bytes", len(data))
	return string(data), nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// firstStringValue returns the first non-empty string value for any of the given keys.
func firstStringValue(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// maskCredentialsInURI redacts embedded credentials from a URI for safe logging.
// e.g. https://user:pass@host/path → https://***@host/path
func maskCredentialsInURI(uri string) string {
	if atIdx := strings.Index(uri, "@"); atIdx > 0 {
		schemeEnd := strings.Index(uri, "://")
		if schemeEnd > 0 && schemeEnd < atIdx {
			return uri[:schemeEnd+3] + "***" + uri[atIdx:]
		}
	}
	return uri
}

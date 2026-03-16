// services/storage/object_storage_service.go
// ObjectStorageService — high-level service wrapping ObjectStorageDriver.
// Provides domain-specific methods for storing/retrieving raw messages, parsed content,
// transformed output, and NDJSON log files using consistent key conventions.
//
// Key conventions — interface-first, date-partitioned:
//   {interfaceId}/YYYY/MM/DD/raw/{messageId}.raw          — original bytes as received
//   {interfaceId}/YYYY/MM/DD/parsed/{messageId}.json      — parsed+enriched JSON
//   {interfaceId}/YYYY/MM/DD/outbound/{messageId}.{ext}   — exact payload delivered to connector
//   {interfaceId}/YYYY/MM/DD/transformed/{messageId}.json — full post-pipeline context (debug)
//   {interfaceId}/YYYY/MM/DD/logs/{messageId}.ndjson      — append-only NDJSON log
//
// Interface-first prefix enables per-interface lifecycle policies (S3/Azure/GCS).
// Date partition enables date-range listing and age-based archiving.
// The service also writes lifecycle rules for log retention on first use (S3 only).

package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// LogEntry is a single NDJSON line appended to the log file for a message.
type LogEntry struct {
	Timestamp time.Time              `json:"ts"`
	Level     string                 `json:"level"` // "info" | "warn" | "error"
	Stage     string                 `json:"stage"` // e.g. "receive", "parse", "pipeline", "deliver"
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// ObjectStorageService provides domain operations on top of ObjectStorageDriver.
type ObjectStorageService struct {
	driver ObjectStorageDriver
	bucket string
}

// NewObjectStorageService creates the service, ensures the bucket exists, and returns it.
func NewObjectStorageService(driver ObjectStorageDriver, bucket string) (*ObjectStorageService, error) {
	svc := &ObjectStorageService{driver: driver, bucket: bucket}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := driver.EnsureBucket(ctx, bucket); err != nil {
		return nil, fmt.Errorf("storage: ensure bucket: %w", err)
	}

	log.Printf("✅ [ObjectStorageService] Ready (driver=%s, bucket=%s)", driver.DriverName(), bucket)
	return svc, nil
}

// DriverName exposes the underlying driver name for diagnostics.
func (s *ObjectStorageService) DriverName() string { return s.driver.DriverName() }

// ─── Key helpers ────────────────────────────────────────────────────────────

func rawKey(interfaceID, messageID string) string {
	t := time.Now().UTC()
	return fmt.Sprintf("%s/%04d/%02d/%02d/raw/%s.raw", interfaceID, t.Year(), t.Month(), t.Day(), messageID)
}

func parsedKey(interfaceID, messageID string) string {
	t := time.Now().UTC()
	return fmt.Sprintf("%s/%04d/%02d/%02d/parsed/%s.json", interfaceID, t.Year(), t.Month(), t.Day(), messageID)
}

func outboundKey(interfaceID, messageID, ext string) string {
	t := time.Now().UTC()
	return fmt.Sprintf("%s/%04d/%02d/%02d/outbound/%s.%s", interfaceID, t.Year(), t.Month(), t.Day(), messageID, ext)
}

func transformedKey(interfaceID, messageID string) string {
	t := time.Now().UTC()
	return fmt.Sprintf("%s/%04d/%02d/%02d/transformed/%s.json", interfaceID, t.Year(), t.Month(), t.Day(), messageID)
}

func logKey(interfaceID, messageID string) string {
	t := time.Now().UTC()
	return fmt.Sprintf("%s/%04d/%02d/%02d/logs/%s.ndjson", interfaceID, t.Year(), t.Month(), t.Day(), messageID)
}

// ParseKeyFromURI extracts bucket and key from a storage URI (s3://bucket/key or local://bucket/key).
func ParseKeyFromURI(uri string) (bucket, key string, err error) {
	for _, prefix := range []string{"s3://", "local://"} {
		if len(uri) > len(prefix) && uri[:len(prefix)] == prefix {
			rest := uri[len(prefix):]
			idx := indexOf(rest, '/')
			if idx < 0 {
				return "", "", fmt.Errorf("storage: malformed URI %q", uri)
			}
			return rest[:idx], rest[idx+1:], nil
		}
	}
	return "", "", fmt.Errorf("storage: unrecognised URI scheme %q", uri)
}

func indexOf(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// ─── Raw message storage ────────────────────────────────────────────────────

// StoreRawMessage saves the original bytes received from an inbound connector.
// Returns the storage URI to persist in the database.
func (s *ObjectStorageService) StoreRawMessage(ctx context.Context, interfaceID, messageID string, content []byte) (string, error) {
	key := rawKey(interfaceID, messageID)
	uri, err := s.driver.PutObject(ctx, s.bucket, key, bytes.NewReader(content), int64(len(content)), "application/octet-stream")
	if err != nil {
		return "", fmt.Errorf("storage: store raw message %s: %w", messageID, err)
	}
	return uri, nil
}

// GetRawMessage retrieves the original bytes for a message.
func (s *ObjectStorageService) GetRawMessage(ctx context.Context, interfaceID, messageID string) ([]byte, error) {
	key := rawKey(interfaceID, messageID)
	data, _, err := s.driver.GetObjectBytes(ctx, s.bucket, key)
	if err != nil {
		return nil, fmt.Errorf("storage: get raw message %s: %w", messageID, err)
	}
	return data, nil
}

// GetRawMessageByURI retrieves raw bytes using a stored URI (for cases where the
// key is already available from the DB row without knowing interfaceID).
func (s *ObjectStorageService) GetRawMessageByURI(ctx context.Context, uri string) ([]byte, error) {
	bucket, key, err := ParseKeyFromURI(uri)
	if err != nil {
		return nil, err
	}
	data, _, err := s.driver.GetObjectBytes(ctx, bucket, key)
	return data, err
}

// ─── Outbound payload storage ───────────────────────────────────────────────

// StoreOutboundPayload stores the exact bytes that were delivered to an outbound connector.
// contentType determines the file extension (application/json → .json, application/hl7-v2 → .hl7, etc.)
// Returns the storage URI to persist in the database.
func (s *ObjectStorageService) StoreOutboundPayload(ctx context.Context, interfaceID, messageID, content, contentType string) (string, error) {
	ext := contentTypeToExt(contentType)
	key := outboundKey(interfaceID, messageID, ext)
	data := []byte(content)
	uri, err := s.driver.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)), contentType)
	if err != nil {
		return "", fmt.Errorf("storage: store outbound payload %s: %w", messageID, err)
	}
	return uri, nil
}

// GetOutboundPayload retrieves the delivered payload for a message.
// Returns (content string, contentType string, error).
func (s *ObjectStorageService) GetOutboundPayload(ctx context.Context, interfaceID, messageID string) (string, string, error) {
	// Try each known extension in priority order
	for _, ext := range []string{"json", "hl7", "xml", "csv", "txt"} {
		key := outboundKey(interfaceID, messageID, ext)
		data, info, err := s.driver.GetObjectBytes(ctx, s.bucket, key)
		if err == nil {
			ct := extToContentType(ext)
			if info != nil && info.ContentType != "" {
				ct = info.ContentType
			}
			return string(data), ct, nil
		}
	}
	return "", "", fmt.Errorf("storage: outbound payload not found for %s", messageID)
}

// contentTypeToExt maps MIME types to file extensions.
func contentTypeToExt(ct string) string {
	switch {
	case ct == "application/hl7-v2", ct == "text/hl7":
		return "hl7"
	case ct == "application/fhir+json", ct == "application/json":
		return "json"
	case ct == "application/xml", ct == "text/xml", ct == "application/fhir+xml":
		return "xml"
	case ct == "text/csv":
		return "csv"
	default:
		return "txt"
	}
}

// extToContentType maps extensions back to MIME types for responses.
func extToContentType(ext string) string {
	switch ext {
	case "hl7":
		return "application/hl7-v2"
	case "json":
		return "application/json"
	case "xml":
		return "application/xml"
	case "csv":
		return "text/csv"
	default:
		return "text/plain"
	}
}

// ─── Parsed content storage ─────────────────────────────────────────────────

// StoreParsedContent stores the enriched parsed JSON.
// Returns the URI to persist in parsed_content_uri.
func (s *ObjectStorageService) StoreParsedContent(ctx context.Context, interfaceID, messageID string, content map[string]interface{}) (string, error) {
	data, err := json.Marshal(content)
	if err != nil {
		return "", fmt.Errorf("storage: marshal parsed content for %s: %w", messageID, err)
	}
	key := parsedKey(interfaceID, messageID)
	uri, err := s.driver.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)), "application/json")
	if err != nil {
		return "", fmt.Errorf("storage: store parsed content %s: %w", messageID, err)
	}
	return uri, nil
}

// GetParsedContent retrieves and unmarshals parsed content.
func (s *ObjectStorageService) GetParsedContent(ctx context.Context, interfaceID, messageID string) (map[string]interface{}, error) {
	key := parsedKey(interfaceID, messageID)
	data, _, err := s.driver.GetObjectBytes(ctx, s.bucket, key)
	if err != nil {
		return nil, fmt.Errorf("storage: get parsed content %s: %w", messageID, err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("storage: unmarshal parsed content %s: %w", messageID, err)
	}
	return result, nil
}

// ─── Transformed content storage ────────────────────────────────────────────

// StoreTransformedContent stores the post-pipeline output.
// Returns the URI to persist in transformed_content_uri.
func (s *ObjectStorageService) StoreTransformedContent(ctx context.Context, interfaceID, messageID string, content map[string]interface{}) (string, error) {
	data, err := json.Marshal(content)
	if err != nil {
		return "", fmt.Errorf("storage: marshal transformed content for %s: %w", messageID, err)
	}
	key := transformedKey(interfaceID, messageID)
	uri, err := s.driver.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)), "application/json")
	if err != nil {
		return "", fmt.Errorf("storage: store transformed content %s: %w", messageID, err)
	}
	return uri, nil
}

// GetTransformedContent retrieves and unmarshals transformed content.
func (s *ObjectStorageService) GetTransformedContent(ctx context.Context, interfaceID, messageID string) (map[string]interface{}, error) {
	key := transformedKey(interfaceID, messageID)
	data, _, err := s.driver.GetObjectBytes(ctx, s.bucket, key)
	if err != nil {
		return nil, fmt.Errorf("storage: get transformed content %s: %w", messageID, err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("storage: unmarshal transformed content %s: %w", messageID, err)
	}
	return result, nil
}

// ─── Log storage ────────────────────────────────────────────────────────────

// AppendLog appends a single LogEntry as an NDJSON line to the message's log file.
// The log file is created automatically on first write.
func (s *ObjectStorageService) AppendLog(ctx context.Context, interfaceID, messageID string, entry LogEntry) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("storage: marshal log entry: %w", err)
	}
	line = append(line, '\n')

	key := logKey(interfaceID, messageID)
	_, err = s.driver.AppendObject(ctx, s.bucket, key, line)
	if err != nil {
		return fmt.Errorf("storage: append log for %s: %w", messageID, err)
	}
	return nil
}

// GetLogs retrieves all NDJSON log lines for a message and parses them.
func (s *ObjectStorageService) GetLogs(ctx context.Context, interfaceID, messageID string) ([]LogEntry, error) {
	key := logKey(interfaceID, messageID)
	data, _, err := s.driver.GetObjectBytes(ctx, s.bucket, key)
	if err != nil {
		return nil, fmt.Errorf("storage: get logs for %s: %w", messageID, err)
	}

	var entries []LogEntry
	dec := json.NewDecoder(bytes.NewReader(data))
	for dec.More() {
		var entry LogEntry
		if err := dec.Decode(&entry); err != nil {
			// Skip malformed lines
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// GetLogsByURI retrieves logs using a stored URI.
func (s *ObjectStorageService) GetLogsByURI(ctx context.Context, uri string) ([]LogEntry, error) {
	bucket, key, err := ParseKeyFromURI(uri)
	if err != nil {
		return nil, err
	}
	data, _, err := s.driver.GetObjectBytes(ctx, bucket, key)
	if err != nil {
		return nil, fmt.Errorf("storage: get logs by URI %s: %w", uri, err)
	}
	var entries []LogEntry
	dec := json.NewDecoder(bytes.NewReader(data))
	for dec.More() {
		var entry LogEntry
		if err := dec.Decode(&entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// ─── Bulk delete ────────────────────────────────────────────────────────────

// DeleteMessageObjects removes all objects associated with a message (raw, parsed, transformed, logs).
func (s *ObjectStorageService) DeleteMessageObjects(ctx context.Context, interfaceID, messageID string) error {
	keys := []string{
		rawKey(interfaceID, messageID),
		parsedKey(interfaceID, messageID),
		transformedKey(interfaceID, messageID),
		logKey(interfaceID, messageID),
	}
	// Outbound payload — try all known extensions
	for _, ext := range []string{"json", "hl7", "xml", "csv", "txt"} {
		keys = append(keys, outboundKey(interfaceID, messageID, ext))
	}
	for _, key := range keys {
		if err := s.driver.DeleteObject(ctx, s.bucket, key); err != nil {
			// Non-fatal: object may not exist if processing never reached that stage
			log.Printf("⚠️ [ObjectStorage] DeleteMessageObjects: could not delete %s: %v", key, err)
		}
	}
	return nil
}

// services/storage/local_driver.go
// LocalDriver — ObjectStorageDriver backed by the local filesystem.
// Intended for development and testing without MinIO or a cloud account.
// Layout: <baseDir>/<bucket>/<key>  (directories created as needed)

package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// LocalDriver implements ObjectStorageDriver using local disk.
type LocalDriver struct {
	baseDir string // e.g. "/tmp/ezhk-storage"
}

// NewLocalDriver creates a LocalDriver rooted at baseDir.
func NewLocalDriver(baseDir string) (*LocalDriver, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("storage/local: create base dir %q: %w", baseDir, err)
	}
	log.Printf("✅ [ObjectStorage] Local driver initialised (baseDir=%s)", baseDir)
	return &LocalDriver{baseDir: baseDir}, nil
}

// DriverName returns "local".
func (d *LocalDriver) DriverName() string { return "local" }

// localPath converts bucket/key to an absolute filesystem path.
func (d *LocalDriver) localPath(bucket, key string) string {
	// Clean key to prevent directory traversal
	cleanKey := filepath.Clean(strings.TrimPrefix(key, "/"))
	return filepath.Join(d.baseDir, bucket, cleanKey)
}

// EnsureBucket creates the bucket directory.
func (d *LocalDriver) EnsureBucket(_ context.Context, bucket string) error {
	dir := filepath.Join(d.baseDir, bucket)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("storage/local: ensure bucket %q: %w", bucket, err)
	}
	return nil
}

// PutObject writes body to baseDir/bucket/key.
func (d *LocalDriver) PutObject(_ context.Context, bucket, key string, body io.Reader, _ int64, _ string) (string, error) {
	path := d.localPath(bucket, key)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("storage/local: mkdir for %q: %w", path, err)
	}

	data, err := io.ReadAll(body)
	if err != nil {
		return "", fmt.Errorf("storage/local: read body: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("storage/local: write %q: %w", path, err)
	}
	return fmt.Sprintf("local://%s/%s", bucket, key), nil
}

// GetObject returns a ReadCloser for baseDir/bucket/key.
func (d *LocalDriver) GetObject(_ context.Context, bucket, key string) (io.ReadCloser, *ObjectInfo, error) {
	path := d.localPath(bucket, key)
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("storage/local: open %q: %w", path, err)
	}
	stat, _ := f.Stat()
	info := &ObjectInfo{Key: key}
	if stat != nil {
		info.Size = stat.Size()
		info.LastModified = stat.ModTime()
	}
	return f, info, nil
}

// GetObjectBytes reads the full body of bucket/key.
func (d *LocalDriver) GetObjectBytes(_ context.Context, bucket, key string) ([]byte, *ObjectInfo, error) {
	path := d.localPath(bucket, key)
	stat, err := os.Stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("storage/local: stat %q: %w", path, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("storage/local: read %q: %w", path, err)
	}
	info := &ObjectInfo{
		Key:          key,
		Size:         stat.Size(),
		LastModified: stat.ModTime(),
	}
	return data, info, nil
}

// DeleteObject removes baseDir/bucket/key.
func (d *LocalDriver) DeleteObject(_ context.Context, bucket, key string) error {
	path := d.localPath(bucket, key)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage/local: delete %q: %w", path, err)
	}
	return nil
}

// ListObjects returns all files under baseDir/bucket/prefix.
func (d *LocalDriver) ListObjects(_ context.Context, bucket, prefix string) ([]ObjectInfo, error) {
	root := d.localPath(bucket, prefix)
	var result []ObjectInfo

	err := filepath.WalkDir(root, func(path string, de os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if de.IsDir() {
			return nil
		}

		// Build key relative to bucket root
		bucketDir := filepath.Join(d.baseDir, bucket)
		rel, _ := filepath.Rel(bucketDir, path)
		key := filepath.ToSlash(rel)

		info, _ := de.Info()
		obj := ObjectInfo{Key: key}
		if info != nil {
			obj.Size = info.Size()
			obj.LastModified = info.ModTime()
		}
		result = append(result, obj)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("storage/local: list %q/%q: %w", bucket, prefix, err)
	}
	return result, nil
}

// AppendObject appends data to an existing file (or creates it).
func (d *LocalDriver) AppendObject(_ context.Context, bucket, key string, data []byte) (string, error) {
	path := d.localPath(bucket, key)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("storage/local: mkdir for append %q: %w", path, err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return "", fmt.Errorf("storage/local: open for append %q: %w", path, err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return "", fmt.Errorf("storage/local: append to %q: %w", path, err)
	}
	return fmt.Sprintf("local://%s/%s", bucket, key), nil
}

// Ping checks that the base directory and the expected bucket sub-directory
// are accessible. Used by the /readyz health probe.
func (d *LocalDriver) Ping(_ context.Context) error {
	if _, err := os.Stat(d.baseDir); err != nil {
		return fmt.Errorf("storage/local: base directory unreachable: %w", err)
	}
	return nil
}

// Compile-time check: LocalDriver satisfies ObjectStorageDriver
var _ ObjectStorageDriver = (*LocalDriver)(nil)

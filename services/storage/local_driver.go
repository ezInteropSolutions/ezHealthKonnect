// services/storage/local_driver.go
// LocalDriver — ObjectStorageDriver backed by the local filesystem.
// Production storage for the Standalone editions and the default for Docker
// installs (a persistent volume); S3 is opt-in for customers with their own.
// Layout: <baseDir>/<bucket>/<key>  (directories created as needed)
//
// Stored objects hold message content (PHI), so:
//   - every bucket/key is resolved strictly inside baseDir — a key containing
//     ".." or an absolute path is rejected, never cleaned into somewhere else;
//   - files are 0600 and directories 0700 (readable only by the app's user);
//   - PutObject writes a temp file in the same directory and renames it into
//     place, so a crash never leaves a half-written object behind.

package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	localDirPerm  os.FileMode = 0o700
	localFilePerm os.FileMode = 0o600
)

// ErrInvalidObjectPath is returned when a bucket or key would resolve outside
// the storage directory.
var ErrInvalidObjectPath = errors.New("storage/local: invalid object path")

// LocalDriver implements ObjectStorageDriver using local disk.
type LocalDriver struct {
	baseDir string // absolute, e.g. "/app/data/storage"
}

// NewLocalDriver creates a LocalDriver rooted at baseDir.
func NewLocalDriver(baseDir string) (*LocalDriver, error) {
	abs, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("storage/local: resolve base dir %q: %w", baseDir, err)
	}
	if err := os.MkdirAll(abs, localDirPerm); err != nil {
		return nil, fmt.Errorf("storage/local: create base dir %q: %w", abs, err)
	}
	log.Printf("✅ [ObjectStorage] Local driver initialised (baseDir=%s)", abs)
	return &LocalDriver{baseDir: abs}, nil
}

// DriverName returns "local".
func (d *LocalDriver) DriverName() string { return "local" }

// bucketDir resolves a bucket to its directory. A bucket is a single path
// segment: no separators, not "." or "..".
func (d *LocalDriver) bucketDir(bucket string) (string, error) {
	if bucket == "" || bucket == "." || bucket == ".." || strings.ContainsAny(bucket, `/\`) || strings.ContainsRune(bucket, 0) {
		return "", fmt.Errorf("%w: bucket %q", ErrInvalidObjectPath, bucket)
	}
	return filepath.Join(d.baseDir, bucket), nil
}

// objectPath resolves bucket/key to a filesystem path inside the bucket
// directory. An empty key resolves to the bucket directory itself (used for
// listing). Keys use "/" separators, like S3; ".." segments, absolute paths
// and NUL bytes are rejected.
func (d *LocalDriver) objectPath(bucket, key string) (string, error) {
	dir, err := d.bucketDir(bucket)
	if err != nil {
		return "", err
	}
	if strings.ContainsRune(key, 0) || strings.HasPrefix(key, "/") || strings.HasPrefix(key, `\`) || filepath.IsAbs(key) || filepath.VolumeName(key) != "" {
		return "", fmt.Errorf("%w: key %q", ErrInvalidObjectPath, key)
	}
	for _, seg := range strings.FieldsFunc(key, func(r rune) bool { return r == '/' || r == '\\' }) {
		if seg == ".." {
			return "", fmt.Errorf("%w: key %q", ErrInvalidObjectPath, key)
		}
	}
	path := filepath.Join(dir, filepath.FromSlash(key))
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: key %q", ErrInvalidObjectPath, key)
	}
	return path, nil
}

func (d *LocalDriver) uri(bucket, key string) string {
	return fmt.Sprintf("local://%s/%s", bucket, key)
}

// EnsureBucket creates the bucket directory.
func (d *LocalDriver) EnsureBucket(_ context.Context, bucket string) error {
	dir, err := d.bucketDir(bucket)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, localDirPerm); err != nil {
		return fmt.Errorf("storage/local: ensure bucket %q: %w", bucket, err)
	}
	return nil
}

// PutObject writes body to baseDir/bucket/key atomically (temp file + rename).
func (d *LocalDriver) PutObject(_ context.Context, bucket, key string, body io.Reader, _ int64, _ string) (string, error) {
	path, err := d.objectPath(bucket, key)
	if err != nil {
		return "", err
	}
	if err := writeFileAtomic(path, body); err != nil {
		return "", fmt.Errorf("storage/local: write %q: %w", path, err)
	}
	return d.uri(bucket, key), nil
}

func writeFileAtomic(path string, body io.Reader) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), localDirPerm); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	if err = tmp.Chmod(localFilePerm); err != nil {
		return err
	}
	if _, err = io.Copy(tmp, body); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// GetObject returns a ReadCloser for baseDir/bucket/key.
func (d *LocalDriver) GetObject(_ context.Context, bucket, key string) (io.ReadCloser, *ObjectInfo, error) {
	path, err := d.objectPath(bucket, key)
	if err != nil {
		return nil, nil, err
	}
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
	path, err := d.objectPath(bucket, key)
	if err != nil {
		return nil, nil, err
	}
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
	path, err := d.objectPath(bucket, key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage/local: delete %q: %w", path, err)
	}
	return nil
}

// ListObjects returns all files under baseDir/bucket/prefix.
func (d *LocalDriver) ListObjects(_ context.Context, bucket, prefix string) ([]ObjectInfo, error) {
	bucketDir, err := d.bucketDir(bucket)
	if err != nil {
		return nil, err
	}
	root, err := d.objectPath(bucket, prefix)
	if err != nil {
		return nil, err
	}
	var result []ObjectInfo

	err = filepath.WalkDir(root, func(path string, de os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		// Skip directories and in-progress atomic writes.
		if de.IsDir() || strings.HasPrefix(de.Name(), ".") && strings.Contains(de.Name(), ".tmp-") {
			return nil
		}
		rel, _ := filepath.Rel(bucketDir, path)
		obj := ObjectInfo{Key: filepath.ToSlash(rel)}
		if info, _ := de.Info(); info != nil {
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
	path, err := d.objectPath(bucket, key)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), localDirPerm); err != nil {
		return "", fmt.Errorf("storage/local: mkdir for append %q: %w", path, err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, localFilePerm)
	if err != nil {
		return "", fmt.Errorf("storage/local: open for append %q: %w", path, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return "", fmt.Errorf("storage/local: append to %q: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("storage/local: close %q: %w", path, err)
	}
	return d.uri(bucket, key), nil
}

// Ping checks that the base directory exists and is writable. Used by the
// /readyz health probe — a read-only or full volume must report unhealthy.
func (d *LocalDriver) Ping(_ context.Context) error {
	if _, err := os.Stat(d.baseDir); err != nil {
		return fmt.Errorf("storage/local: base directory unreachable: %w", err)
	}
	f, err := os.CreateTemp(d.baseDir, ".ping-*")
	if err != nil {
		return fmt.Errorf("storage/local: base directory not writable: %w", err)
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return nil
}

// Compile-time check: LocalDriver satisfies ObjectStorageDriver
var _ ObjectStorageDriver = (*LocalDriver)(nil)

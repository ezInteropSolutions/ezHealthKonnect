package storage

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func newTestLocalDriver(t *testing.T) *LocalDriver {
	t.Helper()
	d, err := NewLocalDriver(filepath.Join(t.TempDir(), "storage"))
	if err != nil {
		t.Fatalf("NewLocalDriver: %v", err)
	}
	return d
}

func TestLocalDriver_RoundTripAndURI(t *testing.T) {
	d := newTestLocalDriver(t)
	ctx := context.Background()
	uri, err := d.PutObject(ctx, "ezhk", "raw/intf-1/2026/10/20/msg-1.hl7", strings.NewReader("MSH|^~\\&|A"), -1, "text/plain")
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	if uri != "local://ezhk/raw/intf-1/2026/10/20/msg-1.hl7" {
		t.Fatalf("uri = %q", uri)
	}
	b, _, err := d.GetObjectBytes(ctx, "ezhk", "raw/intf-1/2026/10/20/msg-1.hl7")
	if err != nil || string(b) != "MSH|^~\\&|A" {
		t.Fatalf("GetObjectBytes = %q, %v", b, err)
	}
	// Existing URIs (s3:// from an earlier S3 deployment) resolve through ParseKeyFromURI.
	bucket, key, err := ParseKeyFromURI(uri)
	if err != nil || bucket != "ezhk" || key != "raw/intf-1/2026/10/20/msg-1.hl7" {
		t.Fatalf("ParseKeyFromURI = %q %q %v", bucket, key, err)
	}
}

func TestLocalDriver_RejectsPathsOutsideStorage(t *testing.T) {
	d := newTestLocalDriver(t)
	ctx := context.Background()
	outside := filepath.Join(filepath.Dir(d.baseDir), "escaped.txt")

	badKeys := []string{"../escaped.txt", "a/../../escaped.txt", "/etc/passwd", "..", "a/..", "x\x00y"}
	if runtime.GOOS == "windows" {
		badKeys = append(badKeys, `..\escaped.txt`, `C:\Windows\x`)
	}
	for _, key := range badKeys {
		if _, err := d.PutObject(ctx, "ezhk", key, strings.NewReader("x"), 1, ""); !errors.Is(err, ErrInvalidObjectPath) {
			t.Errorf("PutObject(%q) err = %v, want ErrInvalidObjectPath", key, err)
		}
		if _, _, err := d.GetObjectBytes(ctx, "ezhk", key); !errors.Is(err, ErrInvalidObjectPath) {
			t.Errorf("GetObjectBytes(%q) err = %v, want ErrInvalidObjectPath", key, err)
		}
		if err := d.DeleteObject(ctx, "ezhk", key); !errors.Is(err, ErrInvalidObjectPath) {
			t.Errorf("DeleteObject(%q) err = %v, want ErrInvalidObjectPath", key, err)
		}
		if _, err := d.AppendObject(ctx, "ezhk", key, []byte("x")); !errors.Is(err, ErrInvalidObjectPath) {
			t.Errorf("AppendObject(%q) err = %v, want ErrInvalidObjectPath", key, err)
		}
	}
	for _, bucket := range []string{"", ".", "..", "a/b", `a\b`} {
		if _, err := d.PutObject(ctx, bucket, "k", strings.NewReader("x"), 1, ""); !errors.Is(err, ErrInvalidObjectPath) {
			t.Errorf("PutObject(bucket %q) err = %v, want ErrInvalidObjectPath", bucket, err)
		}
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("a file was written outside the storage directory")
	}
}

func TestLocalDriver_FilesArePrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}
	d := newTestLocalDriver(t)
	ctx := context.Background()
	if _, err := d.PutObject(ctx, "ezhk", "a/b.txt", strings.NewReader("phi"), 3, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AppendObject(ctx, "ezhk", "logs/l.jsonl", []byte("x\n")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"ezhk/a/b.txt", "ezhk/logs/l.jsonl"} {
		st, _ := os.Stat(filepath.Join(d.baseDir, p))
		if st.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %v, want 0600", p, st.Mode().Perm())
		}
	}
	for _, p := range []string{"ezhk", "ezhk/a"} {
		st, _ := os.Stat(filepath.Join(d.baseDir, p))
		if st.Mode().Perm() != 0o700 {
			t.Errorf("%s mode = %v, want 0700", p, st.Mode().Perm())
		}
	}
}

type failingReader struct{ n int }

func (r *failingReader) Read(p []byte) (int, error) {
	if r.n == 0 {
		r.n++
		return copy(p, "partial"), nil
	}
	return 0, errors.New("disk on fire")
}

func TestLocalDriver_FailedWriteLeavesNoPartialObject(t *testing.T) {
	d := newTestLocalDriver(t)
	ctx := context.Background()
	if _, err := d.PutObject(ctx, "ezhk", "k.txt", strings.NewReader("original"), 8, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := d.PutObject(ctx, "ezhk", "k.txt", &failingReader{}, -1, ""); err == nil {
		t.Fatal("expected write error")
	}
	b, _, err := d.GetObjectBytes(ctx, "ezhk", "k.txt")
	if err != nil || string(b) != "original" {
		t.Fatalf("object after failed overwrite = %q, %v (want the original, untouched)", b, err)
	}
	objs, _ := d.ListObjects(ctx, "ezhk", "")
	if len(objs) != 1 {
		t.Fatalf("ListObjects = %+v, want only k.txt (no temp files)", objs)
	}
}

func TestLocalDriver_ListAppendDelete(t *testing.T) {
	d := newTestLocalDriver(t)
	ctx := context.Background()
	for _, k := range []string{"raw/i1/m1", "raw/i1/m2", "raw/i2/m3"} {
		if _, err := d.PutObject(ctx, "ezhk", k, bytes.NewReader([]byte(k)), int64(len(k)), ""); err != nil {
			t.Fatal(err)
		}
	}
	objs, err := d.ListObjects(ctx, "ezhk", "raw/i1")
	if err != nil || len(objs) != 2 {
		t.Fatalf("ListObjects = %+v, %v", objs, err)
	}
	for _, o := range objs {
		if !strings.HasPrefix(o.Key, "raw/i1/") {
			t.Errorf("key %q not relative to bucket", o.Key)
		}
	}
	if none, err := d.ListObjects(ctx, "ezhk", "missing"); err != nil || len(none) != 0 {
		t.Fatalf("ListObjects(missing) = %+v, %v", none, err)
	}

	d.AppendObject(ctx, "ezhk", "logs/x", []byte("a"))
	d.AppendObject(ctx, "ezhk", "logs/x", []byte("b"))
	if b, _, _ := d.GetObjectBytes(ctx, "ezhk", "logs/x"); string(b) != "ab" {
		t.Fatalf("appended = %q", b)
	}

	if err := d.DeleteObject(ctx, "ezhk", "raw/i1/m1"); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteObject(ctx, "ezhk", "raw/i1/m1"); err != nil {
		t.Fatalf("deleting a missing object should be a no-op: %v", err)
	}
}

func TestLocalDriver_PingDetectsUnwritableDirectory(t *testing.T) {
	d := newTestLocalDriver(t)
	if err := d.Ping(context.Background()); err != nil {
		t.Fatalf("Ping on a healthy dir: %v", err)
	}
	if err := os.RemoveAll(d.baseDir); err != nil {
		t.Fatal(err)
	}
	if err := d.Ping(context.Background()); err == nil {
		t.Fatal("Ping should fail when the storage directory is gone")
	}
}

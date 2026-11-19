package connectors

import "testing"

func TestS3UsePathStyle(t *testing.T) {
	for _, c := range []struct {
		endpoint string
		forced   bool
		want     bool
	}{{"", false, false}, {"", true, true}, {"http://minio:9000", false, true}, {"http://minio:9000", true, true}} {
		if got := s3UsePathStyle(c.endpoint, c.forced); got != c.want {
			t.Errorf("endpoint %q forced %v: %v, want %v", c.endpoint, c.forced, got, c.want)
		}
	}
}

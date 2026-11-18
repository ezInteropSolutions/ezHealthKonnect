package processing

import (
	"testing"

	"ezhealthkonnect/models"
)

func TestSourceFileName(t *testing.T) {
	cases := []struct {
		name string
		msg  models.InboundMessage
		want string
	}{
		{"file listener", models.InboundMessage{Headers: map[string]string{"File-Name": "a.csv"}}, "a.csv"},
		{"s3 key keeps only the object name", models.InboundMessage{Headers: map[string]string{"S3-Key": "in/2026/b.csv"}}, "b.csv"},
		{"sftp", models.InboundMessage{SourceMetadata: map[string]string{"sftp_filename": " c.csv "}}, "c.csv"},
		{"file name wins", models.InboundMessage{Headers: map[string]string{"File-Name": "a.csv", "S3-Key": "x/b.csv"}}, "a.csv"},
		{"not a file", models.InboundMessage{}, ""},
	}
	for _, c := range cases {
		if got := sourceFileName(&c.msg); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

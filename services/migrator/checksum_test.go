package migrator

import (
	"bytes"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const realMigrationsDir = "../../database/migrations"

// Checksums recorded by real Flyway runs in flyway_schema_history. V155 is
// checked out with CRLF line endings on Windows and LF on Linux/CI — Flyway's
// line-based checksum must give the same value for both.
var realFlywayChecksums = map[string]int32{
	"V62__Comprehensive_HL7_FHIR_Templates.sql":            883321849,  // largest file
	"V71__Remove_All_OOB_Templates.sql":                    -375200022, // smallest, negative int32
	"V105__Template_V11_Context_Links.sql":                 1525720345, // has its own BEGIN/COMMIT
	"V155__CDA_Declarative_Mapping_Rules_Observations.sql": 53991740,
}

func TestChecksum_MatchesRealFlywayHistory(t *testing.T) {
	for name, want := range realFlywayChecksums {
		data, err := os.ReadFile(filepath.Join(realMigrationsDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, variant := range []struct {
			label string
			data  []byte
		}{
			{"as checked out", data},
			{"LF", bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))},
			{"CRLF", bytes.ReplaceAll(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")), []byte("\n"), []byte("\r\n"))},
		} {
			got, err := Checksum(bytes.NewReader(variant.data))
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Errorf("%s (%s): checksum %d, Flyway recorded %d", name, variant.label, got, want)
			}
		}
	}
}

func TestChecksum_LineSemantics(t *testing.T) {
	lines := func(ls ...string) int32 { return int32(crc32.ChecksumIEEE([]byte(strings.Join(ls, "")))) }
	cases := []struct {
		in   string
		want int32
	}{
		{"", 0},
		{"a\nb\n", lines("a", "b")},
		{"a\r\nb", lines("a", "b")},
		{"a\rb\r", lines("a", "b")},           // lone CR is a line terminator (Java readLine)
		{"\uFEFFa\nb", lines("a", "b")},       // BOM on the first line is ignored
		{"a\n\uFEFFb", lines("a", "\uFEFFb")}, // ...but only on the first line
		{"\n\n", 0},
	}
	for _, c := range cases {
		got, err := Checksum(strings.NewReader(c.in))
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("Checksum(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

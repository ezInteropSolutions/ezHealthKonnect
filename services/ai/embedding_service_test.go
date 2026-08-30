// services/ai/embedding_service_test.go
// Regression coverage for the chunkText rune-boundary bug: a byte-offset chunk
// boundary landing inside a multi-byte UTF-8 character (em dash, curly quote,
// checkmark, box-drawing, etc.) produced a chunk that was invalid UTF-8 on its
// own, which Postgres then rejected on insert — silently truncating every
// chunk after the failure (see IngestText).
package ai

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

// assertAllChunksValidUTF8 fails the test if any chunk chunkText produced is
// not valid UTF-8 on its own — the exact failure mode that made Postgres
// reject an insert and (before the IngestText fix) abort the rest of the doc.
func assertAllChunksValidUTF8(t *testing.T, chunks []string) {
	t.Helper()
	for i, c := range chunks {
		if !utf8.ValidString(c) {
			t.Errorf("chunk %d is not valid UTF-8 on its own: %q", i, c)
		}
	}
}

func TestChunkText_NeverSplitsMultiByteCharacters(t *testing.T) {
	// Build a document where a multi-byte character (em dash "—", 3 bytes in
	// UTF-8: E2 80 94) sits right at the naive byte-1500 cut point, with no
	// newline nearby to rescue it — exactly the scenario that broke
	// CDA_FHIR_MAPPING_INVENTORY.md and the EHK_Compliance_PHI_Isolation
	// builtin doc in production.
	filler := strings.Repeat("a", 1497) // pushes the em dash's first byte to offset 1497
	text := filler + "— this text continues right after the em dash with no newline in between so the naive byte cut has nothing to rescue it and must fall back to a raw offset that used to land mid-character before this fix"

	chunks := chunkText(text, 1500, 200)
	if len(chunks) < 2 {
		t.Fatalf("expected the text to require at least 2 chunks, got %d", len(chunks))
	}
	assertAllChunksValidUTF8(t, chunks)

	// The em dash itself must survive intact in whichever chunk it landed in —
	// not silently dropped by the boundary snap.
	found := false
	for _, c := range chunks {
		if strings.Contains(c, "—") {
			found = true
			break
		}
	}
	if !found {
		t.Error("em dash character was lost across the chunk boundary")
	}
}

func TestChunkText_ManyMultiByteCharactersNearBoundaries(t *testing.T) {
	// A realistic slice of the kind of content that actually broke: mixed
	// checkmarks, smart quotes, box-drawing separators, and em dashes with
	// short lines (so the newline-rescue heuristic frequently can't find a
	// newline in the back half of the window either).
	var sb strings.Builder
	for i := 0; i < 400; i++ {
		sb.WriteString("✅ Item — uses “smart quotes” and an ─ separator\n")
	}
	text := sb.String()

	chunks := chunkText(text, 1500, 200)
	if len(chunks) < 5 {
		t.Fatalf("expected several chunks from a %d-byte document, got %d", len(text), len(chunks))
	}
	assertAllChunksValidUTF8(t, chunks)
}

func TestSnapToRuneBoundary(t *testing.T) {
	text := "ab—cd" // 'a'(1) 'b'(1) '—'(3 bytes: E2 80 94) 'c'(1) 'd'(1)
	// byte layout: a=0, b=1, em-dash starts at 2 (bytes 2,3,4), c=5, d=6
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"already at rune start (ascii)", 1, 1},
		{"already at rune start (lead byte of em dash)", 2, 2},
		{"one byte into em dash — snaps back to its lead byte", 3, 2},
		{"two bytes into em dash — snaps back to its lead byte", 4, 2},
		{"right after em dash — already a boundary", 5, 5},
		{"end of string", len(text), len(text)},
		{"start of string", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := snapToRuneBoundary(text, tc.in)
			if got != tc.want {
				t.Errorf("snapToRuneBoundary(%q, %d) = %d, want %d", text, tc.in, got, tc.want)
			}
			// Whatever it snaps to must be a valid split point.
			if got > 0 && got < len(text) && !utf8.RuneStart(text[got]) {
				t.Errorf("snapToRuneBoundary(%q, %d) = %d is not a rune boundary", text, tc.in, got)
			}
		})
	}
}

func TestChunkText_ShortTextUnaffected(t *testing.T) {
	text := "short text — under the chunk size"
	chunks := chunkText(text, 1500, 200)
	if len(chunks) != 1 || chunks[0] != text {
		t.Fatalf("short text should pass through as a single unmodified chunk, got %v", chunks)
	}
}

func TestIngestText_NilDBDegradesGracefully(t *testing.T) {
	// Pre-existing contract, unrelated to the chunking fix, kept as a guard
	// against a regression while touching this function's error handling.
	svc := &EmbeddingService{db: nil}
	n, err := svc.IngestText(context.Background(), "test", "ref", "file", "hello world", nil)
	if err != nil {
		t.Fatalf("nil-db IngestText should degrade gracefully, got err: %v", err)
	}
	if n != 0 {
		t.Fatalf("nil-db IngestText should report 0 chunks stored, got %d", n)
	}
}

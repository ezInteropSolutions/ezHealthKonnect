// edi/segment_reader.go
// Tokenizer: delimiter detection plus segment/element/sub-element splitting.
// Delimiter detection is derived robustly from the ISA envelope's own
// declared separators (splitting by the element separator and counting
// positions relative to the literal "ISA" tag) rather than hand-computed
// fixed byte offsets — safer against off-by-one mistakes. The segment-
// terminator fallback logic mirrors (does not import — edi must not depend
// on processing, and processing predates edi) the same fixed-106-length +
// fallback-list algorithm processing/batch_splitter.go's
// detectEDISegmentTerminator already proves correct; duplicating this one
// small, standard-defined utility at two independent layers (batch-level
// interchange splitting vs. message-level tokenizing) is reasonable and
// keeps edi/ free of a cross-package dependency for a handful of lines.
package edi

import "strings"

// Delimiters holds the four separator characters an X12 interchange
// declares (or, for content with no ISA envelope, the defaults used).
type Delimiters struct {
	Element    string // e.g. "*"
	SubElement string // e.g. ":" — component/sub-element separator
	Repetition string // e.g. "^" — 5010's ISA11 repetition separator (not used to split in phase 1, captured for completeness)
	Segment    string // e.g. "~" or "\n" — may be multi-character
}

// defaultDelimiters is used when content has no ISA envelope to declare its
// own — the overwhelmingly common X12 convention, per this plan's own
// documented risk note.
func defaultDelimiters() Delimiters {
	return Delimiters{Element: "*", SubElement: ":", Repetition: "^", Segment: "\n"}
}

// DetectDelimiters inspects content and returns the delimiters in effect,
// plus whether a real ISA envelope was present (false for the bare-ST shape
// processing/batch_splitter.go's splitEDITransactions can produce when a
// payload held more than one transaction set).
func DetectDelimiters(content string) (Delimiters, bool) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "ISA") || len(trimmed) < 4 {
		d := defaultDelimiters()
		d.Segment = detectSegmentTerminatorFallback(trimmed)
		return d, false
	}

	elementSep := string(trimmed[3])
	segTerm := detectSegmentTerminator(trimmed)

	isaBody := trimmed
	if idx := strings.Index(trimmed, segTerm); idx > 0 {
		isaBody = trimmed[:idx]
	}

	fields := strings.Split(isaBody, elementSep)
	// fields[0] is the literal "ISA" tag; fields[1..16] are ISA01..ISA16.
	subElementSep := ":"
	repetitionSep := "^"
	if len(fields) >= 17 {
		if last := fields[16]; len(last) == 1 { // ISA16: component element separator
			subElementSep = last
		}
		if rep := fields[11]; len(rep) == 1 { // ISA11: repetition separator (5010)
			repetitionSep = rep
		}
	}

	return Delimiters{Element: elementSep, SubElement: subElementSep, Repetition: repetitionSep, Segment: segTerm}, true
}

// detectSegmentTerminator returns the segment terminator for ISA-prefixed
// content. The ISA segment is exactly 106 characters when its 16 fixed-
// length elements and single-character delimiters are all standard; the
// character at index 105 is the terminator the sender chose. Falls back to
// the same common-terminator list processing/batch_splitter.go's
// detectEDISegmentTerminator uses if the content is too short or malformed.
func detectSegmentTerminator(content string) string {
	const isaLength = 106
	if len(content) >= isaLength {
		ch := string(content[isaLength-1])
		if ch == "\r" || ch == "\n" || ch == "~" || ch == "|" {
			return ch
		}
	}
	return detectSegmentTerminatorFallback(content)
}

func detectSegmentTerminatorFallback(content string) string {
	for _, t := range []string{"~\r\n", "~\n", "~\r", "~", "\r\n", "\n"} {
		if strings.Contains(content, t) {
			return t
		}
	}
	return "\n"
}

// SplitSegments splits content into individual segment strings using the
// given segment terminator, trimming whitespace and dropping empty parts.
func SplitSegments(content string, delimiters Delimiters) []string {
	raw := strings.Split(content, delimiters.Segment)
	segments := make([]string, 0, len(raw))
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s != "" {
			segments = append(segments, s)
		}
	}
	return segments
}

// ParseSegment splits one segment string into its ID (the first element)
// and the remaining elements, using the element separator.
func ParseSegment(segment string, delimiters Delimiters) (id string, elements []string) {
	parts := strings.Split(segment, delimiters.Element)
	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], parts[1:]
}

// SplitSubElements splits one composite element into its sub-elements using
// the component/sub-element separator.
func SplitSubElements(element string, delimiters Delimiters) []string {
	if delimiters.SubElement == "" {
		return []string{element}
	}
	return strings.Split(element, delimiters.SubElement)
}

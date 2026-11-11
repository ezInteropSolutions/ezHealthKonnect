// services/executors/transform/hl7_query_text.go
//
// A small reader for the received HL7 v2 message text, used by
// hl7.query_parse: it keeps each field's repetitions, components and
// subcomponents (which the parsed message flattens) and the segment text
// itself (to echo a QPD back). The message is first re-encoded to the
// standard delimiters (| ^ ~ \ &), so everything read from it, and every
// segment echoed, is in the encoding the reply uses.
package transform

import (
	"encoding/hex"
	"strings"
)

const standardDelimiters = "|^~\\&" // field, component, repetition, escape, subcomponent

// hl7Text is a received message, re-encoded to the standard delimiters.
type hl7Text struct {
	segments []string
}

// readHL7Text splits raw into segments and re-encodes them when the sender
// used other delimiters. ok is false when raw doesn't start with an MSH.
func readHL7Text(raw string) (hl7Text, bool) {
	raw = strings.TrimLeft(raw, "\x0b\r\n \t")
	if len(raw) < 8 || !strings.HasPrefix(raw, "MSH") {
		return hl7Text{}, false
	}
	from := string(raw[3])
	if enc, _, _ := strings.Cut(raw[4:], from); len(enc) >= 4 {
		from += enc[:4]
	} else {
		return hl7Text{}, false
	}
	var segs []string
	for _, line := range strings.FieldsFunc(raw, func(r rune) bool { return r == '\r' || r == '\n' || r == '\x1c' }) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if from != standardDelimiters {
			line = reencodeSegment(line, from)
		}
		segs = append(segs, line)
	}
	return hl7Text{segments: segs}, true
}

// reencodeSegment rewrites one segment from the delimiters in from (field,
// component, repetition, escape, subcomponent) to the standard ones. A
// standard delimiter that was data under the sender's encoding is escaped.
// MSH-1 and MSH-2 are rewritten to the standard values.
func reencodeSegment(seg, from string) string {
	fieldSep := rune(from[0])
	body := seg
	prefix := ""
	if strings.HasPrefix(seg, "MSH") && len(seg) >= 4 {
		prefix = "MSH" + standardDelimiters
		body = seg[4:]
		if _, rest, found := strings.Cut(body, string(fieldSep)); found {
			body = string(fieldSep) + rest
		} else {
			body = ""
		}
	}
	mapped := map[rune]rune{}
	for i, r := range from {
		mapped[r] = rune(standardDelimiters[i])
	}
	escapeFor := map[rune]string{'|': `\F\`, '^': `\S\`, '~': `\R\`, '\\': `\E\`, '&': `\T\`}
	esc := rune(from[3])
	var b strings.Builder
	b.WriteString(prefix)
	inEscape := false
	for _, r := range body {
		switch {
		case r == esc:
			b.WriteRune('\\')
			inEscape = !inEscape
		case inEscape:
			b.WriteRune(r)
		default:
			if to, ok := mapped[r]; ok {
				b.WriteRune(to)
			} else if e, ok := escapeFor[r]; ok {
				b.WriteString(e)
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// segment returns the n-th (1-based) segment named name, or "".
func (m hl7Text) segment(name string, n int) string {
	for _, s := range m.segments {
		if segmentName(s) == name {
			if n--; n == 0 {
				return s
			}
		}
	}
	return ""
}

func segmentName(seg string) string {
	name, _, _ := strings.Cut(seg, "|")
	return name
}

// field returns field n of seg, encoded, with all its repetitions. MSH is
// numbered as the standard numbers it: MSH-1 is the field separator.
func field(seg string, n int) string {
	parts := strings.Split(seg, "|")
	if strings.HasPrefix(seg, "MSH|") {
		switch {
		case n == 1:
			return "|"
		case n < 1:
			return ""
		}
		n-- // parts[1] is MSH-2
	}
	if n < 1 || n >= len(parts) {
		return ""
	}
	return parts[n]
}

// fieldAt reads "SEG.F" from the message: the first such segment's field,
// encoded, with all its repetitions.
func (m hl7Text) fieldAt(path string) (string, bool) {
	seg, num, ok := splitFieldPath(path)
	if !ok {
		return "", false
	}
	return field(m.segment(seg, 1), num), true
}

// splitFieldPath splits "QPD.3" into ("QPD", 3).
func splitFieldPath(path string) (string, int, bool) {
	seg, rest, found := strings.Cut(strings.TrimSpace(path), ".")
	if !found || seg == "" {
		return "", 0, false
	}
	n := 0
	for _, c := range rest {
		if c < '0' || c > '9' {
			return "", 0, false
		}
		n = n*10 + int(c-'0')
	}
	return seg, n, n > 0
}

// repetitions splits an encoded field into its repetitions.
func repetitions(f string) []string {
	if f == "" {
		return nil
	}
	return strings.Split(f, "~")
}

// component returns component n (1-based) of an encoded repetition.
func component(rep string, n int) string {
	parts := strings.Split(rep, "^")
	if n < 1 || n > len(parts) {
		return ""
	}
	return parts[n-1]
}

// subcomponent returns subcomponent n (1-based) of an encoded component.
func subcomponent(comp string, n int) string {
	parts := strings.Split(comp, "&")
	if n < 1 || n > len(parts) {
		return ""
	}
	return parts[n-1]
}

// unescapeHL7 decodes the standard escape sequences of an encoded value:
// \F\ \S\ \T\ \R\ \E\ and \Xhh..\; formatting sequences (\.br\ and the
// like) and anything unknown are dropped.
func unescapeHL7(v string) string {
	if !strings.Contains(v, `\`) {
		return v
	}
	var b strings.Builder
	for {
		i := strings.IndexByte(v, '\\')
		if i < 0 {
			b.WriteString(v)
			return b.String()
		}
		b.WriteString(v[:i])
		rest := v[i+1:]
		j := strings.IndexByte(rest, '\\')
		if j < 0 {
			// A lone escape character: keep the rest as it is.
			b.WriteString(v[i:])
			return b.String()
		}
		seq := rest[:j]
		switch {
		case seq == "F":
			b.WriteByte('|')
		case seq == "S":
			b.WriteByte('^')
		case seq == "T":
			b.WriteByte('&')
		case seq == "R":
			b.WriteByte('~')
		case seq == "E":
			b.WriteByte('\\')
		case strings.HasPrefix(seq, "X"):
			if raw, err := hex.DecodeString(seq[1:]); err == nil {
				b.Write(raw)
			}
		}
		v = rest[j+1:]
	}
}

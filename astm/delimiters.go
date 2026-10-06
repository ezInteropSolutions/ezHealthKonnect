// astm/delimiters.go
// Reads the H (Header) record's own delimiter-declaration field live —
// mirrors edi.DetectDelimiters' own "never hardcoded" discipline — with
// "|"/"\"/"^"/"&" as sane fallback defaults when no H record is present
// (the overwhelmingly common ASTM convention, confirmed against the real
// Mindray manual excerpt this feature's own plan cites).
//
// ASTM's H record is structurally analogous to HL7's MSH-1/MSH-2: the field
// delimiter is whichever byte immediately follows the leading "H", and the
// record's own second field (delimited by that byte) declares the three
// remaining special characters in a fixed order — repeat, component, escape
// (e.g. "H|\^&|..." declares field="|", repeat="\", component="^",
// escape="&").
package astm

import "strings"

// Delimiters holds the four separator characters an ASTM message declares
// (or, for content with no H record, the defaults used).
type Delimiters struct {
	Field     string // e.g. "|"
	Repeat    string // e.g. "\\"
	Component string // e.g. "^"
	Escape    string // e.g. "&"
}

// defaultDelimiters is the standard ASTM E1394-97 convention, used when
// content has no H record to declare its own.
func defaultDelimiters() Delimiters {
	return Delimiters{Field: "|", Repeat: "\\", Component: "^", Escape: "&"}
}

// DetectDelimiters inspects records (already split on CR by the caller — see
// SplitRecords) and returns the delimiters in effect, plus whether a real H
// record was found to declare them.
func DetectDelimiters(records []string) (Delimiters, bool) {
	for _, rec := range records {
		if len(rec) < 2 || rec[0] != 'H' {
			continue
		}
		fieldDelim := string(rec[1])
		fields := strings.Split(rec, fieldDelim)
		// fields[0] is the literal "H" record-type marker; fields[1] is the
		// delimiter-declaration field (e.g. "\^&").
		if len(fields) < 2 || len(fields[1]) < 3 {
			d := defaultDelimiters()
			d.Field = fieldDelim
			return d, true
		}
		decl := fields[1]
		return Delimiters{
			Field:     fieldDelim,
			Repeat:    string(decl[0]),
			Component: string(decl[1]),
			Escape:    string(decl[2]),
		}, true
	}
	return defaultDelimiters(), false
}

// SplitRecords splits raw message content into individual record strings on
// CR (0x0D), trimming whitespace/LF and dropping empty parts. Accepts both
// bare-CR and CRLF record separation.
func SplitRecords(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\r")
	raw := strings.Split(normalized, "\r")
	records := make([]string, 0, len(raw))
	for _, r := range raw {
		r = strings.Trim(r, "\n\r \t")
		if r != "" {
			records = append(records, r)
		}
	}
	return records
}

// SplitFields splits one record string into its record-type ID (the first
// character) and the remaining fields, using the field separator. Field 0 in
// the returned slice is the record's own sequence number (e.g. O1's "1"),
// matching ASTM's own convention that the record-type letter is NOT counted
// as field 1 — field numbering in ASTMFieldDef.Pos starts at the sequence
// number position.
func SplitFields(record string, d Delimiters) (recordID string, fields []string) {
	if record == "" {
		return "", nil
	}
	recordID = string(record[0])
	rest := record[1:]
	rest = strings.TrimPrefix(rest, d.Field)
	if rest == "" && len(record) == 1 {
		return recordID, nil
	}
	fields = strings.Split(rest, d.Field)
	return recordID, fields
}

// SplitComponents splits one field's raw value into its component-delimited
// sub-values.
func SplitComponents(field string, d Delimiters) []string {
	if d.Component == "" {
		return []string{field}
	}
	return strings.Split(field, d.Component)
}

// SplitRepeats splits one field's raw value into its repeat-delimited items.
func SplitRepeats(field string, d Delimiters) []string {
	if d.Repeat == "" {
		return []string{field}
	}
	return strings.Split(field, d.Repeat)
}

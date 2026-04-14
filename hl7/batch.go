// hl7/batch.go
//
// SplitMessages detects and splits a raw input that contains more than one HL7
// message.  The three forms it handles are:
//
//  1. Bare MSH-to-MSH concatenation — the most common case for file uploads and
//     copy-paste.  Each MSH starts a new message; everything up to (but not
//     including) the next MSH belongs to the current message.
//
//  2. BHS/BTS batch wrappers (HL7 §2.9.2) — the BHS segment opens the batch and
//     BTS closes it; the individual messages are still delineated by MSH segments.
//     BHS, BTS, FHS, FTS wrapper segments are stripped before returning results.
//
//  3. Single message — returns a one-element slice so callers can always iterate.
//
// Segment delimiter normalisation:
//
//	The function normalises \r\n → \r and \n → \r before splitting, matching
//	the HL7 spec (CR = segment terminator) while tolerating Unix/Windows files.
package hl7

import "strings"

// SplitMessages splits rawHL7 into individual HL7 messages.
// Returns a slice with at least one element (the original text) even for a
// single message.  Empty strings and batch-wrapper segments (BHS/BTS/FHS/FTS)
// are removed.
func SplitMessages(rawHL7 string) []string {
	if strings.TrimSpace(rawHL7) == "" {
		return nil
	}

	// Normalise line endings → bare CR (HL7 segment terminator)
	norm := strings.ReplaceAll(rawHL7, "\r\n", "\r")
	norm = strings.ReplaceAll(norm, "\n", "\r")

	segments := strings.Split(norm, "\r")

	var messages []string
	var current []string

	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}

		// Skip HL7 batch/file wrapper segments
		if isBatchWrapper(seg) {
			continue
		}

		if strings.HasPrefix(seg, "MSH") {
			// Flush the previous message
			if len(current) > 0 {
				messages = append(messages, strings.Join(current, "\r"))
			}
			current = []string{seg}
		} else {
			current = append(current, seg)
		}
	}

	// Flush the final message
	if len(current) > 0 {
		messages = append(messages, strings.Join(current, "\r"))
	}

	return messages
}

// CountMessages returns the number of individual HL7 messages in rawHL7 by
// counting MSH segments.  It is cheaper than SplitMessages when you only need
// the count.
func CountMessages(rawHL7 string) int {
	norm := strings.ReplaceAll(rawHL7, "\r\n", "\r")
	norm = strings.ReplaceAll(norm, "\n", "\r")

	count := 0
	for _, seg := range strings.Split(norm, "\r") {
		seg = strings.TrimSpace(seg)
		if strings.HasPrefix(seg, "MSH") {
			count++
		}
	}
	return count
}

// isBatchWrapper returns true for HL7 batch / file envelope segments that
// should be stripped when splitting.
func isBatchWrapper(seg string) bool {
	if len(seg) < 3 {
		return false
	}
	switch seg[:3] {
	case "BHS", "BTS", "FHS", "FTS":
		return true
	}
	return false
}

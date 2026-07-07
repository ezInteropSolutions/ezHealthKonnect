// services/parsers/cda/generic_section_processor.go
// GenericSectionProcessor replaces all 9 individual section_*.go files.
// It reads field XPaths from the CDASectionDef loaded via ccda_2_1.json —
// zero Go code changes are required to add a new CDA section or new fields.
//
// XPath conventions used in ccda_2_1.json (all relative to <section>):
//   entry/<path>/@attr  — attribute value on a descendant element
//   entry/<path>        — text content of a descendant element
//
// The processor strips the leading "entry/" prefix and evaluates each XPath
// relative to each <entry> child of the section, producing one map per entry.

package cdaparser

import (
	"strings"

	cdaSchema "ezhealthkonnect/cda"
	"github.com/beevik/etree"
)

// GenericSectionProcessor implements SectionProcessor by reading all XPaths
// from the CDASectionDef embedded at construction time. It handles the
// primary xpath plus optional xpathDisplay, xpathSystem, xpathUnit, and
// xpathFamily supplementary paths defined in ccda_2_1.json.
type GenericSectionProcessor struct {
	baseSectionProcessor
	sectionDef *cdaSchema.CDASectionDef
}

// NewGenericSectionProcessor wraps a CDASectionDef in a SectionProcessor.
func NewGenericSectionProcessor(def *cdaSchema.CDASectionDef) *GenericSectionProcessor {
	return &GenericSectionProcessor{sectionDef: def}
}

// SectionKey satisfies SectionProcessor.
func (p *GenericSectionProcessor) SectionKey() string {
	return p.sectionDef.Key
}

// Process extracts one map per <entry> element in sectionEl.
// Each map contains field.Key = primary value, field.Key+"Display" = display, etc.
// Missing XPath values produce empty strings (no panic, no nil entries).
func (p *GenericSectionProcessor) Process(sectionEl *etree.Element, _ *cdaSchema.CDASectionDef) *SectionResult {
	result := p.newResult(p.SectionKey(), p.extractNarrativeHTML(sectionEl))

	for _, entry := range p.findEntries(sectionEl) {
		rec := make(map[string]interface{})

		for _, field := range p.sectionDef.Fields {
			// Primary value
			if v := extractValueByXPath(entry, field.XPath); v != "" {
				rec[field.Key] = v
			}
			// Display name
			if field.XPathDisplay != "" {
				if v := extractValueByXPath(entry, field.XPathDisplay); v != "" {
					rec[field.Key+"Display"] = v
				}
			}
			// Coding system OID/URI
			if field.XPathSystem != "" {
				if v := extractValueByXPath(entry, field.XPathSystem); v != "" {
					rec[field.Key+"System"] = v
				}
			}
			// Unit (for PQ values)
			if field.XPathUnit != "" {
				if v := extractValueByXPath(entry, field.XPathUnit); v != "" {
					rec[field.Key+"Unit"] = v
				}
			}
			// Family name component (for PN values where given is the primary xpath)
			if field.XPathFamily != "" {
				if v := extractValueByXPath(entry, field.XPathFamily); v != "" {
					rec[field.Key+"Family"] = v
				}
			}
		}

		if len(rec) > 0 {
			result.Entries = append(result.Entries, rec)
		}
	}

	result.EntryCount = len(result.Entries)
	return result
}

// =====================================
// XPath extraction helpers
// =====================================

// extractValueByXPath evaluates a single XPath expression (as stored in
// ccda_2_1.json) relative to an <entry> element.  The "entry/" prefix is
// stripped before evaluation since we already hold the entry element.
//
// Attribute access: paths ending in /@attrName call SelectAttrValue.
// Text content:     paths without a /@attrName suffix call element.Text().
func extractValueByXPath(entry *etree.Element, xpath string) string {
	if xpath == "" {
		return ""
	}
	// Strip entry/ prefix — we start from the entry element itself.
	rel := stripEntryPrefix(xpath)
	if rel == "" {
		return ""
	}
	// Remove namespace-qualified attribute predicates that etree cannot match.
	rel = sanitizeXPath(rel)

	// Detect attribute access: path ends with /@attrName
	if idx := strings.LastIndex(rel, "/@"); idx >= 0 {
		elemPath := rel[:idx]
		attrName := rel[idx+2:]
		if elemPath == "" {
			// Attribute directly on the entry element.
			return entry.SelectAttrValue(attrName, "")
		}
		target := entry.FindElement(elemPath)
		if target == nil {
			return ""
		}
		return target.SelectAttrValue(attrName, "")
	}

	// Text content.
	target := entry.FindElement(rel)
	if target == nil {
		return ""
	}
	return strings.TrimSpace(target.Text())
}

// stripEntryPrefix removes the leading "entry/" from an XPath that is rooted
// at the <section> element so it can be used relative to an <entry> element.
func stripEntryPrefix(xpath string) string {
	const prefix = "entry/"
	if strings.HasPrefix(xpath, prefix) {
		return xpath[len(prefix):]
	}
	return xpath
}

// sanitizeXPath strips any predicate expression from an XPath that etree cannot
// evaluate without panicking.  etree only supports:
//
//	[@attr]            — attribute existence (no namespace prefix)
//	[@attr='value']    — attribute equality  (no namespace prefix)
//	[N]                — 1-based positional integer
//
// Everything else (child-element filters like [code/@code='X'], function calls
// like [not(@nullFlavor)], namespace-qualified attributes like [@xsi:type='Y'],
// compound expressions, etc.) is silently stripped so FindElement degrades to a
// broader — but safe — match rather than panicking.
func sanitizeXPath(xpath string) string {
	var b strings.Builder
	i := 0
	for i < len(xpath) {
		if xpath[i] != '[' {
			b.WriteByte(xpath[i])
			i++
			continue
		}
		// Locate the matching closing bracket.
		end := strings.Index(xpath[i:], "]")
		if end < 0 {
			// No matching ] — write the rest verbatim and stop.
			b.WriteString(xpath[i:])
			break
		}
		pred := xpath[i+1 : i+end] // content between [ and ]
		if isEtreeSafePredicate(pred) {
			b.WriteString(xpath[i : i+end+1]) // keep [pred]
		}
		// Unsafe predicate: skip (strip the [pred] from output).
		i += end + 1
	}
	return b.String()
}

// isEtreeSafePredicate returns true only for predicates etree can evaluate
// without panicking.
func isEtreeSafePredicate(pred string) bool {
	pred = strings.TrimSpace(pred)
	if pred == "" {
		return false
	}
	// [@a='x',@b='y'] — cda/builder's multi-condition write-side grammar
	// (needed there to disambiguate same-predicate siblings like Allergy's
	// Severity vs Status Observation — see cda/builder/xpath_writer.go).
	// etree's own path compiler has no comma-condition syntax at all and
	// panics ("mismatched filter quotes") if handed one raw, so this is
	// unsafe here the same way child-element filters already are below —
	// strip it and degrade to a first-match read, don't crash.
	if strings.Contains(pred, ",") {
		return false
	}
	// [N] — positional integer.
	allDigits := true
	for _, c := range pred {
		if c < '0' || c > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		return true
	}
	// [@attr] or [@attr='value'] — must start with @, no namespace prefix (:),
	// no sub-element path (/), no function calls (().
	if strings.HasPrefix(pred, "@") {
		rest := pred[1:]
		if !strings.ContainsAny(rest, ":/.(") {
			return true
		}
	}
	return false
}

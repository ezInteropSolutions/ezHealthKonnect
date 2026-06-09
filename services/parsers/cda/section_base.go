// services/parsers/cda/section_base.go
// baseSectionProcessor provides shared helpers for all section processor
// implementations: narrative HTML extraction, XPath value retrieval, and
// entry-level iteration.

package cdaparser

import (
	"strings"

	"github.com/beevik/etree"
)

// baseSectionProcessor is embedded by all concrete section processors to
// inherit the common helper methods below. It carries no state.
type baseSectionProcessor struct{}

// extractNarrativeHTML returns the inner XML of the <text> child of a
// CDA <section> element as a string. Returns "" if no <text> is found.
// The XHTML content is returned as-is for the clinical document viewer.
func (b *baseSectionProcessor) extractNarrativeHTML(sectionEl *etree.Element) string {
	textEl := sectionEl.FindElement("text")
	if textEl == nil {
		return ""
	}
	doc := etree.NewDocument()
	// Copy the element so we can serialise just that subtree.
	doc.SetRoot(textEl.Copy())
	html, err := doc.WriteToString()
	if err != nil {
		return ""
	}
	return html
}

// attr retrieves an XML attribute value from the first element matching path
// (relative to parent). Returns "" if the element or attribute is absent.
// Example: b.attr(sectionEl, "entry/act/statusCode", "code")
func (b *baseSectionProcessor) attr(parent *etree.Element, path, attrName string) string {
	el := parent.FindElement(path)
	if el == nil {
		return ""
	}
	return el.SelectAttrValue(attrName, "")
}

// text retrieves the trimmed text content of the first element matching path.
// Returns "" if the element is absent or has no text.
func (b *baseSectionProcessor) text(parent *etree.Element, path string) string {
	el := parent.FindElement(path)
	if el == nil {
		return ""
	}
	return strings.TrimSpace(el.Text())
}

// setIfNotEmpty adds key→value to m only when value is a non-empty string.
func (b *baseSectionProcessor) setIfNotEmpty(m map[string]interface{}, key, value string) {
	if value != "" {
		m[key] = value
	}
}

// extractCodedValue is a convenience helper that extracts a coded concept from
// an element at path, returning a map with "code", "display", and "system" keys.
// Only non-empty values are included.
func (b *baseSectionProcessor) extractCodedValue(
	parent *etree.Element,
	path string,
) map[string]string {
	el := parent.FindElement(path)
	if el == nil {
		return nil
	}
	result := make(map[string]string)
	if code := el.SelectAttrValue("code", ""); code != "" {
		result["code"] = code
	}
	if display := el.SelectAttrValue("displayName", ""); display != "" {
		result["display"] = display
	}
	if system := el.SelectAttrValue("codeSystem", ""); system != "" {
		result["system"] = system
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// findEntries returns all <entry> child elements of a section.
func (b *baseSectionProcessor) findEntries(sectionEl *etree.Element) []*etree.Element {
	return sectionEl.SelectElements("entry")
}

// newResult constructs a SectionResult with the given key and narrative HTML
// and an initialised (non-nil) Entries slice.
func (b *baseSectionProcessor) newResult(key, narrativeHTML string) *SectionResult {
	return &SectionResult{
		SectionKey:    key,
		NarrativeHTML: narrativeHTML,
		Entries:       make([]map[string]interface{}, 0),
	}
}

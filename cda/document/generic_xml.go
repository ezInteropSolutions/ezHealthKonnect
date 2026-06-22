// cda/document/generic_xml.go
//
// GenericXMLToJSON is the authoritative, guaranteed-complete XML->JSON
// representation of a CDA document. Unlike the rest of this package (which
// builds a curated, hand-typed CDADocument tree for FHIR mapping
// convenience), this function has ZERO knowledge of CDA, C-CDA, or RIM
// semantics. It does not know what a "templateId" or "negationInd" is; it
// just mechanically mirrors every element, attribute, and text node into a
// JSON-shaped map[string]interface{}.
//
// Because it enumerates nothing CDA-specific, it cannot have a "missing
// field" class of bug, and it requires zero maintenance as new C-CDA
// templates, vendor extensions, or document variants appear -- a curated
// struct tree fundamentally cannot make that claim, no matter how many
// fields are added to it.
//
// This is the layer to address when completeness matters (compliance
// export, audit, "show me everything in this document"). The curated
// CDADocument/CDAEntry tree (and the sections/header maps built from it)
// remain the right tool for semantic, named access and FHIR mapping -- they
// are a convenience VIEW, not the fidelity guarantee.
//
// Convention (a standard, widely used XML<->JSON mapping -- not novel):
//   - Every element becomes a JSON object.
//   - Every attribute becomes a key prefixed with "@", e.g. root="..." -> "@root".
//   - An element's own text (including text after nested child elements,
//     i.e. XML "tail" text) is concatenated and trimmed into "#text".
//   - Each distinct child tag name becomes a key holding either a single
//     nested object (one occurrence) or a JSON array of nested objects
//     (multiple occurrences) -- "auto-array-on-repeat".
//   - A leaf element with no attributes and no children collapses to a
//     plain JSON string (its text) instead of a wrapper object, to keep
//     the common case readable.
//   - XML namespace prefixes are not part of the key (etree's Element.Tag
//     is already the local name; the namespace URI, if ever needed, would
//     live in Element.Space -- omitted here since this codebase's source
//     documents use a single default namespace and no consumer has needed
//     namespace-qualified disambiguation).
package cdadocument

import (
	"strings"

	"github.com/beevik/etree"
)

// GenericXMLToJSON converts an entire element subtree into a generic,
// schema-agnostic JSON-shaped value. Pass the document root for a
// full-fidelity mirror of the whole CDA document.
func GenericXMLToJSON(el *etree.Element) interface{} {
	if el == nil {
		return nil
	}

	attrs := el.Attr
	children := el.ChildElements()
	text := elementOwnText(el)

	if len(attrs) == 0 && len(children) == 0 {
		// Leaf with no attributes/children: collapse to a plain string
		// (still 100% lossless -- there is nothing else to carry).
		return text
	}

	obj := make(map[string]interface{}, len(attrs)+len(children)+1)
	for _, a := range attrs {
		obj["@"+a.Key] = a.Value
	}
	if text != "" {
		obj["#text"] = text
	}

	// Group children by tag name, preserving first-seen order isn't
	// required for JSON object keys (Go maps + encoding/json don't
	// guarantee key order anyway); within a tag, document order IS
	// preserved since ChildElements() walks in document order.
	byTag := make(map[string][]interface{})
	var order []string
	for _, child := range children {
		tag := child.Tag
		if _, seen := byTag[tag]; !seen {
			order = append(order, tag)
		}
		byTag[tag] = append(byTag[tag], GenericXMLToJSON(child))
	}
	for _, tag := range order {
		vals := byTag[tag]
		if len(vals) == 1 {
			obj[tag] = vals[0]
		} else {
			obj[tag] = vals
		}
	}

	return obj
}

// elementOwnText concatenates every CharData token that is a direct child
// of el -- both text appearing before the first child element and text
// trailing after child elements (XML "tail" text, e.g. "<a>x<b/>y</a>" has
// tail text "y" after <b/>). etree.Element.Text() only returns the leading
// segment, which would silently drop "y" -- walking el.Child directly avoids
// that loss.
func elementOwnText(el *etree.Element) string {
	var b strings.Builder
	for _, tok := range el.Child {
		if cd, ok := tok.(*etree.CharData); ok {
			b.WriteString(cd.Data)
		}
	}
	return strings.TrimSpace(b.String())
}

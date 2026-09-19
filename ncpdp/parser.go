// ncpdp/parser.go
//
// ParseMessage is the read-direction mirror of ncpdp/builder.BuildDocument:
// ONE generic recursive walk (parseNode) over the schema's Fields/Groups
// tree, driven entirely by schema data — no per-transaction-type or
// per-group Go function, matching edi.ParseTransactionSet's precedent.
// Simpler than EDI's own loop-matching (NCPDP elements are uniquely named
// at each tree position, so there is no trigger-segment/sibling-discriminator
// ambiguity to resolve) and simpler than CDA's typed entry parser (no
// RIM act-type dispatch) — a plain schema-driven tree walk suffices.
package ncpdp

import (
	"fmt"
	"strings"

	"ezhealthkonnect/xmlpath"

	"github.com/beevik/etree"
)

// ParseResult is the canonical output of parsing one NCPDP SCRIPT message.
type ParseResult struct {
	TransactionType string                 `json:"transactionType"`
	MessageAttrs    map[string]string      `json:"messageAttrs"`
	Header          map[string]interface{} `json:"header"`
	Body            map[string]interface{} `json:"body"`
}

// ParseMessage parses a raw NCPDP SCRIPT XML message against spec, returning
// the canonical field/group tree for both <Header> and the single <Body>
// transaction element.
func ParseMessage(spec *NCPDPSpecDef, xmlContent string) (*ParseResult, error) {
	if spec == nil {
		return nil, fmt.Errorf("ncpdp: nil spec")
	}
	doc := etree.NewDocument()
	if err := doc.ReadFromString(xmlContent); err != nil {
		return nil, fmt.Errorf("ncpdp: parsing XML: %w", err)
	}
	root := doc.SelectElement("Message")
	if root == nil {
		return nil, fmt.Errorf("ncpdp: missing root <Message> element")
	}

	msgAttrs := make(map[string]string, len(root.Attr))
	for _, a := range root.Attr {
		msgAttrs[a.Key] = a.Value
	}

	var header map[string]interface{}
	if hdr := spec.Header(); hdr != nil {
		if headerEl := root.SelectElement("Header"); headerEl != nil {
			header = parseNode(spec, hdr.Fields, hdr.Groups, headerEl)
		}
	}

	bodyEl := root.SelectElement("Body")
	if bodyEl == nil {
		return nil, fmt.Errorf("ncpdp: missing <Body> element")
	}
	txEl := firstChildElement(bodyEl)
	if txEl == nil {
		return nil, fmt.Errorf("ncpdp: <Body> has no transaction element")
	}
	txType := txEl.Tag
	tx, ok := spec.Transactions[txType]
	if !ok {
		return nil, fmt.Errorf("ncpdp: unsupported transaction type %q", txType)
	}

	return &ParseResult{
		TransactionType: txType,
		MessageAttrs:    msgAttrs,
		Header:          header,
		Body:            parseNode(spec, tx.Fields, tx.Groups, txEl),
	}, nil
}

// parseNode extracts fields and (recursively) nested groups from el into a
// canonical map, keyed by each field's/group-ref's own Key — the single
// mechanism used for both NCPDPGroupDef.{Fields,Groups} and
// NCPDPTransactionDef.{Fields,Groups}, so adding a new group or transaction
// type never needs a new Go function.
func parseNode(spec *NCPDPSpecDef, fields []NCPDPFieldDef, groups []NCPDPGroupRef, el *etree.Element) map[string]interface{} {
	out := map[string]interface{}{}
	for _, f := range fields {
		if v, ok := extractValue(el, f.XPath); ok {
			out[f.Key] = v
		} else if f.FallbackXPath != "" {
			if v, ok := extractValue(el, f.FallbackXPath); ok {
				out[f.Key] = v
			}
		}
	}
	for _, ref := range groups {
		childGroup, ok := spec.Groups[ref.GroupKey]
		if !ok {
			continue // already fail-fast validated at load time; defensive only
		}
		tag := ref.XMLElement
		if tag == "" {
			tag = childGroup.XMLElement
		}
		if ref.Repeatable {
			var items []interface{}
			for _, childEl := range el.SelectElements(tag) {
				items = append(items, parseNode(spec, childGroup.Fields, childGroup.Groups, childEl))
			}
			if len(items) > 0 {
				out[ref.Key] = items
			}
			continue
		}
		if childEl := el.SelectElement(tag); childEl != nil {
			out[ref.Key] = parseNode(spec, childGroup.Fields, childGroup.Groups, childEl)
		}
	}
	return out
}

// extractValue resolves xpath (the same grammar xmlpath.WriteAtXPath writes,
// relative to anchor) read-only, returning the element's text or, for an
// "@attr"/"tag/@attr" path, the attribute value. Reuses
// xmlpath.TryFindAtXPath/SplitPathSegments directly rather than a second,
// parallel path-walking implementation.
func extractValue(anchor *etree.Element, xpath string) (string, bool) {
	if xpath == "" {
		return "", false
	}
	// "." is a self-reference sentinel (see NCPDPFieldDef.FallbackXPath's own
	// doc comment) — read the anchor's own text content directly, bypassing
	// xmlpath's segment walk entirely (it has no "self" concept: an empty
	// segment list from SplitPathSegments(".") would otherwise try to find a
	// CHILD element literally named ".").
	if xpath == "." {
		v := anchor.Text()
		return v, v != ""
	}
	el, ok := xmlpath.TryFindAtXPath(anchor, xpath)
	if !ok || el == nil {
		return "", false
	}
	segments := xmlpath.SplitPathSegments(xpath)
	last := segments[len(segments)-1]
	if strings.HasPrefix(last, "@") {
		v := el.SelectAttrValue(last[1:], "")
		return v, v != ""
	}
	v := el.Text()
	return v, v != ""
}

func firstChildElement(el *etree.Element) *etree.Element {
	for _, child := range el.ChildElements() {
		return child
	}
	return nil
}

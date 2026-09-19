// ncpdp/builder/document_builder.go
//
// BuildDocument is the write-direction mirror of ncpdp.ParseMessage: ONE
// generic recursive walk (buildNode) over the schema's Fields/Groups tree,
// driven entirely by schema data, using ezhealthkonnect/xmlpath for every
// element/attribute write — the same proven mechanism cda/builder uses,
// extracted specifically so a second XML-based engine wouldn't need to
// reimplement it. Unlike CDA's builder, there is no classCode/moodCode/
// templateId boilerplate to inject and no predicate-based sibling
// disambiguation to reason about (NCPDP elements are uniquely named at each
// tree position) — the one real complication carried over from CDA is
// schema-sequence ordering: a group's own children are written in whatever
// order Fields/Groups happen to be declared, which doesn't always match the
// real interleaved XML order (see NCPDPGroupDef.ElementOrder), so
// xmlpath.ReorderChildrenByTag is applied as the same kind of post-hoc pass
// cda/builder/entry_archetypes.go already established.
package builder

import (
	"fmt"

	"ezhealthkonnect/ncpdp"
	"ezhealthkonnect/xmlpath"

	"github.com/beevik/etree"
)

// BuildDocument builds a complete NCPDP SCRIPT XML message for
// transactionType from canonical header/body maps (the same shape
// ncpdp.ParseMessage produces, so a parsed document round-trips through
// BuildDocument(spec, txType, result.Header, result.Body)).
func BuildDocument(spec *ncpdp.NCPDPSpecDef, transactionType string, header map[string]interface{}, body map[string]interface{}) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("ncpdp: nil spec")
	}
	tx, ok := spec.Transactions[transactionType]
	if !ok {
		return "", fmt.Errorf("ncpdp: unknown transaction type %q", transactionType)
	}

	doc := etree.NewDocument()
	doc.CreateProcInst("xml", `version="1.0" encoding="UTF-8"`)

	msgEl := doc.CreateElement("Message")
	attrs := spec.MessageAttrs
	setAttrIfNonEmpty(msgEl, "DatatypesVersion", attrs.DatatypesVersion)
	setAttrIfNonEmpty(msgEl, "TransportVersion", attrs.TransportVersion)
	setAttrIfNonEmpty(msgEl, "TransactionDomain", attrs.TransactionDomain)
	setAttrIfNonEmpty(msgEl, "TransactionVersion", attrs.TransactionVersion)
	setAttrIfNonEmpty(msgEl, "StructuresVersion", attrs.StructuresVersion)
	setAttrIfNonEmpty(msgEl, "ECLVersion", attrs.ECLVersion)

	headerEl := msgEl.CreateElement("Header")
	if hdr := spec.Header(); hdr != nil && header != nil {
		buildNode(spec, hdr.Fields, hdr.Groups, hdr.ElementOrder, headerEl, header)
	}

	bodyEl := msgEl.CreateElement("Body")
	txEl := bodyEl.CreateElement(tx.Key)
	buildNode(spec, tx.Fields, tx.Groups, tx.ElementOrder, txEl, body)

	doc.Indent(4)
	out, err := doc.WriteToString()
	if err != nil {
		return "", fmt.Errorf("ncpdp: serializing document: %w", err)
	}
	return out, nil
}

// buildNode writes fields and (recursively) nested groups from data onto
// el — the single mechanism used for both NCPDPGroupDef and
// NCPDPTransactionDef, mirroring parseNode's identical role on the read
// side.
func buildNode(spec *ncpdp.NCPDPSpecDef, fields []ncpdp.NCPDPFieldDef, groups []ncpdp.NCPDPGroupRef, elementOrder []string, el *etree.Element, data map[string]interface{}) {
	for _, f := range fields {
		v, present := data[f.Key]
		if !present {
			continue
		}
		s, ok := ncpdp.StringValue(v)
		if !ok {
			continue
		}
		xmlpath.WriteAtXPath(el, f.XPath, s)
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
		raw, present := data[ref.Key]
		if !present {
			continue
		}
		if ref.Repeatable {
			for _, item := range ncpdp.ToSlice(raw) {
				itemMap, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				// Always direct-create for repeats — every item is a
				// genuinely new sibling, mirroring cda/builder's
				// writeRepeatingGroups rationale for why this is never a
				// find-or-reuse predicate match.
				childEl := el.CreateElement(tag)
				buildNode(spec, childGroup.Fields, childGroup.Groups, childGroup.ElementOrder, childEl, itemMap)
			}
			continue
		}
		itemMap, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		childEl := xmlpath.WriteAtXPath(el, tag, "")
		buildNode(spec, childGroup.Fields, childGroup.Groups, childGroup.ElementOrder, childEl, itemMap)
	}

	if len(elementOrder) > 0 {
		xmlpath.ReorderChildrenByTag(el, elementOrder)
	}
}

func setAttrIfNonEmpty(el *etree.Element, name, value string) {
	if value != "" {
		el.CreateAttr(name, value)
	}
}

// cda/document/parser.go
// CDAParser is the entry point for the Sprint B typed document model.
// It converts an etree root element (ClinicalDocument) into a fully-typed
// CDADocument by delegating to headerParser, sectionParser, and entryParser.
//
// Usage:
//   loader, _ := cda.NewCDASchemaLoader("./cda/schemas")
//   p          := cdadocument.NewCDAParser(loader)
//   doc        := p.ParseDocument(root, rawXML)

package cdadocument

import (
	"fmt"

	cdaSchema "ezhealthkonnect/cda"

	"github.com/beevik/etree"
)

// CDAParser builds a CDADocument from an etree root element.
// It uses the CDASchemaLoader to resolve section template IDs to schema keys.
// A single CDAParser instance is safe for concurrent use across goroutines.
type CDAParser struct {
	schemaLoader *cdaSchema.CDASchemaLoader
}

// NewCDAParser constructs a CDAParser backed by the given schema loader.
func NewCDAParser(loader *cdaSchema.CDASchemaLoader) *CDAParser {
	return &CDAParser{schemaLoader: loader}
}

// ParseDocument converts an etree root element (ClinicalDocument) into a fully
// typed CDADocument. The raw XML string is stored on CDADocument.Raw verbatim
// so no clinical data is ever lost.
//
// The returned document is always non-nil. On a completely empty root the
// Header and Sections fields will be zero-valued.
func (p *CDAParser) ParseDocument(root *etree.Element, raw string) *CDADocument {
	doc := &CDADocument{
		Raw:           raw,
		SectionsByKey: make(map[string]*CDASection),
	}

	hp := &headerParser{}
	doc.Header = hp.parseHeader(root)

	sp := &sectionParser{schemaLoader: p.schemaLoader}
	doc.Sections = sp.parseSections(root)

	// Build SectionsByKey index for O(1) lookup by sprint C/D consumers.
	for i := range doc.Sections {
		sec := &doc.Sections[i]
		if sec.Key != "" {
			doc.SectionsByKey[sec.Key] = sec
		}
	}

	return doc
}

// ParseFromRawXML parses a raw CDA XML string into a typed CDADocument without
// requiring the caller to import etree. Prefer this over ParseDocument when the
// etree root is not already available (e.g. in pipeline executors).
func (p *CDAParser) ParseFromRawXML(rawXML string) (*CDADocument, error) {
	etreeDoc := etree.NewDocument()
	if err := etreeDoc.ReadFromString(rawXML); err != nil {
		return nil, fmt.Errorf("cdaparser: XML parse failed: %w", err)
	}
	root := etreeDoc.Root()
	if root == nil {
		return nil, fmt.Errorf("cdaparser: empty XML document (no root element)")
	}
	return p.ParseDocument(root, rawXML), nil
}

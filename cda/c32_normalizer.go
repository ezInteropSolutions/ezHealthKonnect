// cda/c32_normalizer.go
// C32TemplateNormalizer upgrades legacy C32/HITSP template OIDs to their
// C-CDA 2.1 equivalents so that all section processors can use a single
// C-CDA 2.1 code path regardless of the source document profile.
//
// Normalization is purely structural: it rewrites <templateId root="..."/>
// attributes in place; clinical content is never modified.

package cda

import (
	"fmt"
	"strings"

	"github.com/beevik/etree"
)

// C32TemplateNormalizer detects and normalises C32/HITSP documents.
// It is stateless beyond the loader reference — safe for concurrent use.
type C32TemplateNormalizer struct {
	loader *CDASchemaLoader
}

// NewC32TemplateNormalizer constructs a normaliser backed by the given schema loader.
func NewC32TemplateNormalizer(loader *CDASchemaLoader) *C32TemplateNormalizer {
	return &C32TemplateNormalizer{loader: loader}
}

// DetectProfile parses the document header to determine which CDA profile the
// document conforms to. It inspects only ClinicalDocument-level templateId
// elements to avoid false positives from section-level OIDs.
func (n *C32TemplateNormalizer) DetectProfile(rawXML string) (CDAProfile, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromString(rawXML); err != nil {
		return ProfileBase, fmt.Errorf("c32_normalizer: cannot parse XML: %w", err)
	}

	root := doc.Root()
	if root == nil {
		return ProfileBase, nil
	}

	// Check direct-child templateId elements on ClinicalDocument.
	for _, el := range root.SelectElements("templateId") {
		oid := el.SelectAttrValue("root", "")
		if oid == "" {
			continue
		}
		if n.loader.IsC32DetectionOID(oid) {
			// Distinguish C32 from HITSP by prefix convention.
			if strings.HasPrefix(oid, "1.3.6.1.4.1.19376") {
				return ProfileHITSP, nil
			}
			return ProfileC32, nil
		}
		// Check for C-CDA 2.1 document-level OIDs.
		if _, isCCDA := n.loader.profile.DocumentTemplates[oid]; isCCDA {
			return ProfileCCDA21, nil
		}
	}

	return ProfileBase, nil
}

// NormalizationResult carries the outcome of a Normalize call.
type NormalizationResult struct {
	NormalizedXML   string
	Substitutions   int    // number of templateId root attributes rewritten
	OriginalProfile CDAProfile
}

// Normalize rewrites all C32/HITSP <templateId root="..."/> attributes to their
// C-CDA 2.1 equivalents throughout the entire document tree. Returns the
// normalised XML string. If no substitutions are needed the original XML is
// returned unchanged (zero allocations).
func (n *C32TemplateNormalizer) Normalize(rawXML string) (*NormalizationResult, error) {
	profile, err := n.DetectProfile(rawXML)
	if err != nil {
		return nil, err
	}

	result := &NormalizationResult{
		OriginalProfile: profile,
	}

	// Nothing to do for C-CDA 2.1 and base CDA documents.
	if profile == ProfileCCDA21 || profile == ProfileBase {
		result.NormalizedXML = rawXML
		return result, nil
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromString(rawXML); err != nil {
		return nil, fmt.Errorf("c32_normalizer: cannot parse XML: %w", err)
	}

	// Walk all <templateId> elements anywhere in the document.
	for _, el := range doc.FindElements("//templateId") {
		attr := el.SelectAttr("root")
		if attr == nil {
			continue
		}
		if newOID, found := n.loader.ResolveC32TemplateID(attr.Value); found {
			attr.Value = newOID
			result.Substitutions++
		}
	}

	if result.Substitutions == 0 {
		result.NormalizedXML = rawXML
		return result, nil
	}

	normalized, err := doc.WriteToString()
	if err != nil {
		return nil, fmt.Errorf("c32_normalizer: cannot serialise normalised XML: %w", err)
	}
	result.NormalizedXML = normalized
	return result, nil
}

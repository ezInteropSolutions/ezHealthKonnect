// cda/document/entry_parser.go
// entryParser converts CDA <entry> elements into typed CDAEntry structs.
//
// Supported entry act types (per C-CDA 2.1 §4 entry templates):
//   observation, act, procedure, substanceAdministration, encounter, organizer, supply
//
// Each <entry> element is a thin wrapper; the actual clinical act is its first
// recognised child element. entryParser dispatches to parseClinicalAct which
// handles all shared fields (id, code, statusCode, effectiveTime, value,
// participants, entryRelationships) plus type-specific fields.
//
// Invariant: SelectElements is used for every element that can repeat.
// nav() handles path navigation — no FindElement calls in this file.

package cdadocument

import (
	"strings"

	"github.com/beevik/etree"
)

// entryActTypes lists the XML element names that can appear as the direct clinical
// act child of a CDA <entry> or <entryRelationship> element, in search order.
var entryActTypes = []string{
	"observation",
	"act",
	"organizer",
	"substanceAdministration",
	"procedure",
	"encounter",
	"supply",
}

type entryParser struct{}

// parseEntries returns one CDAEntry for each <entry> child of a <section>.
func (ep *entryParser) parseEntries(sectionEl *etree.Element) []CDAEntry {
	var entries []CDAEntry
	for _, entryEl := range sectionEl.SelectElements("entry") {
		if e, ok := ep.extractAct(entryEl); ok {
			entries = append(entries, e)
		}
	}
	return entries
}

// extractAct finds the first recognised act-type child of wrapper and parses it.
// Returns (entry, true) when an act is found; (zero, false) otherwise.
func (ep *entryParser) extractAct(wrapper *etree.Element) (CDAEntry, bool) {
	for _, actType := range entryActTypes {
		if child := wrapper.SelectElement(actType); child != nil {
			return ep.parseClinicalAct(child, actType), true
		}
	}
	return CDAEntry{}, false
}

// parseClinicalAct builds a CDAEntry from any of the seven act-type elements.
// Fields common to all types are extracted first; type-specific fields follow
// in the switch block.
func (ep *entryParser) parseClinicalAct(el *etree.Element, entryType string) CDAEntry {
	entry := CDAEntry{
		EntryType:   entryType,
		ClassCode:   el.SelectAttrValue("classCode", ""),
		MoodCode:    el.SelectAttrValue("moodCode", ""),
		NullFlavor:  el.SelectAttrValue("nullFlavor", ""),
		NegationInd: el.SelectAttrValue("negationInd", "false") == "true",
	}

	// ALL <id> elements — entries can carry multiple identifiers.
	for _, idEl := range el.SelectElements("id") {
		entry.Id = append(entry.Id, parseII(idEl))
	}

	// ALL <templateId> elements on this act itself (entry-level conformance,
	// distinct from the enclosing section's templateIds).
	for _, tidEl := range el.SelectElements("templateId") {
		if root := tidEl.SelectAttrValue("root", ""); root != "" {
			entry.TemplateIds = append(entry.TemplateIds, root)
		}
	}

	if codeEl := el.SelectElement("code"); codeEl != nil {
		entry.Code = parseCD(codeEl)
	}
	if textEl := el.SelectElement("text"); textEl != nil {
		entry.Text = parseEntryText(textEl)
	}
	if scEl := el.SelectElement("statusCode"); scEl != nil {
		entry.StatusCode = scEl.SelectAttrValue("code", "")
	}
	// Result Observation's <interpretationCode> (CONF:1198-7147) is a direct
	// child, a sibling of <code>/<statusCode>/<value> — not nested under an
	// entryRelationship. 0..*; only the first is captured (see CDAEntry's
	// InterpretationCode doc comment).
	if icEl := el.SelectElement("interpretationCode"); icEl != nil {
		entry.InterpretationCode = parseCD(icEl)
	}
	// <referenceRange><observationRange><text> -- only the free-text shape is
	// evidenced in the corpus (see CDAEntry.ReferenceRangeText's doc comment);
	// only the first <referenceRange> is captured.
	if rrEl := el.SelectElement("referenceRange"); rrEl != nil {
		if orEl := rrEl.SelectElement("observationRange"); orEl != nil {
			if textEl := orEl.SelectElement("text"); textEl != nil {
				entry.ReferenceRangeText = strings.TrimSpace(textEl.Text())
			}
		}
	}
	// ALL <effectiveTime> elements — substanceAdministration commonly carries
	// two: an IVL_TS duration and a PIVL_TS/EIVL_TS dosing frequency.
	for _, etEl := range el.SelectElements("effectiveTime") {
		xsiType := etEl.SelectAttrValue("xsi:type", "")
		if xsiType == "" {
			xsiType = etEl.SelectAttrValue("type", "")
		}
		etEntry := CDAEffectiveTimeEntry{
			XSIType: xsiType,
			Range:   parseIVL_TS(etEl),
		}
		// PIVL_TS/EIVL_TS carry the dosing-frequency interval in <period>,
		// not in low/high/value — parseIVL_TS doesn't read it.
		if periodEl := etEl.SelectElement("period"); periodEl != nil {
			period := parsePQ(periodEl)
			etEntry.Period = &period
		}
		entry.EffectiveTimes = append(entry.EffectiveTimes, etEntry)
	}
	if len(entry.EffectiveTimes) > 0 {
		entry.EffectiveTime = entry.EffectiveTimes[0].Range
	}
	if valEls := el.SelectElements("value"); len(valEls) > 0 {
		if len(valEls) == 1 {
			entry.Value = parseValue(valEls[0])
		} else {
			// Multiple sibling <value> elements on one act (non-conformant per
			// C-CDA's own templates, which allow at most one -- but real vendor
			// documents do this). Blank/nullFlavor-only siblings are filtered
			// out first (pure noise), then any remaining sibling that is ALSO
			// coded (CD/CE/CS/CV) is folded into the primary value's
			// Code.Translations -- the same "alternate coding for the same
			// concept" relationship CDACodeToCodeableConcept already renders as
			// extra FHIR codings for a genuine <translation> child. A sibling
			// that isn't coded (a different value[x] shape than the primary,
			// e.g. an INT score alongside a CD interpretation) can't be merged
			// this way and still lands in AdditionalValues, surfaced by
			// additionalValuesWarningMessages (declarative_engine.go). Real gap
			// found on a vendor Smoking Status entry: 3 sibling <value>
			// elements, the first a blank nullFlavor placeholder and the other
			// two carrying alternate codes for the same "never smoked" concept
			// -- the old first-element-wins logic kept the blank one and
			// silently dropped both real codes.
			var nonEmpty []*CDAValue
			for _, valEl := range valEls {
				if v := parseValue(valEl); v != nil && !isEmptyCDAValue(v) {
					nonEmpty = append(nonEmpty, v)
				}
			}
			if len(nonEmpty) > 0 {
				entry.Value = nonEmpty[0]
				for _, extra := range nonEmpty[1:] {
					if entry.Value.Code != nil && extra.Code != nil {
						entry.Value.Code.Translations = append(entry.Value.Code.Translations, *extra.Code)
						continue
					}
					entry.AdditionalValues = append(entry.AdditionalValues, *extra)
				}
			}
		}
	}

	entry.Participants = ep.parseParticipants(el)
	entry.EntryRelationships = ep.parseEntryRelationships(el)
	entry.Authors = ep.parseAuthors(el)
	entry.Performers = ep.parsePerformers(el)
	entry.Informants = ep.parseInformants(el)

	switch entryType {
	case "substanceAdministration":
		entry.RouteCode = ep.parseRouteCode(el)
		entry.ApproachSiteCode = ep.parseChildCode(el, "approachSiteCode")
		entry.AdministrationUnitCode = ep.parseChildCode(el, "administrationUnitCode")
		entry.DoseQuantity = ep.parsePQField(el, "doseQuantity")
		entry.RateQuantity = ep.parsePQField(el, "rateQuantity")
		entry.MaxDoseQuantity = ep.parseMaxDoseQuantity(el)
		entry.Consumable = ep.parseConsumable(el)
		entry.RepeatNumber = parseRepeatNumber(el)
		entry.Precondition = ep.parsePrecondition(el)
	case "procedure":
		entry.TargetSiteCode = ep.parseTargetSiteCode(el)
	case "supply":
		// Medication Supply Order/Dispense (e.g. entryRelationship typeCode="REFR"
		// off a substanceAdministration) — refill count, dispensed quantity, and
		// the product being supplied (mirrors substanceAdministration's
		// Consumable/DoseQuantity but under <quantity>/<product> instead).
		entry.RepeatNumber = parseRepeatNumber(el)
		entry.Quantity = ep.parsePQField(el, "quantity")
		entry.Product = ep.parseProduct(el)
	case "organizer":
		entry.Components = ep.parseComponents(el)
		entry.RelatedSubjectCode = ep.parseRelatedSubjectCode(el)
	}

	return entry
}

// ========================
// Participants
// ========================

func (ep *entryParser) parseParticipants(el *etree.Element) []CDAParticipant {
	var participants []CDAParticipant
	for _, pEl := range el.SelectElements("participant") {
		p := CDAParticipant{
			TypeCode: pEl.SelectAttrValue("typeCode", ""),
		}
		if fc := pEl.SelectElement("functionCode"); fc != nil {
			p.FunctionCode = parseCD(fc)
		}
		if t := pEl.SelectElement("time"); t != nil {
			p.Time = parseIVL_TS(t)
		}
		if roleEl := pEl.SelectElement("participantRole"); roleEl != nil {
			p.ParticipantRole = ep.parseParticipantRole(roleEl)
		}
		participants = append(participants, p)
	}
	return participants
}

func (ep *entryParser) parseParticipantRole(el *etree.Element) CDAParticipantRole {
	role := CDAParticipantRole{
		ClassCode: el.SelectAttrValue("classCode", ""),
	}
	for _, idEl := range el.SelectElements("id") {
		role.Ids = append(role.Ids, parseII(idEl))
	}
	if codeEl := el.SelectElement("code"); codeEl != nil {
		role.Code = parseCD(codeEl)
	}
	for _, addrEl := range el.SelectElements("addr") {
		role.Addresses = append(role.Addresses, parseAD(addrEl))
	}
	for _, telEl := range el.SelectElements("telecom") {
		role.Telecoms = append(role.Telecoms, parseTEL(telEl))
	}
	if peEl := el.SelectElement("playingEntity"); peEl != nil {
		pe := CDAPlayingEntity{
			ClassCode: peEl.SelectAttrValue("classCode", ""),
		}
		if c := peEl.SelectElement("code"); c != nil {
			pe.Code = parseCD(c)
		}
		for _, n := range peEl.SelectElements("name") {
			pe.Names = append(pe.Names, parsePN(n))
		}
		if d := peEl.SelectElement("desc"); d != nil {
			pe.Desc = d.Text()
		}
		role.PlayingEntity = &pe
	}
	if seEl := el.SelectElement("scopingEntity"); seEl != nil {
		se := CDAEntity{}
		for _, idEl := range seEl.SelectElements("id") {
			se.Ids = append(se.Ids, parseII(idEl))
		}
		if c := seEl.SelectElement("code"); c != nil {
			se.Code = parseCD(c)
		}
		if d := seEl.SelectElement("desc"); d != nil {
			se.Desc = d.Text()
		}
		role.ScopingEntity = &se
	}
	return role
}

// ========================
// Authors / Performers
// ========================
// Distinct from <participant>: <author> and <performer> identify who
// recorded/ordered or carried out this specific act (e.g. the ordering
// provider on a substanceAdministration), separate from the document-level
// header.Authors. Reuses the same free-function parsers the header parser
// uses (parseAssignedAuthor in header_parser.go, parseAssignedEntity in
// datatypes.go) so there is one implementation for each, not two.

func (ep *entryParser) parseAuthors(el *etree.Element) []CDAAuthor {
	var authors []CDAAuthor
	for _, authorEl := range el.SelectElements("author") {
		author := CDAAuthor{}
		if t := authorEl.SelectElement("time"); t != nil {
			author.Time = parseTS(t)
		}
		if aaEl := authorEl.SelectElement("assignedAuthor"); aaEl != nil {
			author.AssignedAuthor = parseAssignedAuthor(aaEl)
		}
		authors = append(authors, author)
	}
	return authors
}

func (ep *entryParser) parsePerformers(el *etree.Element) []CDAPerformer {
	var performers []CDAPerformer
	for _, perfEl := range el.SelectElements("performer") {
		perf := CDAPerformer{
			TypeCode: perfEl.SelectAttrValue("typeCode", ""),
		}
		if fc := perfEl.SelectElement("functionCode"); fc != nil {
			perf.FunctionCode = parseCD(fc)
		}
		if t := perfEl.SelectElement("time"); t != nil {
			perf.Time = parseIVL_TS(t)
		}
		if aeEl := perfEl.SelectElement("assignedEntity"); aeEl != nil {
			perf.AssignedEntity = parseAssignedEntity(aeEl)
		}
		performers = append(performers, perf)
	}
	return performers
}

// parseInformants extracts <informant><assignedEntity> children of el. The
// base CDA R2 schema also allows <informant><relatedEntity> — not parsed
// here, no real entry seen using it.
func (ep *entryParser) parseInformants(el *etree.Element) []CDAInformant {
	var informants []CDAInformant
	for _, infEl := range el.SelectElements("informant") {
		if aeEl := infEl.SelectElement("assignedEntity"); aeEl != nil {
			informants = append(informants, CDAInformant{AssignedEntity: parseAssignedEntity(aeEl)})
		}
	}
	return informants
}

// ========================
// Entry relationships
// ========================

func (ep *entryParser) parseEntryRelationships(el *etree.Element) []CDAEntryRelationship {
	var rels []CDAEntryRelationship
	for _, relEl := range el.SelectElements("entryRelationship") {
		rel := CDAEntryRelationship{
			TypeCode:     relEl.SelectAttrValue("typeCode", ""),
			InversionInd: relEl.SelectAttrValue("inversionInd", "false") == "true",
		}
		if nested, ok := ep.extractAct(relEl); ok {
			rel.Entry = nested
		}
		rels = append(rels, rel)
	}
	return rels
}

// ========================
// Organizer components
// ========================

func (ep *entryParser) parseComponents(el *etree.Element) []CDAEntry {
	var components []CDAEntry
	for _, compEl := range el.SelectElements("component") {
		if e, ok := ep.extractAct(compEl); ok {
			components = append(components, e)
		}
	}
	return components
}

// parseRelatedSubjectCode extracts the Family History Organizer's
// <subject><relatedSubject><code> (the family member's relationship to the
// patient). Returns nil when no <subject> child is present — most organizers
// (Vital Signs, Care Team, Results) don't have one.
func (ep *entryParser) parseRelatedSubjectCode(el *etree.Element) *CDACode {
	codeEl := nav(el, "subject", "relatedSubject", "code")
	if codeEl == nil {
		return nil
	}
	c := parseCD(codeEl)
	return &c
}

// ========================
// substanceAdministration-specific fields
// ========================

func (ep *entryParser) parseRouteCode(el *etree.Element) *CDACode {
	return ep.parseChildCode(el, "routeCode")
}

// parseMaxDoseQuantity reads <maxDoseQuantity> (RTO) -- see CDAEntry.MaxDoseQuantity.
func (ep *entryParser) parseMaxDoseQuantity(el *etree.Element) *CDARatio {
	mdEl := el.SelectElement("maxDoseQuantity")
	if mdEl == nil {
		return nil
	}
	r := CDARatio{}
	if numEl := mdEl.SelectElement("numerator"); numEl != nil {
		r.Numerator = parsePQ(numEl)
	}
	if denEl := mdEl.SelectElement("denominator"); denEl != nil {
		r.Denominator = parsePQ(denEl)
	}
	return &r
}

// parsePrecondition reads <precondition><criterion><value> -- see
// CDAEntry.Precondition's doc comment for why <value>, not <criterion>'s own
// (usually fixed "ASSERTION") <code>.
func (ep *entryParser) parsePrecondition(el *etree.Element) *CDACode {
	valEl := nav(el, "precondition", "criterion", "value")
	if valEl == nil {
		return nil
	}
	c := parseCD(valEl)
	return &c
}

// ========================
// procedure-specific fields
// ========================

// parseTargetSiteCode reads <targetSiteCode> — a direct child of <procedure>
// in the base CDA R2 schema, not an entryRelationship. See the doc comment on
// CDAEntry.TargetSiteCode for why this is procedure-only.
func (ep *entryParser) parseTargetSiteCode(el *etree.Element) *CDACode {
	return ep.parseChildCode(el, "targetSiteCode")
}

// parseChildCode reads a single direct-child coded element (routeCode,
// targetSiteCode, approachSiteCode, administrationUnitCode, ...) — the
// common "optional CD/CE child, parsed via parseCD" shape shared by several
// entry fields. Returns nil when the child is absent, matching every
// existing *CDACode field's own nil-when-absent contract.
func (ep *entryParser) parseChildCode(el *etree.Element, tag string) *CDACode {
	if childEl := el.SelectElement(tag); childEl != nil {
		c := parseCD(childEl)
		return &c
	}
	return nil
}

func (ep *entryParser) parsePQField(el *etree.Element, tag string) *CDAQuantity {
	if pqEl := el.SelectElement(tag); pqEl != nil {
		q := parsePQ(pqEl)
		return &q
	}
	return nil
}

func (ep *entryParser) parseConsumable(el *etree.Element) *CDAConsumable {
	mpEl := nav(el, "consumable", "manufacturedProduct")
	if mpEl == nil {
		return nil
	}
	mp := ep.parseManufacturedProduct(mpEl)
	cons := CDAConsumable{ManufacturedProduct: mp}
	return &cons
}

// parseProduct extracts <supply><product><manufacturedProduct> — the same
// shape as substanceAdministration's <consumable><manufacturedProduct>, but
// reached via a different wrapper element per the Medication Supply Order
// template (C-CDA 2.1 §4.43).
func (ep *entryParser) parseProduct(el *etree.Element) *CDAManufacturedProduct {
	mpEl := nav(el, "product", "manufacturedProduct")
	if mpEl == nil {
		return nil
	}
	mp := ep.parseManufacturedProduct(mpEl)
	return &mp
}

func (ep *entryParser) parseManufacturedProduct(mpEl *etree.Element) CDAManufacturedProduct {
	mp := CDAManufacturedProduct{}
	for _, idEl := range mpEl.SelectElements("id") {
		mp.Ids = append(mp.Ids, parseII(idEl))
	}
	if matEl := mpEl.SelectElement("manufacturedMaterial"); matEl != nil {
		mat := CDAMaterial{}
		if c := matEl.SelectElement("code"); c != nil {
			mat.Code = parseCD(c)
		}
		if n := matEl.SelectElement("name"); n != nil {
			mat.Name = n.Text()
		}
		if lot := matEl.SelectElement("lotNumberText"); lot != nil {
			mat.LotNumberText = lot.Text()
		}
		mp.ManufacturedMaterial = &mat
	}
	if orgEl := mpEl.SelectElement("manufacturerOrganization"); orgEl != nil {
		org := parseOrganization(orgEl)
		mp.ManufacturerOrganization = &org
	}
	return mp
}

// parseRepeatNumber reads <repeatNumber value="..."/> (refill/occurrence count)
// or, when no value is present, its IVL_INT nullFlavor (e.g. nullFlavor="OTH"
// when the count is genuinely not documented as a number) — same fallback
// pattern as every other CDA datatype parser in this package.
func parseRepeatNumber(el *etree.Element) string {
	rnEl := el.SelectElement("repeatNumber")
	if rnEl == nil {
		return ""
	}
	if v := rnEl.SelectAttrValue("value", ""); v != "" {
		return v
	}
	return rnEl.SelectAttrValue("nullFlavor", "")
}

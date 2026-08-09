// cda/builder/entry_archetypes.go
//
// buildEntry is the single, schema-driven entry constructor used for every
// CCD section. It turned out not to need per-archetype functions
// (act+SUBJ-observation, substanceAdministration, organizer, plain
// observation, etc. all reduce to the same "create root anchor, create
// optional nested anchor, write every field via its own XPath" shape once
// WriteAtXPath exists). The only genuinely archetype-specific knowledge
// left is the classCode/moodCode/statusCode boilerplate each root/nested XML
// tag needs per the C-CDA 2.1 IG — a small, finite lookup table, not
// per-section Go code.
package builder

import (
	"strings"

	cdaSchema "ezhealthkonnect/cda"
	"github.com/beevik/etree"
	"github.com/google/uuid"
)

// tagBoilerplate is the fixed classCode/moodCode/statusCode every CDA entry
// root or nested-observation tag needs per the C-CDA 2.1 IG, keyed by the
// tag's own name (the last path segment of EntryElementPath/
// ObservationElementPath). StatusCode is only injected when non-empty and
// no statusCode is already present — several tags (substanceAdministration,
// encounter, procedure) get their real statusCode from a mapped field
// instead (e.g. medications.status), so no synthetic value is forced there.
var tagBoilerplate = map[string]struct {
	ClassCode  string
	MoodCode   string
	StatusCode string
}{
	"act":                     {"ACT", "EVN", "active"},
	"observation":             {"OBS", "EVN", "completed"},
	"substanceAdministration": {"SBADM", "EVN", ""},
	"organizer":               {"CLUSTER", "EVN", "completed"},
	"encounter":               {"ENC", "EVN", ""},
	"procedure":               {"PROC", "EVN", ""},
	"supply":                  {"SPLY", "EVN", ""},
}

// structuralElementOrder is the front-loaded child order every CDA
// act/observation/organizer/substanceAdministration entry needs, regardless
// of which order this engine's own construction steps happened to create
// them in — confirmed via a real schema validator run (2026-07) that this
// was still wrong even after the earlier templateId-only reorder fix:
// applyTagBoilerplate's own <statusCode> creation (for a tag with a non-
// empty StatusCode default, e.g. observation -> "completed") runs BEFORE
// templateId/id/code exist at all, and code/id are themselves added even
// later (EntryFixedCode/ObsFixedCode/anchor.FixedCode and
// ensureGeneratedID both run after the fields loop). Construction order was
// never a reliable proxy for schema order once any of these overlapped —
// this list is applied as one final pass per constructed element instead.
//
// effectiveTime/value/routeCode/doseQuantity/consumable were added after a
// SECOND real schema validator run (2026-07) found the fields loop's own
// iteration order (JSON array order, not schema order) was leaking straight
// into output order for these five tags specifically — e.g. a section whose
// fields declare drugCode (creates <consumable>) before doseQuantity/
// routeCode, or onsetDate (creates <effectiveTime>) after
// medicationAllergyCode (creates <participant>)/reaction (creates
// <entryRelationship>), produced <consumable> before <doseQuantity>/
// <routeCode> and <effectiveTime>/<value> after <participant>/
// <entryRelationship> — both invalid per the base CDA schema's fixed
// sequence (POCD_MT000040.Observation / .SubstanceAdministration), which
// wants effectiveTime/value BEFORE participant/entryRelationship, and
// effectiveTime/routeCode/doseQuantity BEFORE consumable/performer/author.
// Safe to apply globally: no tag in this list ever appears on more than one
// of the archetypes this engine builds with conflicting order requirements
// (value/routeCode/doseQuantity never co-occur — value is observation-only,
// routeCode/doseQuantity/consumable are substanceAdministration-only), and
// every section that doesn't produce these tags is unaffected by construction.
var structuralElementOrder = []string{
	"realmCode", "typeId", "templateId", "id", "code",
	// text sits right after code (before statusCode) per every CDA base
	// type's fixed sequence (Act/Observation/Procedure/SubstanceAdministration
	// all declare "..., code?, text?, statusCode, ..."). Added after a live
	// Test Pipeline run (2026-07) showed Problem Status's own statusText
	// field landing AFTER value/interpretationCode/etc — it's unlisted-vs-
	// listed here on purpose, same class of bug as author below: statusText
	// is positioned, in the schema's fields[] array, after fields that
	// create value/interpretationCode, so construction order alone put text
	// last purely because of JSON field-list order.
	"text",
	"statusCode",
	"effectiveTime", "value",
	// interpretationCode/methodCode/targetSiteCode are Observation-only tags
	// that sit right after value in the base Observation sequence (value,
	// interpretationCode?, methodCode?, targetSiteCode?, subject?, specimen?,
	// performer*, author*, ...) — found via a live Test Pipeline run
	// (2026-07) after adding Result Observation's interpretationCode/
	// referenceRange: interpretationCode was landing AFTER author, same
	// unlisted-vs-listed class of bug as text/performer/author above (once
	// author became a listed tag, interpretationCode's own "unlisted, keeps
	// construction order" fallback lost to author's fixed low rank).
	"interpretationCode", "methodCode", "targetSiteCode",
	"routeCode", "doseQuantity", "quantity", "consumable",
	// performer precedes author in every CDA base type's fixed sequence
	// (Act/Observation/Procedure/SubstanceAdministration/Encounter/Organizer
	// all declare "..., performer*, author*, informant*, participant*, ...").
	// Added alongside author (below) after a live Test Pipeline run (2026-07)
	// found Procedure/Encounter's pre-existing performer fields (created
	// BEFORE this session's newly-appended author fields, so performer was
	// chronologically first) still sorted AFTER author once author became a
	// LISTED tag — author's own low rank beat performer's "everything else"
	// unlisted rank, inverting their real relative order. Both must be
	// listed, in the correct relative order, not just author alone.
	"performer",
	// author was added after a live Test Pipeline run (2026-07) showed
	// entry-level Author Participation landing AFTER entryRelationship —
	// invalid per every CDA base type's fixed sequence (author always
	// precedes entryRelationship/participant/precondition/referenceRange/
	// reference). Both are unlisted-vs-listed here on purpose: author's own
	// schema field is sometimes positioned, in a section's fields[] array,
	// AFTER other fields that create entryRelationship siblings (e.g.
	// Problem's status/prognosisCode) — construction order alone put
	// entryRelationship first purely because of JSON field-list order, the
	// same class of bug this list exists to correct for other tags.
	"author",
}

// buildEntryRoot creates the root element at entryElementPath (relative to
// "entry/"), applies its tag's classCode/moodCode/statusCode boilerplate,
// injects templateId, and stamps a generated <id> — the core construction
// steps shared by buildEntry's own primary root and buildAlternateEntry's
// single root (see AlternateEntryArchetype's own doc comment). Returns nil
// if entryElementPath is empty (narrative-only sections have none).
func buildEntryRoot(entryEl *etree.Element, entryElementPath, templateID, templateIDExt string) *etree.Element {
	if entryElementPath == "" {
		return nil
	}
	rootEl := WriteAtXPath(entryEl, stripEntryPrefix(entryElementPath), "")
	applyTagBoilerplate(rootEl, lastSegmentTag(entryElementPath))
	if templateID != "" {
		injectTemplateID(rootEl, templateID, templateIDExt)
	}
	ensureGeneratedID(rootEl)
	return rootEl
}

// applyStructuralTemplateAnchors is buildEntry's own StructuralTemplateIDs
// loop, extracted so buildAlternateEntry can share it exactly rather than
// duplicating it — the anchor-processing logic (templateId injection,
// Fixed*/FixedStatusCode, the author/participant/default id-generation
// switch, final reorder) has no dependency on which caller's entryEl it's
// given.
func applyStructuralTemplateAnchors(entryEl *etree.Element, anchors []cdaSchema.StructuralTemplateAnchor) {
	for _, anchor := range anchors {
		el, found := TryFindAtXPath(entryEl, stripEntryPrefix(anchor.Path))
		if !found {
			continue
		}
		applyTagBoilerplate(el, lastSegmentTag(anchor.Path))
		injectTemplateID(el, anchor.TemplateID, anchor.TemplateIDExt)
		if anchor.FixedCode != "" && el.SelectElement("code") == nil {
			codeEl := el.CreateElement("code")
			codeEl.CreateAttr("code", anchor.FixedCode)
			if anchor.FixedCodeSystem != "" {
				codeEl.CreateAttr("codeSystem", anchor.FixedCodeSystem)
			}
			if anchor.FixedCodeDisplay != "" {
				codeEl.CreateAttr("displayName", anchor.FixedCodeDisplay)
			}
		}
		if anchor.FixedStatusCode != "" {
			if sc := el.SelectElement("statusCode"); sc != nil {
				sc.CreateAttr("code", anchor.FixedStatusCode)
			} else {
				el.CreateElement("statusCode").CreateAttr("code", anchor.FixedStatusCode)
			}
		}
		// "author" (Author Participation, templateId .4.119) and
		// "participant" (e.g. Advance Directive's Verifier, Participant2
		// type) are the anchor tags whose CDA base type has NO <id> in
		// their content model at all (unlike act/observation/organizer/
		// manufacturedProduct, which all permit id* and where
		// ensureGeneratedID is correct) — calling it here would add an
		// invalid stray <id> directly under <author>/<participant>.
		// author additionally needs ensureAuthorParticipationShape's own
		// fixup (a required <time> the canonical field catalog never
		// supplies for an entry-level author, unlike the document-level
		// author's real build-time nowCDATimestamp value); participant
		// (Participant2: templateId*, functionCode?, time?, awarenessCode?,
		// participantRole) has no such requirement — confirmed missing
		// entirely via a live Test Pipeline run (2026-07) that found a
		// stray <id> on Advance Directive's Verifier participant.
		switch lastSegmentTag(anchor.Path) {
		case "author":
			ensureAuthorParticipationShape(el)
		case "participant":
			// no-op: no id, no other required fixup
		default:
			ensureGeneratedID(el)
		}
		// Reordered LAST, after templateId/code/id have all been added —
		// not right after injectTemplateID — since code/id are only
		// created above, well after a field write (e.g. reactionCode's
		// own predicate-anchored xpath) may have already put <value> in
		// as this element's first real content.
		reorderChildrenByTag(el, structuralElementOrder)
	}
}

// buildAlternateEntry builds one <entry> element for an AlternateEntryArchetype
// — a section's second (or later), independent entry shape (see
// AlternateEntryArchetype's own doc comment for the motivating case, Medical
// Equipment's optional Procedure Activity Procedure2 entries).
func buildAlternateEntry(sectionEl *etree.Element, alt *cdaSchema.AlternateEntryArchetype, record map[string]interface{}) *etree.Element {
	entryEl := sectionEl.CreateElement("entry")
	entryEl.CreateAttr("typeCode", "DRIV")

	rootEl := buildEntryRoot(entryEl, alt.EntryElementPath, alt.EntryTemplateID, alt.EntryTemplateIDExt)

	for _, field := range alt.Fields {
		writeFieldValue(entryEl, field, record)
	}

	applyStructuralTemplateAnchors(entryEl, alt.StructuralTemplateIDs)

	if rootEl != nil {
		reorderChildrenByTag(rootEl, structuralElementOrder)
	}
	return entryEl
}

// buildEntry builds one <entry> element (appended to sectionEl) from a
// single canonical record, using sec's schema metadata for templateIds and
// every field's own XPath (via WriteAtXPath) for values — no per-section Go
// logic. Canonical field keys follow generic_section_processor.go's exact
// convention: field.Key (primary), field.Key+"Display", +"System", +"Unit",
// +"Family" for the companion XPath variants.
func buildEntry(sectionEl *etree.Element, sec *cdaSchema.CDASectionDef, record map[string]interface{}) *etree.Element {
	entryEl := sectionEl.CreateElement("entry")
	entryEl.CreateAttr("typeCode", "DRIV")

	var rootEl, obsEl *etree.Element
	if sec.EntryElementPath != "" {
		rootEl = buildEntryRoot(entryEl, sec.EntryElementPath, sec.EntryTemplateID, sec.EntryTemplateIDExt)
		if sec.EntryStatusCodeOverride != "" {
			if sc := rootEl.SelectElement("statusCode"); sc != nil {
				sc.CreateAttr("code", sec.EntryStatusCodeOverride)
			}
		}
		if sec.EntryClassCodeOverride != "" {
			rootEl.CreateAttr("classCode", sec.EntryClassCodeOverride)
		}
		if sec.EntryMoodCodeOverride != "" {
			rootEl.CreateAttr("moodCode", sec.EntryMoodCodeOverride)
		}

		if sec.ObservationElementPath != "" {
			obsEl = WriteAtXPath(entryEl, stripEntryPrefix(sec.ObservationElementPath), "")
			applyTagBoilerplate(obsEl, lastSegmentTag(sec.ObservationElementPath))
			if sec.ObsTemplateID != "" {
				injectTemplateID(obsEl, sec.ObsTemplateID, sec.ObsTemplateIDExt)
			}
			ensureGeneratedID(obsEl)
		}
	}

	for _, field := range sec.Fields {
		writeFieldValue(entryEl, field, record)
	}

	if sec.EntryFixedCode != "" && rootEl != nil && rootEl.SelectElement("code") == nil {
		codeEl := rootEl.CreateElement("code")
		codeEl.CreateAttr("code", sec.EntryFixedCode)
		codeSystem := sec.EntryFixedCodeSystem
		if codeSystem == "" {
			codeSystem = "2.16.840.1.113883.6.1" // LOINC — true for every current use
		}
		codeEl.CreateAttr("codeSystem", codeSystem)
		if sec.EntryFixedCodeDisplay != "" {
			codeEl.CreateAttr("displayName", sec.EntryFixedCodeDisplay)
		}
	}

	if sec.ObsFixedCode != "" && obsEl != nil && obsEl.SelectElement("code") == nil {
		codeEl := obsEl.CreateElement("code")
		codeEl.CreateAttr("code", sec.ObsFixedCode)
		if sec.ObsFixedCodeSystem != "" {
			codeEl.CreateAttr("codeSystem", sec.ObsFixedCodeSystem)
		}
		if sec.ObsFixedCodeDisplay != "" {
			codeEl.CreateAttr("displayName", sec.ObsFixedCodeDisplay)
		}
	}

	writeCodeTranslation(rootEl, sec.EntryCodeTranslationCode, sec.EntryCodeTranslationSystem, sec.EntryCodeTranslationDisplay)
	writeCodeTranslation(obsEl, sec.ObsCodeTranslationCode, sec.ObsCodeTranslationSystem, sec.ObsCodeTranslationDisplay)

	applyStructuralTemplateAnchors(entryEl, sec.StructuralTemplateIDs)

	reorderChildrenByTag(rootEl, structuralElementOrder)
	reorderChildrenByTag(obsEl, structuralElementOrder)

	writeRepeatingGroups(rootEl, obsEl, sec, record)
	ensureAssignedEntityIDs(entryEl)
	removeEmptyPlaceholderRoot(entryEl, rootEl)

	return entryEl
}

// removeEmptyPlaceholderRoot drops rootEl from entryEl when rootEl never
// received any real content AND at least one other clinical-statement
// element already carries the entry's actual data. A real schema validator
// found this happening for Social History: EntryElementPath ("entry/
// observation", bare/unpredicated) unconditionally creates a generic Social
// History Observation as rootEl BEFORE any fields run, but every one of
// socialHistory's actual fields (smokingStatus, pregnancyStatus,
// sexualOrientation, genderIdentity) targets a PREDICATED path
// ("entry/observation[code/@code='...']") that only matches an <observation>
// which ALREADY carries that discriminating code — since rootEl never gets
// one (no "observationCode" field is mapped in practice), the predicate
// can't match it and a completely separate, second <observation> sibling
// gets created instead, leaving rootEl an empty shell alongside it. <entry>
// only permits exactly ONE clinical statement child (a CDA schema CHOICE,
// not a sequence), so two siblings — one empty, one real — is invalid
// either way; this removes the empty one rather than leaving dead output.
//
// Deliberately requires BOTH conditions (empty AND a sibling exists) rather
// than just "rootEl is empty": every other section's EntryElementPath is
// either unconditionally populated by a directly-targeting field (so it's
// never actually empty) or is the section's ONLY child of entryEl (so
// removing it would leave nothing) — this only ever fires for the exact
// shape Social History has today, verified via a full section-by-section
// read of every RepeatingGroup/ObservationElementPath combination in
// ccda_2_1.json before writing this guard.
func removeEmptyPlaceholderRoot(entryEl, rootEl *etree.Element) {
	if rootEl == nil || !isEmptyPlaceholder(rootEl) || len(entryEl.ChildElements()) <= 1 {
		return
	}
	entryEl.RemoveChild(rootEl)
}

// isEmptyPlaceholder reports whether el carries no actual clinical content —
// no <code>, no <value>, and no children beyond the universal boilerplate
// (templateId/id/statusCode) every element gets regardless of real data.
func isEmptyPlaceholder(el *etree.Element) bool {
	if el.SelectElement("code") != nil || el.SelectElement("value") != nil {
		return false
	}
	boilerplateTags := map[string]bool{"templateId": true, "id": true, "statusCode": true}
	for _, c := range el.ChildElements() {
		if !boilerplateTags[c.Tag] {
			return false
		}
	}
	return true
}

// writeCodeTranslation adds a static <translation> child under el's own
// <code> element (if el has one and it doesn't already carry a
// <translation>) — see CDASectionDef.EntryCodeTranslationCode's doc comment
// for why this is a code-independent constant, applied regardless of
// whether the <code> itself came from EntryFixedCode/ObsFixedCode or a real
// per-record field. A no-op when code is empty (the common case — most
// sections need no translation) or el/el's own <code> is missing.
func writeCodeTranslation(el *etree.Element, code, codeSystem, display string) {
	if code == "" || el == nil {
		return
	}
	codeEl := el.SelectElement("code")
	if codeEl == nil || codeEl.SelectElement("translation") != nil {
		return
	}
	t := codeEl.CreateElement("translation")
	t.CreateAttr("code", code)
	if codeSystem != "" {
		t.CreateAttr("codeSystem", codeSystem)
	}
	if display != "" {
		t.CreateAttr("displayName", display)
	}
}

// ensureAssignedEntityIDs gives every <assignedEntity> descendant of entryEl
// at least one <id>, <addr>, and <telecom> — CDA's base AssignedEntity type
// is SHALL contain [1..*] id unconditionally regardless of section, and
// several concrete archetypes (e.g. Procedure Activity Performer,
// CONF:1098-7731/7732) additionally SHALL contain at least one addr/telecom
// once a performer is present at all (confirmed missing via a real schema
// validator run, 2026-07: a section's performer/assignedEntity is created by
// name fields alone — e.g. procedures.performerGiven/Family — with no
// guarantee NPI/address/phone were also mapped, since those are each their
// own separate, independently-optional fields). nullFlavor="UNK" is the
// correct CDA idiom for "this element is present but its real value isn't
// known" (already used elsewhere in this package, e.g.
// documentationOf/serviceEvent/effectiveTime/low) — never fabricated content,
// since these are real-world identifier/contact spaces (unlike
// ensureGeneratedID's act/observation instance ids, which mean something
// different and ARE safe to fabricate). Applied uniformly to every
// assignedEntity regardless of which specific archetype's rules technically
// require addr/telecom — CDA's base type always permits them as optional
// [0..*], so the fallback is harmless even where a section's own archetype
// doesn't strictly SHALL for it. Reorders each assignedEntity found since a
// fallback inserted after assignedPerson/representedOrganization already
// exist (the common case: name fields run before this) would otherwise land
// in the wrong schema position.
func ensureAssignedEntityIDs(entryEl *etree.Element) {
	for _, ae := range entryEl.FindElements(".//assignedEntity") {
		if ae.SelectElement("id") == nil {
			ae.CreateElement("id").CreateAttr("nullFlavor", "UNK")
		}
		if ae.SelectElement("addr") == nil {
			ae.CreateElement("addr").CreateAttr("nullFlavor", "UNK")
		}
		if ae.SelectElement("telecom") == nil {
			ae.CreateElement("telecom").CreateAttr("nullFlavor", "UNK")
		}
		reorderChildrenByTag(ae, []string{"id", "code", "addr", "telecom", "assignedPerson", "representedOrganization"})
	}
}

// ensureAuthorParticipationShape completes an <author> element to satisfy
// CDA's base Author participation content model (templateId*, time,
// assignedAuthor — confirmed against the same document-level <author> this
// package already writes in writeAuthorHeader, which always includes
// <time>) once Author Participation's own templateId (2.16.840.1.113883.
// 10.20.22.4.119) has been injected onto it. time is schema-required but
// this engine's entry-level author fields only ever supply who (given/
// family/NPI), never when — unlike the document-level author's real
// build-time nowCDATimestamp value — so nullFlavor="UNK" is used instead,
// the same idiom ensureAssignedEntityIDs already uses for "present but not
// known". Also locally reorders templateId/time/assignedAuthor: time may be
// added here AFTER assignedAuthor already exists (assignedAuthor is created
// earlier, during buildEntry's own Fields loop, when an authorGiven/
// authorFamily field first writes into it), and structuralElementOrder
// doesn't cover either tag, so the general reorder pass wouldn't fix their
// relative order on its own.
func ensureAuthorParticipationShape(authorEl *etree.Element) {
	if authorEl == nil {
		return
	}
	if authorEl.SelectElement("time") == nil {
		authorEl.CreateElement("time").CreateAttr("nullFlavor", "UNK")
	}
	reorderChildrenByTag(authorEl, []string{"templateId", "time", "assignedAuthor"})

	// assignedAuthor's own sequence (id*, code?, addr*, telecom*,
	// assignedPerson, representedOrganization?) needs the same reorder
	// writeAuthorHeader already applies to the document-level author —
	// entry-level authorGiven/authorFamily/authorNPI fields are plain
	// schema-driven WriteAtXPath writes (unlike the document-level author's
	// dedicated writeNPI() call), so id naturally lands AFTER assignedPerson
	// (created first, by authorGiven) purely by field-list order. Confirmed
	// via a live Test Pipeline run (2026-07) producing a schema-invalid
	// <assignedAuthor><assignedPerson>...</assignedPerson><id .../></assignedAuthor>.
	if assignedAuthor := authorEl.SelectElement("assignedAuthor"); assignedAuthor != nil {
		reorderChildrenByTag(assignedAuthor, []string{"id", "code", "addr", "telecom", "assignedPerson", "assignedAuthoringDevice", "representedOrganization"})
	}
}

// writeRepeatingGroups is the loop engine: for each RepeatingGroup a section
// declares, checks whether record[group.Key] is a canonical array
// (map_to_canonical's GroupBy — or a hand-authored pipeline field — is what
// actually produces one); if absent, this is a complete no-op, so every
// section not using RepeatingGroups is byte-for-byte unaffected by this
// function's existence. If present, creates one brand-new WrapperTag sibling
// per array item that passes RequiredPaths, reusing writeFieldValue — the
// exact same field-writing primitive every flat (non-repeating) section
// already uses.
//
// Deliberately uses direct etree creation (anchorEl.CreateElement), NOT
// WriteAtXPath's "tag[N]" positional predicate: every item here always wants
// a genuinely NEW wrapper element, never a find-or-reuse of an existing one
// (unlike a plain field's own xpath, which a second field write on the SAME
// item must re-resolve to the SAME element). A positional predicate would
// count ALL same-tag children of anchorEl regardless of what created them —
// safe only as long as nothing else on the section ever creates a same-tag
// sibling; WrapperAttr/WrapperAttrValue exists precisely so WrapperTag can
// safely be a bare, non-predicated tag (e.g. "entryRelationship") shared
// with other, unrelated same-tag elements elsewhere on the same section
// (e.g. Medication Activity's REFR/CAUS/SUBJ entryRelationships) without any
// risk of miscounting into one of those.
//
// rootEl/obsEl are the section's own two anchors buildEntry already
// resolved (may be nil if the section has no EntryElementPath/
// ObservationElementPath at all); group.AnchorRoot picks which one
// WrapperTag attaches under.
func writeRepeatingGroups(rootEl, obsEl *etree.Element, sec *cdaSchema.CDASectionDef, record map[string]interface{}) {
	for _, group := range sec.RepeatingGroups {
		items, ok := record[group.Key].([]interface{})
		if !ok {
			continue
		}

		anchorEl := rootEl
		if group.AnchorRoot == "observation" {
			anchorEl = obsEl
		}
		if anchorEl == nil {
			continue // section has no matching anchor to attach the group under — nothing to do
		}

		for _, itemRaw := range items {
			item, ok := itemRaw.(map[string]interface{})
			if !ok {
				continue
			}
			if !requiredPathsPresent(item, group.RequiredPaths) {
				continue
			}

			wrapperEl := anchorEl.CreateElement(group.WrapperTag)
			if group.WrapperAttr != "" {
				wrapperEl.CreateAttr(group.WrapperAttr, group.WrapperAttrValue)
			}

			targetEl := wrapperEl
			if group.ObservationElementPath != "" {
				targetEl = WriteAtXPath(wrapperEl, group.ObservationElementPath, "")
				applyTagBoilerplate(targetEl, lastSegmentTag(group.ObservationElementPath))
				if group.TemplateID != "" {
					injectTemplateID(targetEl, group.TemplateID, group.TemplateIDExt)
				}
			}

			for _, field := range group.Fields {
				writeFieldValue(targetEl, field, item)
			}

			if group.FixedCode != "" && targetEl.SelectElement("code") == nil {
				codeEl := targetEl.CreateElement("code")
				codeEl.CreateAttr("code", group.FixedCode)
				if group.FixedCodeSystem != "" {
					codeEl.CreateAttr("codeSystem", group.FixedCodeSystem)
				}
				if group.FixedCodeDisplay != "" {
					codeEl.CreateAttr("displayName", group.FixedCodeDisplay)
				}
			}
			if group.AuthorTemplateID != "" {
				if authorEl := targetEl.SelectElement("author"); authorEl != nil {
					injectTemplateID(authorEl, group.AuthorTemplateID, group.AuthorTemplateIDExt)
					ensureAuthorParticipationShape(authorEl)
				}
			}
			ensureGeneratedID(targetEl)
			// See structuralElementOrder's doc comment — templateId/code/id
			// here were built up across three separate steps (anchor
			// resolution, the fields loop, FixedCode, ensureGeneratedID),
			// none of which land in schema order by construction alone.
			reorderChildrenByTag(targetEl, structuralElementOrder)
		}
	}
}

// ensureGeneratedID gives el a system-generated <id root="<uuid>"/> if it
// doesn't already have one — every C-CDA act/observation/organizer instance
// SHALL have at least one id (confirmed missing across the board via a real
// schema validator run against this builder's own output, 2026-07). A
// random UUID as the sole root is a standard, self-sufficient identifier
// (no registered enterprise OID needed) and is safe to regenerate on every
// build since these ids identify THIS clinical statement instance, not a
// real-world identifier a downstream system needs to correlate against
// (unlike patient/author ids, which come from real source data via
// header_fields.go and are deliberately NOT auto-generated here).
func ensureGeneratedID(el *etree.Element) {
	if el == nil || el.SelectElement("id") != nil {
		return
	}
	el.CreateElement("id").CreateAttr("root", uuid.NewString())
}

// requiredPathsPresent reports whether record has a non-empty value at every
// one of keys — the RepeatingGroup.RequiredPaths gate (don't build an empty
// or junk repeated instance).
func requiredPathsPresent(record map[string]interface{}, keys []string) bool {
	for _, k := range keys {
		if _, ok := stringValue(record[k]); !ok {
			return false
		}
	}
	return true
}

// writeFieldValue writes a single field's primary + companion (Display/
// System/Unit/Family) values via WriteAtXPath, mirroring
// generic_section_processor.go's extractValueByXPath field-key convention
// exactly, in reverse. Whenever any of these writes lands on an element
// literally named "value" (CDA's polymorphic ANY-typed element — SHALL
// carry an explicit xsi:type before it may carry attributes at all,
// confirmed via a real schema validator run against this builder's own
// output, 2026-07), the field's own DataType is injected as xsi:type.
func writeFieldValue(entryEl *etree.Element, field *cdaSchema.CDAFieldDef, record map[string]interface{}) {
	if field.SkipIfXPathPresent != "" {
		if _, found := TryFindAtXPath(entryEl, stripEntryPrefix(field.SkipIfXPathPresent)); found {
			return
		}
	}
	if v, ok := stringValue(record[field.Key]); ok && field.XPath != "" {
		el := WriteAtXPath(entryEl, stripEntryPrefix(field.XPath), v)
		injectValueXsiType(el, field)
		injectNPIRootForAssignedIdentity(el)
	}
	if field.XPathDisplay != "" {
		if v, ok := stringValue(record[field.Key+"Display"]); ok {
			el := WriteAtXPath(entryEl, stripEntryPrefix(field.XPathDisplay), v)
			injectValueXsiType(el, field)
		}
	}
	if field.XPathSystem != "" {
		if v, ok := stringValue(record[field.Key+"System"]); ok {
			el := WriteAtXPath(entryEl, stripEntryPrefix(field.XPathSystem), v)
			injectValueXsiType(el, field)
			injectCodeSystemName(el, v)
		}
	}
	if field.XPathUnit != "" {
		if v, ok := stringValue(record[field.Key+"Unit"]); ok {
			el := WriteAtXPath(entryEl, stripEntryPrefix(field.XPathUnit), v)
			injectValueXsiType(el, field)
		}
	}
	if field.XPathFamily != "" {
		if v, ok := stringValue(record[field.Key+"Family"]); ok {
			WriteAtXPath(entryEl, stripEntryPrefix(field.XPathFamily), v)
		}
	}
}

// injectNPIRootForAssignedIdentity defaults @root to npiOID when a field
// write creates a bare <id> element (no root yet) whose parent is
// assignedAuthor or assignedEntity — every person/entity identity field in
// this schema (authorNPI, performerNPI, ...) targets a bare ".../id/@extension"
// path with no way to also supply a companion root (unlike XPathSystem's
// code/codeSystem pairing), so without this the element is left
// schema-incomplete: an <id> with @extension but no @root. Scoped
// specifically to assignedAuthor/assignedEntity parents — NOT applied
// blanket to every "id/@extension" field, since a few (e.g. Payers'
// participantRole/id for member/group numbers) are genuinely NOT
// NPI-rooted and must be left alone. Confirmed via a live Test Pipeline run
// (2026-07): entry-level author/performer NPI fields are written by plain
// WriteAtXPath like any other field (unlike the document-level author's
// dedicated writeNPI() call, which always sets root+extension together).
func injectNPIRootForAssignedIdentity(el *etree.Element) {
	if el == nil || el.Tag != "id" || el.SelectAttrValue("root", "") != "" {
		return
	}
	parent := el.Parent()
	if parent == nil || (parent.Tag != "assignedAuthor" && parent.Tag != "assignedEntity") {
		return
	}
	el.CreateAttr("root", npiOID)
	attrs := el.Attr
	for i, a := range attrs {
		if a.Key == "root" && i > 0 {
			moved := a
			copy(attrs[1:i+1], attrs[0:i])
			attrs[0] = moved
			break
		}
	}
}

// injectValueXsiType sets xsi:type on el when el is literally a <value>
// element and doesn't already carry one. field.DataType is already an HL7
// V3 datatype name (CD, PQ, CE, IVL_PQ, ...) per its own doc comment in
// schema_types.go, so this is a direct passthrough for every concrete type —
// "ANY" is this schema's own placeholder for "figure it out from shape",
// disambiguated by whether the field also has a companion XPathUnit (a
// physical quantity, PQ) versus not (a coded value, CD).
//
// May run after @code/@displayName/etc. were already set by an earlier
// field write reusing this same element (e.g. a field's primary xpath sets
// @code, then its own XPathDisplay call sets @displayName on the same
// <value>) — real CDA documents conventionally write xsi:type as <value>'s
// FIRST attribute, so the new attribute is moved to the front rather than
// left wherever CreateAttr's plain append landed.
func injectValueXsiType(el *etree.Element, field *cdaSchema.CDAFieldDef) {
	if el == nil || el.Tag != "value" || el.SelectAttrValue("xsi:type", "") != "" {
		return
	}
	dataType := field.DataType
	if dataType == "" || dataType == "ANY" {
		if field.XPathUnit != "" {
			dataType = "PQ"
		} else {
			dataType = "CD"
		}
	}
	el.CreateAttr("xsi:type", dataType)
	attrs := el.Attr
	for i, a := range attrs {
		if a.FullKey() == "xsi:type" && i > 0 {
			moved := a
			copy(attrs[1:i+1], attrs[0:i])
			attrs[0] = moved
			break
		}
	}
}

// knownCodeSystemNames maps the handful of codeSystem OIDs this engine's
// fields actually reference to their conventional human-readable label —
// the same labels real C-CDA IG worked examples use (e.g. Figure 219's own
// <value ... codeSystem="2.16.840.1.113883.6.96" codeSystemName="SNOMED CT" />).
// codeSystemName is a fixed, well-known convenience label for a given OID,
// never independent per-record data, so deriving it here (once, centrally)
// is DRY compared to asking every section to also map a
// "<field>SystemNameSystemName" companion for a value that's 1:1 with the
// codeSystem OID it already supplies.
var knownCodeSystemNames = map[string]string{
	"2.16.840.1.113883.6.96":       "SNOMED CT",
	"2.16.840.1.113883.6.1":        "LOINC",
	"2.16.840.1.113883.6.88":       "RxNorm",
	"2.16.840.1.113883.6.90":       "ICD-10-CM",
	"2.16.840.1.113883.6.103":      "ICD-9-CM",
	"2.16.840.1.113883.6.104":      "ICD-9-CM",
	"2.16.840.1.113883.12.292":     "CVX",
	"2.16.840.1.113883.5.4":        "ActCode",
	"2.16.840.1.113883.5.14":       "ActStatus",
	"2.16.840.1.113883.5.6":        "ActClass",
	"2.16.840.1.113883.5.1":        "AdministrativeGender",
	"2.16.840.1.113883.6.238":      "Race & Ethnicity - CDC",
	"2.16.840.1.113883.11.20.9.38": "Current Smoking Status",
	"2.16.840.1.113883.11.20.9.41": "Tobacco Use",
	"2.16.840.1.113883.6.12":       "CPT",
	"2.16.840.1.113883.6.26":       "UNII",
	"2.16.840.1.113883.6.259":      "HealthcareServiceLocation",
	"2.16.840.1.113883.3.26.1.1":   "NCI Thesaurus",
	"2.16.840.1.113883.6.101":      "NUCC Health Care Provider Taxonomy",
	"2.16.840.1.113883.5.1063":     "HL7ObservationValue",
	"2.16.840.1.113883.5.83":       "ObservationInterpretation",
	"2.16.840.1.113883.5.111":      "HL7RoleCode",
}

// injectCodeSystemName sets codeSystemName on el (the same element a field's
// XPathSystem write just set codeSystem on) when the codeSystem value is one
// of this engine's known OIDs — a no-op for unrecognized OIDs (never guesses)
// or when codeSystemName is already present (never overwrites real data).
func injectCodeSystemName(el *etree.Element, codeSystemOID string) {
	if el == nil || el.SelectAttrValue("codeSystemName", "") != "" {
		return
	}
	if name, ok := knownCodeSystemNames[codeSystemOID]; ok {
		el.CreateAttr("codeSystemName", name)
	}
}

func applyTagBoilerplate(el *etree.Element, tag string) {
	bp, ok := tagBoilerplate[tag]
	if !ok {
		return
	}
	if el.SelectAttrValue("classCode", "") == "" {
		el.CreateAttr("classCode", bp.ClassCode)
	}
	if el.SelectAttrValue("moodCode", "") == "" {
		el.CreateAttr("moodCode", bp.MoodCode)
	}
	if bp.StatusCode != "" && el.SelectElement("statusCode") == nil {
		el.CreateElement("statusCode").CreateAttr("code", bp.StatusCode)
	}
}

// injectTemplateID adds (or, if a field write already created one via a
// "[templateId/@root='...']" disambiguating predicate — see
// StructuralTemplateAnchor — completes) a <templateId root extension>
// child. Reuses an existing <templateId> with a matching root instead of
// appending a duplicate, since exactly one is ever valid per element.
func injectTemplateID(el *etree.Element, oid string, extension string) {
	var tid *etree.Element
	for _, existing := range el.SelectElements("templateId") {
		if existing.SelectAttrValue("root", "") == oid {
			tid = existing
			break
		}
	}
	if tid == nil {
		tid = el.CreateElement("templateId")
		tid.CreateAttr("root", oid)
	}
	if extension != "" && tid.SelectAttrValue("extension", "") == "" {
		tid.CreateAttr("extension", extension)
	}
	ensureR1CompatTemplateID(el, oid, extension)
}

// ensureR1CompatTemplateID appends a second, bare (no @extension)
// <templateId root="oid"/> sibling whenever a dated @extension is used —
// CONF:1198-32936/32934 (repeated on nearly every entry/section/document
// template in a real schema validator run, 2026-07): "all C-CDA R2.1
// document, section, and entry templates that had a previous version in
// C-CDA R1.1 SHALL include both the C-CDA R2.1 templateId [with extension]
// and the C-CDA R1.1 templateId root [without extension]".
//
// Verified directly against the real 2012 R1.1 IG (not guessed): its own
// "Entry Change Tracking Table" (Table 319) shows the exact same
// 2.16.840.1.113883.10.20.22.4.x / .2.x / .1.x numbering was introduced BY
// R1.1 itself in 2012 (replacing older, separate CCD/IHE-PCC/HITSP OIDs) —
// R2.1 (2015) never renumbered these roots, it only added dated @extension
// attributes on top of them. Confirmed with literal `<templateId root="...">`
// examples straight from the R1.1 IG's own body text for every archetype
// spot-checked this session (Advance Directive .4.48, Encounter Activities
// .4.49, Vital Signs Organizer .4.26, Vital Sign Observation .4.27,
// Immunization Activity .4.52, Functional Status Observation .4.67,
// Cognitive/Mental Status Observation .4.74, plus Table 319's own listing
// for Allergy .4.7/.4.8/.4.9, Problem Concern Act .4.3, Problem Observation
// .4.4, Problem Status .4.6, Result Organizer .4.1, Result Observation .4.2,
// Medication Activity .4.16, Indication .4.19, Medication Information
// .4.23, and the CCD document templates .1.1/.1.2 themselves) — so the
// "same root, no extension" companion is the CORRECT R1.1 identity in every
// case checked, not a guess.
//
// Deliberately gated on extension being non-empty rather than a separate
// per-call opt-out flag: a genuinely NEW R2.1-only template (no R1.1
// predecessor at all — the only confirmed case in this schema is Nutrition,
// verified via a zero-hit search of the entire R1.1 IG for its OIDs) simply
// has no dated @extension in this schema's own data either, since there was
// no earlier version to date against — so the guard naturally excludes it
// without needing a second field to keep in sync.
func ensureR1CompatTemplateID(el *etree.Element, oid string, extension string) {
	if el == nil || oid == "" || extension == "" {
		return
	}
	for _, existing := range el.SelectElements("templateId") {
		if existing.SelectAttrValue("root", "") == oid && existing.SelectAttrValue("extension", "") == "" {
			return
		}
	}
	el.CreateElement("templateId").CreateAttr("root", oid)
}

// stripEntryPrefix mirrors generic_section_processor.go's helper of the same
// name exactly (kept as a private copy here since builder and cdaparser are
// separate packages and this one-line helper isn't worth a shared import).
func stripEntryPrefix(xpath string) string {
	const prefix = "entry/"
	if strings.HasPrefix(xpath, prefix) {
		return xpath[len(prefix):]
	}
	return xpath
}

// lastSegmentTag returns the tag name of the LAST path segment (stripping
// any "entry/" prefix and any "[predicate]"), e.g.
// "entry/act/entryRelationship[@typeCode='SUBJ']/observation" -> "observation".
func lastSegmentTag(path string) string {
	segments := splitPathSegments(stripEntryPrefix(path))
	if len(segments) == 0 {
		return ""
	}
	last := segments[len(segments)-1]
	if i := strings.Index(last, "["); i != -1 {
		return last[:i]
	}
	return last
}

// stringValue converts a canonical record value to a string, returning
// ok=false for nil/missing/empty values so callers skip writing them.
func stringValue(v interface{}) (string, bool) {
	if v == nil {
		return "", false
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

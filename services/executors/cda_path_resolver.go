// services/executors/cda_path_resolver.go
//
// Phase 1 of the CDA→FHIR Declarative Mapping Engine (see
// architecture/CDA_FHIR_MAPPING_ENGINE_SPRINT_PLAN.md, "Phase 1 — Path
// Resolution Language" and its design note) — wildcard/predicate traversal
// for CDA's recursive/repeating structure, addressing both the typed
// "document.*" tree (PathTypeCDADocument) and the generic "xml.*" mirror
// (PathTypeCDAXMLMirror).
//
// Today's scattered alternative — services/cda_fhir/mappers/helpers.go's
// findRelByTypeCode, medication_mapper.go's hasTemplateID, and every
// mapper's own hand-rolled "for _, rel := range entry.EntryRelationships"
// loop — is exactly what this resolver replaces for declarative mapping
// rules. It does not touch those call sites (Phase 3's job); it gives Phase
// 2's engine a single, tested primitive to call instead of writing new ones.
package executors

import "strings"

// cdaPredicateKeys is the fixed, closed set of RIM discriminators the
// resolver understands inside a "[key=value]" bracket. Deliberately not a
// general expression language — see the sprint plan's Performance
// checklist ("Predicate matching must be O(1) equality/map-lookup ... not a
// general expression evaluator").
//
// "code" and "xsiType" were added during Phase 3 (OOB Template Migration),
// not Phase 1/2, for two concrete needs surfaced while porting real mapper
// logic to declarative rows — not spec'd in advance, per the Phase 1 design
// note's own "revisit only if a concrete need surfaces" caution:
//   - "code": several C-CDA templates discriminate by a *fixed* act/observation
//     code rather than (or in addition to) templateId — e.g. the Severity
//     Observation's code is fixed to "SEV" (CONF:1098-19169), the only
//     reliable way condition_mapper.go/allergy_mapper.go find the right
//     nested SUBJ observation among possibly-several. Unlike
//     typeCode/classCode/moodCode/inversionInd (bare attributes on the node
//     itself), "code" is a nested CD/CE element on both backends, so it needs
//     its own matcher (matchesCDACodeField), the same way "templateId" already
//     does.
//   - "xsiType": CDAEffectiveTimeEntry's "xsiType" tag distinguishes the
//     duration entry (IVL_TS) from the dosing-frequency entry (PIVL_TS/EIVL_TS)
//     on a substanceAdministration — medication_mapper.go's applyDosageTiming
//     needs to select "the PIVL_TS/EIVL_TS effectiveTime", which a flat
//     "effectiveTimes[*]" wildcard can't discriminate. Unlike "code", this is
//     a bare string field on the node itself, so it needs no special-casing —
//     it falls through the existing generic attribute-equality branch.
//   - "type" (added porting Vital Signs/Results/Social History): CDAValue's
//     own "type" JSON tag (its xsi:type discriminator: "PQ"|"CD"|"CE"|"CS"|
//     "ST"|"ED"|"BL"|"INT"|"REAL"|"IVL_TS") — observation_mapper.go's
//     setObservationValue switches on exactly this field to pick
//     valueQuantity/valueCodeableConcept/valueString/etc. A DIFFERENT field
//     from "xsiType" above: CDAEffectiveTimeEntry.XSIType's JSON tag is
//     literally "xsiType", while CDAValue.Type's JSON tag is "type" — same
//     xsi:type concept on the CDA side, two different Go field names, so
//     this needs its own key. Also a bare string field on the node itself —
//     no special-casing, same generic attribute-equality branch.
//   - "entryType" (added porting CarePlan/Goal): CDAEntry's own "entryType"
//     JSON tag ("procedure"|"observation"|"act"|"encounter"|"supply"|
//     "substanceAdministration"|...) — plan_of_care_mapper.go's
//     dispatchPlanOfCareEntry switches on exactly this field (after
//     moodCode) to route one Plan-of-Care entry to ServiceRequest/
//     Appointment/SupplyRequest/MedicationRequest. This is the SAME
//     dispatch shape Medications' EntryMatch="moodCode=INT" already proved
//     for BuildResourcesForRules, just discriminating on a different bare
//     field — without this key, an EntryMatch like
//     "entryType=substanceAdministration" would silently match nothing per
//     parseCDAPredicates' "unknown keys dropped" rule, then (worse) an
//     EMPTY predicate list matches every entry unconditionally, which would
//     have made the whole multi-rule dispatch silently wrong instead of
//     loudly broken. Also a bare string field on the node itself — no
//     special-casing, same generic attribute-equality branch.
var cdaPredicateKeys = map[string]struct{}{
	"typeCode":       {},
	"inversionInd":   {},
	"templateId":     {},
	"classCode":      {},
	"moodCode":       {},
	"code":           {},
	"xsiType":        {},
	"type":           {},
	"entryType":      {},
	// refrTemplateId checks whether any entryRelationship[typeCode=REFR].entry
	// carries the given templateId -- used by HealthConcernsMappingRules to
	// distinguish Assessment Scale Observations from Problem Observations when
	// both appear as REFR-linked entries inside a Health Concern Act, since
	// the outer act itself always carries the same .4.132 templateId regardless
	// of what's inside. A plain [templateId=X] predicate only checks the
	// CURRENT entry node's own templateIds and cannot reach nested entries.
	"refrTemplateId": {},
}

// cdaSegmentKind identifies how a path segment's optional "[...]" suffix
// should narrow/fan out the current node set during resolution.
type cdaSegmentKind int

const (
	segmentPlain     cdaSegmentKind = iota // no bracket: "entry", "value"
	segmentWildcard                        // "[*]"
	segmentIndex                           // "[0]", "[12]"
	segmentPredicate                       // "[typeCode=SUBJ]" or a compound AND
)

// cdaPredicate is one "key=value" clause inside a "[...]" bracket.
type cdaPredicate struct {
	key   string
	value string
}

// cdaPathSegment is one dot-separated component of a wildcard/predicate CDA
// path, e.g. "entryRelationships[typeCode=SUBJ,inversionInd=true]" parses to
// {key: "entryRelationships", kind: segmentPredicate, predicates: [...]}.
type cdaPathSegment struct {
	key        string
	kind       cdaSegmentKind
	index      int
	predicates []cdaPredicate
}

// parseCDAPath splits a dot-separated CDA path into segments, extracting
// each segment's optional "[...]" suffix. Unrecognised/malformed bracket
// content degrades to a plain literal-key segment (which simply never
// matches anything) rather than erroring — resolution fails closed, never
// panics, on malformed input.
func parseCDAPath(path string) []cdaPathSegment {
	rawParts := splitCDAPathOnDots(path)
	segments := make([]cdaPathSegment, 0, len(rawParts))
	for _, raw := range rawParts {
		if raw == "" {
			continue
		}
		segments = append(segments, parseCDAPathSegment(raw))
	}
	return segments
}

// splitCDAPathOnDots splits path on '.' the way every other segment
// boundary in this grammar expects, with one deliberate exception: a '.'
// inside an unmatched "[...]" bracket does not split the segment. CDA
// templateId values are themselves dot-separated OIDs (e.g.
// "[templateId=2.16.840.1.113883.10.20.22.4.147]") — splitting naively on
// every '.' would shred that OID into bogus segments. Bracket-depth
// tracking is sufficient (not a general parser) because predicate values
// never themselves contain '[' or ']'.
func splitCDAPathOnDots(path string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(path); i++ {
		switch path[i] {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case '.':
			if depth == 0 {
				parts = append(parts, path[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, path[start:])
	return parts
}

func parseCDAPathSegment(raw string) cdaPathSegment {
	open := strings.IndexByte(raw, '[')
	if open < 0 || !strings.HasSuffix(raw, "]") {
		return cdaPathSegment{key: raw, kind: segmentPlain}
	}
	key := raw[:open]
	body := raw[open+1 : len(raw)-1]

	if body == "*" {
		return cdaPathSegment{key: key, kind: segmentWildcard}
	}

	if idx, ok := parseCDANonNegativeInt(body); ok {
		return cdaPathSegment{key: key, kind: segmentIndex, index: idx}
	}

	if preds := parseCDAPredicates(body); len(preds) > 0 {
		return cdaPathSegment{key: key, kind: segmentPredicate, predicates: preds}
	}

	// Unrecognised bracket content (malformed predicate, unknown key, empty
	// body, etc.) — treat the whole token as a literal key that will never
	// match real data, instead of guessing or erroring.
	return cdaPathSegment{key: raw, kind: segmentPlain}
}

func parseCDANonNegativeInt(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

// parseCDAPredicates parses a comma-separated "key=value" list. A predicate
// whose key is outside cdaPredicateKeys is dropped silently — unknown keys
// can never match real CDA data, so the clause behaves as if it weren't
// written rather than as a parse error.
func parseCDAPredicates(body string) []cdaPredicate {
	var preds []cdaPredicate
	for _, clause := range strings.Split(body, ",") {
		clause = strings.TrimSpace(clause)
		eq := strings.IndexByte(clause, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(clause[:eq])
		value := strings.TrimSpace(clause[eq+1:])
		if _, ok := cdaPredicateKeys[key]; !ok {
			continue
		}
		preds = append(preds, cdaPredicate{key: key, value: value})
	}
	return preds
}

// ===============================================================
// NODE-SET RESOLUTION
// ===============================================================

// ResolveCDAPaths walks path against root (either the typed "document.*"
// tree or the generic "xml.*" mirror, both already JSON-round-tripped to
// map[string]interface{}/[]interface{}/string/etc.) and returns every
// matching node — an XPath-style node-set, not a single value. Plain and
// numeric-index segments narrow the set 1:1 per input node; "[*]" and
// predicate segments can fan a single input node out to zero, one, or many
// output nodes.
//
// isXMLMirror selects which backend's attribute/templateId convention the
// predicate matcher uses — true for the "xml.*" generic mirror, false for
// the typed "document.*" tree. See matchesCDAPredicate.
func ResolveCDAPaths(root interface{}, path string, isXMLMirror bool) []interface{} {
	segments := parseCDAPath(path)
	nodes := []interface{}{root}

	for _, seg := range segments {
		var next []interface{}
		for _, node := range nodes {
			nodeMap, ok := node.(map[string]interface{})
			if !ok {
				continue
			}
			child, exists := nodeMap[seg.key]
			if !exists {
				continue
			}

			switch seg.kind {
			case segmentWildcard:
				next = append(next, asCDANodeList(child)...)
			case segmentIndex:
				list := asCDANodeList(child)
				if seg.index >= 0 && seg.index < len(list) {
					next = append(next, list[seg.index])
				}
			case segmentPredicate:
				for _, candidate := range asCDANodeList(child) {
					if candMap, ok := candidate.(map[string]interface{}); ok &&
						matchesCDAPredicates(candMap, seg.predicates, isXMLMirror) {
						next = append(next, candidate)
					}
				}
			default: // segmentPlain
				next = append(next, child)
			}
		}
		nodes = next
		if len(nodes) == 0 {
			return nil
		}
	}
	return nodes
}

// ResolveCDAPath returns the first node ResolveCDAPaths finds, or nil. This
// is the single-value contract GetFieldValue's "document."/"xml." dispatch
// already promises callers: pre-existing plain/numeric-index paths resolve
// identically to before; wildcard/predicate paths now additionally resolve
// instead of silently returning nil.
func ResolveCDAPath(root interface{}, path string, isXMLMirror bool) interface{} {
	nodes := ResolveCDAPaths(root, path, isXMLMirror)
	if len(nodes) == 0 {
		return nil
	}
	return nodes[0]
}

// ResolveCDAFallback tries each path in order via ResolveCDAPath, returning
// the first result that resolves to a non-nil, non-empty-string value. This
// is the "field A, else field B" primitive that
// architecture/CDA_FHIR_MAPPING_INVENTORY.md's cross-cutting finding #1 asks
// for by name — e.g. Allergy's CSM-participant-code falling back to the
// assertion observation's own value when the CSM participant is absent
// (allergy_mapper.go:93-97) — as a named, testable function instead of an
// inline if/else duplicated per mapper.
func ResolveCDAFallback(root interface{}, isXMLMirror bool, paths ...string) interface{} {
	for _, path := range paths {
		if v := ResolveCDAPath(root, path, isXMLMirror); v != nil && v != "" {
			return v
		}
	}
	return nil
}

// asCDANodeList normalises a resolved child value into a slice for
// wildcard/index/predicate iteration. Handles the one real structural
// asymmetry between the two backends this resolver supports:
// GenericXMLToJSON (cda/document/generic_xml.go) collapses a single child
// occurrence to a bare object instead of a one-element array
// ("auto-array-on-repeat", so the common case stays readable) while
// encoding/json always serialises a Go slice as a JSON array regardless of
// length. Treating a bare object as an implicit one-element list makes
// "[*]"/predicate matching behave identically on both backends regardless of
// how many times the source element actually repeated.
func asCDANodeList(v interface{}) []interface{} {
	switch t := v.(type) {
	case []interface{}:
		return t
	case nil:
		return nil
	default:
		return []interface{}{t}
	}
}

// matchesCDAPredicates evaluates an AND of all predicates against node.
func matchesCDAPredicates(node map[string]interface{}, predicates []cdaPredicate, isXMLMirror bool) bool {
	for _, p := range predicates {
		if !matchesCDAPredicate(node, p, isXMLMirror) {
			return false
		}
	}
	return true
}

// matchesCDAPredicate evaluates one "key=value" clause against node. Each
// check is a single O(1) map lookup (plus, for "templateId" only, an O(k)
// membership/equality scan over that one node's own templateIds — typically
// 1-3 entries, never a document-wide scan).
func matchesCDAPredicate(node map[string]interface{}, p cdaPredicate, isXMLMirror bool) bool {
	if p.key == "templateId" {
		return matchesCDATemplateID(node, p.value, isXMLMirror)
	}
	if p.key == "refrTemplateId" {
		return matchesCDAREFRTemplateID(node, p.value, isXMLMirror)
	}
	if p.key == "code" {
		return matchesCDACodeField(node, p.value, isXMLMirror)
	}

	attrKey := p.key
	if isXMLMirror {
		// GenericXMLToJSON prefixes every XML attribute with "@" — typeCode,
		// classCode, moodCode, inversionInd are all act/participation
		// attributes, never child elements, on both backends.
		attrKey = "@" + p.key
	}
	actual, ok := node[attrKey]
	if !ok {
		return false
	}
	return cdaPredicateValueEquals(actual, p.value)
}

// matchesCDATemplateID checks templateId membership using each backend's own
// shape: the typed document tree exposes a flat []string under
// "templateIds" (CDAEntry.TemplateIds' JSON tag); the generic XML mirror
// exposes one-or-more <templateId root="..."/> child elements under the
// "templateId" key, each carrying its root in "@root" — never a flat string
// list.
func matchesCDATemplateID(node map[string]interface{}, want string, isXMLMirror bool) bool {
	if isXMLMirror {
		for _, tid := range asCDANodeList(node["templateId"]) {
			tidMap, ok := tid.(map[string]interface{})
			if !ok {
				continue
			}
			if root, _ := tidMap["@root"].(string); root == want {
				return true
			}
		}
		return false
	}

	ids, _ := node["templateIds"].([]interface{})
	for _, id := range ids {
		if s, ok := id.(string); ok && s == want {
			return true
		}
	}
	return false
}

// matchesCDAREFRTemplateID checks whether any entryRelationship with
// typeCode="REFR" on the node contains an entry whose templateIds include want.
// This is the implementation backing the "refrTemplateId" predicate key —
// see cdaPredicateKeys' own doc comment for why a plain "templateId" predicate
// can't reach nested entries.
func matchesCDAREFRTemplateID(node map[string]interface{}, want string, isXMLMirror bool) bool {
	rels, _ := node["entryRelationships"].([]interface{})
	for _, rel := range rels {
		relMap, _ := rel.(map[string]interface{})
		if isXMLMirror {
			if tc, _ := relMap["@typeCode"].(string); tc != "REFR" {
				continue
			}
		} else {
			if tc, _ := relMap["typeCode"].(string); tc != "REFR" {
				continue
			}
		}
		entry, _ := relMap["entry"].(map[string]interface{})
		if entry != nil && matchesCDATemplateID(entry, want, isXMLMirror) {
			return true
		}
	}
	return false
}

// matchesCDACodeField checks whether node's own <code> element carries the
// given code value — a nested CD/CE on both backends, never a bare
// string/bool on the node itself (that's what makes it special-cased here
// the same way matchesCDATemplateID is): the typed tree's CDACode.Code JSON
// field, or the XML mirror's <code code="..."/> "@code" attribute. A node
// with no "code" element at all (or one shaped unexpectedly) simply doesn't
// match, the same fail-closed behavior as every other predicate here.
func matchesCDACodeField(node map[string]interface{}, want string, isXMLMirror bool) bool {
	codeNode, ok := node["code"].(map[string]interface{})
	if !ok {
		return false
	}
	key := "code"
	if isXMLMirror {
		key = "@code"
	}
	actual, _ := codeNode[key].(string)
	return actual == want
}

// cdaPredicateValueEquals compares a resolved attribute/field value (string
// or bool — the only two types typeCode/classCode/moodCode/inversionInd ever
// take on either backend) against the bracket's literal string operand.
//
// want may be a "|"-separated OR-list (e.g. "PRF|SPRF") — needed because
// real C-CDA data legitimately uses more than one typeCode for the same
// semantic role: procedure_mapper.go:54 already checks
// `p.TypeCode == "PRF" || p.TypeCode == "SPRF"` (Performer / Secondary
// Performer, both standard HL7 ParticipationType codes) when finding a
// Procedure's performer, a real Go && that a single-value predicate can't
// express. No existing predicate value contains a literal "|" (templateId
// OIDs are dot-separated digits; typeCode/classCode/moodCode/xsiType are
// short ActCode/XSI mnemonics) so this is purely additive — every existing
// caller's single-value "want" splits into a 1-element list and behaves
// identically.
func cdaPredicateValueEquals(actual interface{}, want string) bool {
	for _, w := range strings.Split(want, "|") {
		if cdaPredicateValueEqualsOne(actual, w) {
			return true
		}
	}
	return false
}

func cdaPredicateValueEqualsOne(actual interface{}, want string) bool {
	switch v := actual.(type) {
	case string:
		return v == want
	case bool:
		return (want == "true" && v) || (want == "false" && !v)
	default:
		return false
	}
}

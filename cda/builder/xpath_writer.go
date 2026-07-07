// cda/builder/xpath_writer.go
//
// WriteAtXPath is the construction-direction mirror of
// services/parsers/cda/generic_section_processor.go's extractValueByXPath:
// that function reads a CDAFieldDef.XPath (as stored in ccda_2_1.json)
// relative to an <entry> element via etree.FindElement/SelectAttrValue; this
// file walks the SAME xpath strings and creates the elements/attributes
// instead, so cda.parse and cda.build share one schema-driven contract in
// both directions — adding a section or field is a schema change, not a new
// Go function on either side.
//
// Supported path grammar is deliberately the same restricted subset
// GenericSectionProcessor's isEtreeSafePredicate already accepts for
// reading (a predicate this engine can't safely read back could never have
// round-tripped anyway), extended with comma-separated multi-attribute
// conditions (needed because several real C-CDA entryRelationship elements
// are only uniquely identified by TWO attributes together, e.g.
// @typeCode='SUBJ' AND @inversionInd='true'):
//
//	tag                              - child element by name
//	tag/@attr                        - attribute on tag (leaf only)
//	tag[@attr='value']               - child matched/created by its OWN attribute
//	tag[child/@attr='value']         - child matched/created by a NESTED element's attribute
//	tag[@a='x',@b='y']               - child matched/created by TWO+ of its own attributes (AND)
//	tag[N]                           - child matched/created by 1-based position
package builder

import (
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

// WriteAtXPath walks path (relative to root, using the same "entry/"-stripped
// convention as extractValueByXPath) under root, creating each intermediate
// element/predicate-matched-child as needed, and returns the final element.
//
//   - When value is non-empty and the path's last segment is "@attrName",
//     the attribute is set on the final element (the one the attribute
//     belongs to) and that element is returned.
//   - When value is non-empty and the last segment is a plain tag, that
//     element's text is set.
//   - When value is "", no leaf is set — this is used to fetch-or-create a
//     pure structural anchor (e.g. an entry's root/nested-observation
//     element) for the caller to attach templateId/classCode/moodCode to
//     directly.
func WriteAtXPath(root *etree.Element, path string, value string) *etree.Element {
	if root == nil || path == "" {
		return root
	}

	segments := splitPathSegments(path)
	if len(segments) == 0 {
		return root
	}

	last := segments[len(segments)-1]
	if strings.HasPrefix(last, "@") {
		el := walkElements(root, segments[:len(segments)-1])
		if value != "" {
			el.CreateAttr(last[1:], value)
		}
		return el
	}

	el := walkElements(root, segments)
	if value != "" {
		el.SetText(value)
	}
	return el
}

// TryFindAtXPath is the read-only counterpart to WriteAtXPath: it walks path
// under root exactly the same way, but never creates anything — it returns
// (nil, false) as soon as any segment has no existing match. Used to inject
// a structural templateId only onto a nested node a real field write already
// created (e.g. a Reaction/Severity/Status Observation), never onto an
// empty container conjured up just to hold a templateId with no data.
func TryFindAtXPath(root *etree.Element, path string) (*etree.Element, bool) {
	if root == nil || path == "" {
		return nil, false
	}
	segments := splitPathSegments(path)
	if len(segments) == 0 {
		return root, true
	}
	last := segments[len(segments)-1]
	if strings.HasPrefix(last, "@") {
		segments = segments[:len(segments)-1]
		if len(segments) == 0 {
			return root, true
		}
	}
	return tryWalkElements(root, segments)
}

// splitPathSegments splits a "/"-delimited path into segments, respecting
// "[...]" predicate brackets (which may themselves contain "/", e.g.
// "observation[code/@code='ASSERTION']") so a predicate is never split
// across two segments — the construction-direction analogue of
// fhir_path_writer.go's findTopLevelDot, for "/" instead of ".".
func splitPathSegments(path string) []string {
	var segments []string
	depth := 0
	start := 0
	for i, ch := range path {
		switch ch {
		case '[':
			depth++
		case ']':
			depth--
		case '/':
			if depth == 0 {
				if i > start {
					segments = append(segments, path[start:i])
				}
				start = i + 1
			}
		}
	}
	if start < len(path) {
		segments = append(segments, path[start:])
	}
	return segments
}

// walkElements resolves each segment in turn, starting from root, creating
// elements as needed, and returns the final element.
//
// For a segment with a predicate, when there IS a remaining path after it,
// walkElements prefers an existing candidate sibling whose descendant chain
// ALREADY satisfies that remaining path (checked read-only, via
// tryWalkElements) over blindly reusing the first predicate match. This
// matters when two structurally distinct nested observations share the same
// wrapping predicate — e.g. Allergy's Severity and Status observations both
// nest under entryRelationship[@typeCode='SUBJ',@inversionInd='true'],
// distinguished only by their OWN nested templateId
// (observation[templateId/@root='...']) one level deeper. Without this
// lookahead, the second field to write would incorrectly reuse the first
// field's entryRelationship and end up with two observations crammed under
// one entryRelationship instead of two separate entryRelationship siblings.
// When the remaining path's next segment has no predicate of its own (e.g.
// plain "effectiveTime" or "participant"), every candidate trivially
// "already satisfies" an unconstrained lookup, so the first (only
// meaningful) candidate is reused as before — this is the common case (most
// fields under one section share one observation) and is unaffected.
func walkElements(root *etree.Element, segments []string) *etree.Element {
	cur := root
	for i, seg := range segments {
		cur = findOrCreateChild(cur, seg, segments[i+1:])
	}
	return cur
}

// tryWalkElements is walkElements' read-only counterpart, used both by
// TryFindAtXPath directly and by findOrCreateChild's own lookahead. Unlike a
// naive segment-by-segment walk, it backtracks across EVERY existing
// candidate at each level, not just the first predicate match — required
// because, e.g., Allergy's Status Observation lookup must skip past a
// same-predicate Severity Observation sibling (both nest under
// entryRelationship[@typeCode='SUBJ',@inversionInd='true']) and keep
// searching until it finds the one whose OWN nested templateId actually
// matches, rather than dead-ending on the first (wrong) sibling.
func tryWalkElements(root *etree.Element, segments []string) (*etree.Element, bool) {
	if len(segments) == 0 {
		return root, true
	}
	rest := segments[1:]
	for _, candidate := range candidatesForSegment(root, segments[0]) {
		if len(rest) == 0 {
			return candidate, true
		}
		if found, ok := tryWalkElements(candidate, rest); ok {
			return found, true
		}
	}
	return nil, false
}

// candidatesForSegment returns every existing child of parent matching one
// path segment ("tag", "tag[@attr='value']", "tag[child/@attr='value']",
// "tag[@a='x',@b='y']", or "tag[N]"), in document order. A bare tag (no
// predicate) only ever yields its first match — matching etree.FindElement's
// own single-match semantics on the read side (see xpath_writer.go's package
// doc comment on the known first-match limitation for unpredicated segments)
// — predicated segments yield every match, so backtracking can distinguish
// same-predicate siblings by what's nested inside them.
func candidatesForSegment(parent *etree.Element, segment string) []*etree.Element {
	tag, pred := splitPredicate(segment)

	if pred == "" {
		if el := parent.SelectElement(tag); el != nil {
			return []*etree.Element{el}
		}
		return nil
	}

	if n, err := strconv.Atoi(pred); err == nil {
		children := parent.SelectElements(tag)
		if n >= 1 && n <= len(children) {
			return []*etree.Element{children[n-1]}
		}
		return nil
	}

	if conds := parseConditions(pred); len(conds) > 0 {
		var out []*etree.Element
		for _, child := range parent.SelectElements(tag) {
			if allConditionsMatch(child, conds) {
				out = append(out, child)
			}
		}
		return out
	}

	if el := parent.SelectElement(tag); el != nil {
		return []*etree.Element{el}
	}
	return nil
}

// findOrCreateChild resolves one path segment against parent's children,
// creating a new child (with the predicate's constraint(s) already applied)
// if no existing candidate — checked via the SAME backtracking search
// tryWalkElements uses — already satisfies the rest of the path. See
// walkElements' doc comment for why this lookahead matters.
//
// The lookahead only ever runs when THIS segment itself carries a predicate
// (pred != ""): a bare tag's candidatesForSegment is capped at one match by
// construction (etree.SelectElement's own first-match semantics), so there
// is never more than one real candidate to disambiguate between — running
// the lookahead anyway would judge that lone candidate by whether it
// ALREADY has the remaining path fully built, which is false the very first
// time a second field starts writing through it (e.g. "act" itself, on a
// second field's call, would otherwise look like a non-match purely because
// the deeper node from THIS call hasn't been created yet) and incorrectly
// spawn a duplicate.
func findOrCreateChild(parent *etree.Element, segment string, remaining []string) *etree.Element {
	tag, pred := splitPredicate(segment)
	candidates := candidatesForSegment(parent, segment)

	if pred != "" && len(remaining) > 0 && segmentHasPredicate(remaining[0]) {
		for _, c := range candidates {
			if _, ok := tryWalkElements(c, remaining); ok {
				return c
			}
		}
		// No existing candidate already has the specific nested identity
		// `remaining` asks for — this is a distinct sibling instance (e.g. a
		// second entryRelationship for a different nested observation
		// type), not a reuse of an existing one. Fall through to create.
	} else if len(candidates) > 0 {
		return candidates[0]
	}

	if pred == "" {
		return parent.CreateElement(tag)
	}
	if _, err := strconv.Atoi(pred); err == nil {
		return parent.CreateElement(tag)
	}
	if conds := parseConditions(pred); len(conds) > 0 {
		el := parent.CreateElement(tag)
		for _, c := range conds {
			applyPredicateConstraint(el, c.lhs, c.rhs)
		}
		return el
	}
	// Unrecognized predicate form — degrade to a bare create by tag.
	return parent.CreateElement(tag)
}

// segmentHasPredicate reports whether a path segment carries a "[...]"
// predicate (as opposed to a bare tag name).
func segmentHasPredicate(segment string) bool {
	return strings.Contains(segment, "[")
}

// splitPredicate splits "tag[predicate]" into ("tag", "predicate"), or
// returns (segment, "") when there is no bracket.
func splitPredicate(segment string) (tag, pred string) {
	i := strings.Index(segment, "[")
	if i == -1 {
		return segment, ""
	}
	end := strings.LastIndex(segment, "]")
	if end == -1 || end < i {
		return segment, ""
	}
	return segment[:i], segment[i+1 : end]
}

// condition is one parsed "@attr='value'" or "childTag/@attr='value'" clause.
type condition struct{ lhs, rhs string }

// parseConditions splits a predicate body on "," into individual conditions
// — e.g. "@typeCode='SUBJ',@inversionInd='true'" (two conditions that must
// BOTH hold) — the same comma-separated-AND grammar already used elsewhere
// in this codebase for entry-match predicates (see MappingRule.EntryMatch,
// services/cda_fhir/declarative_schema.go).
func parseConditions(pred string) []condition {
	var conds []condition
	for _, part := range strings.Split(pred, ",") {
		part = strings.TrimSpace(part)
		eq := strings.Index(part, "=")
		if eq == -1 {
			continue
		}
		lhs := strings.TrimSpace(part[:eq])
		rhs := strings.Trim(strings.TrimSpace(part[eq+1:]), "'\"")
		conds = append(conds, condition{lhs, rhs})
	}
	return conds
}

func allConditionsMatch(el *etree.Element, conds []condition) bool {
	for _, c := range conds {
		if !predicateMatches(el, c.lhs, c.rhs) {
			return false
		}
	}
	return true
}

// predicateMatches checks whether an existing child element satisfies a
// single "@attr='value'" or "childTag/@attr='value'" condition already
// parsed into lhs ("@attr" or "childTag/@attr") and rhs ("value").
func predicateMatches(el *etree.Element, lhs, rhs string) bool {
	if strings.HasPrefix(lhs, "@") {
		return el.SelectAttrValue(lhs[1:], "") == rhs
	}
	if idx := strings.LastIndex(lhs, "/@"); idx >= 0 {
		childTag := lhs[:idx]
		attrName := lhs[idx+2:]
		child := el.SelectElement(childTag)
		if child == nil {
			return false
		}
		return child.SelectAttrValue(attrName, "") == rhs
	}
	return false
}

// applyPredicateConstraint writes the attribute(s) a freshly-created element
// needs so it actually satisfies the condition that selected it — otherwise
// a second WriteAtXPath call for a sibling field would fail to find it again
// and create a duplicate.
func applyPredicateConstraint(el *etree.Element, lhs, rhs string) {
	if strings.HasPrefix(lhs, "@") {
		el.CreateAttr(lhs[1:], rhs)
		return
	}
	if idx := strings.LastIndex(lhs, "/@"); idx >= 0 {
		childTag := lhs[:idx]
		attrName := lhs[idx+2:]
		var child *etree.Element
		for _, existing := range el.SelectElements(childTag) {
			if existing.SelectAttrValue(attrName, "") == rhs {
				child = existing
				break
			}
		}
		if child == nil {
			child = el.CreateElement(childTag)
			child.CreateAttr(attrName, rhs)
		}
	}
}

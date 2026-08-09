// services/executors/cda_element_translation.go
//
// CDA Coverage Audit, element granularity (see cda_coverage_tracker.go's own
// doc comment for why entry-level was the original, coarser design). The
// mapping engine (services/cda_fhir/declarative_engine.go) resolves fields
// against the TYPED "document.*" tree using Go-struct JSON tag names
// (row.Scope/row.SourcePath, e.g. "entryRelationships[typeCode=SUBJ].entry.
// value.code.code"). CDA Coverage Audit's inventory is deliberately built
// from the RAW, schema-agnostic "xml.*" mirror instead (GenericXMLToJSON,
// cda/document/generic_xml.go) so it isn't blind to content the typed parser
// doesn't model at all. Those two trees' field names don't line up 1:1 (see
// cda_element_translation_table.go's header comment for the three kinds of
// mismatch) -- this file resolves a typed path AND builds its raw-XML-mirror
// equivalent in a single walk, since the hardest mismatch (an
// <entryRelationship>/<component>'s act-tag child -- observation, act,
// substanceAdministration, etc.) can only be resolved by reading the
// already-in-hand resolved node's own EntryType, not derived from the path
// string alone.
//
// Deliberately NOT a modification of cda_path_resolver.go: ResolveCDAPaths/
// ResolveCDAPath are used everywhere in this codebase and stay completely
// untouched (zero regression risk to any existing caller). This file reuses
// their unexported grammar (parseCDAPath, cdaPathSegment, segment kind
// constants, asCDANodeList, matchesCDAPredicates) directly -- same package,
// so no duplicated wildcard/predicate/index parsing logic to drift out of
// sync.
package executors

import (
	"strconv"
	"strings"
)

// CDAResolvedNode is one node a translated path walk matched, carrying both
// its concrete typed-tree breadcrumb (indices resolved, no "[*]"/predicate
// left) and -- when a raw-XML equivalent could be determined -- the
// corresponding path into the "xml.*" GenericXMLToJSON mirror for the SAME
// underlying source element.
type CDAResolvedNode struct {
	// Value is the resolved typed-tree node itself (whatever ResolveCDAPath
	// would have returned for this same path).
	Value interface{}

	// Breadcrumb is the concrete typed-tree path to Value, e.g.
	// "entryRelationships[1].entry.value.code.code" -- every wildcard/
	// predicate segment resolved to the real index it matched. Diagnostic
	// only; not used for coverage recording.
	Breadcrumb string

	// XMLPath is Breadcrumb's equivalent in the raw "xml.*" mirror's own
	// addressing (e.g. "entryRelationship[1].observation.value" -- note
	// "value.code.code" collapsed away entirely: CD-typed <value> carries
	// code/displayName/codeSystem as attributes on itself, not a nested
	// <code> child -- see cda_element_translation_table.go's xlInlineStruct
	// kind). Only meaningful when Translatable is true. Relative to whatever
	// XML entry-root path the caller's rootKind corresponds to (e.g.
	// "sectionsByKey.<sectionKey>.entries[N]"'s raw-mirror equivalent
	// "entry[N]" -- callers are responsible for combining XMLPath with a
	// CDAEntryKey-style prefix; this file only ever produces the
	// entry-relative suffix).
	XMLPath string

	// Kind is the struct-kind context AFTER this path (e.g. "CDAParticipant"
	// for a "participants[*]" match) — the rootKind a caller should pass to a
	// FURTHER ResolveCDAPathsTranslated call resolving a nested path relative
	// to Value (e.g. a recursive engine call applying child MappingRow.Fields
	// against this same scoped node). Meaningless when Translatable is false.
	Kind string

	// Translatable is false when the path crossed a field this codebase's
	// translation table has no safe rule for (xlUnmapped, or an unknown
	// field name entirely). Recording code must skip -- never guess. Under-
	// recording only means a real gap goes unreported this cycle (safe);
	// recording a wrong/guessed path would corrupt the coverage report with
	// keys the inventory can never match, silently hiding real gaps instead.
	Translatable bool
}

// cdaTranslateWalkState is one in-flight node during a translated path walk
// -- the typed value plus everything needed to keep extending both the
// breadcrumb and (while still translatable) the XML path.
type cdaTranslateWalkState struct {
	typedNode    interface{}
	breadcrumb   string
	xmlPath      string
	kind         string // current struct-kind context for table lookups, e.g. "CDAEntry", "CDACode"
	translatable bool
}

// ResolveCDAPathsTranslated walks path (the same dot/bracket/wildcard/
// predicate grammar cda_path_resolver.go's ResolveCDAPaths uses) against
// root, a typed-tree node, resolving each segment exactly like
// ResolveCDAPaths(root, path, false) would -- but additionally carrying a
// translated XML-mirror-equivalent path alongside every match.
//
// rootKind is the struct-kind context root already represents -- almost
// always "CDAEntry" for every real caller (row.Scope/row.SourcePath are
// always resolved starting from an entry's own map), but kept as a parameter
// rather than hardcoded so a future caller resolving from a different
// context (e.g. directly from a CDAParticipant) isn't forced through a fake
// entry wrapper.
func ResolveCDAPathsTranslated(root interface{}, path string, rootKind string) []CDAResolvedNode {
	segments := parseCDAPath(path)
	states := []cdaTranslateWalkState{{typedNode: root, kind: rootKind, translatable: true}}

	for _, seg := range segments {
		var next []cdaTranslateWalkState
		for _, st := range states {
			nodeMap, ok := st.typedNode.(map[string]interface{})
			if !ok {
				continue
			}
			child, exists := nodeMap[seg.key]
			if !exists {
				continue
			}
			rule, hasRule := lookupCDAFieldRule(st.kind, seg.key)

			switch seg.kind {
			case segmentWildcard:
				for i, c := range asCDANodeList(child) {
					next = append(next, extendCDATranslateWalk(st, seg.key, i, c, rule, hasRule))
				}
			case segmentIndex:
				list := asCDANodeList(child)
				if seg.index >= 0 && seg.index < len(list) {
					next = append(next, extendCDATranslateWalk(st, seg.key, seg.index, list[seg.index], rule, hasRule))
				}
			case segmentPredicate:
				for i, c := range asCDANodeList(child) {
					if cm, ok := c.(map[string]interface{}); ok && matchesCDAPredicates(cm, seg.predicates, false) {
						next = append(next, extendCDATranslateWalk(st, seg.key, i, c, rule, hasRule))
					}
				}
			default: // segmentPlain
				// No concrete index from the path itself. Index 0 when child
				// is a single occurrence (asCDANodeList's own bare-object
				// convention -- NOT array-typed), matching what a raw-XML
				// inventory walker naturally produces for that same
				// single-occurrence tag with zero typed-schema knowledge (see
				// services/cda_coverage/inventory.go's walkEntryElements,
				// which can only ever see "how many raw occurrences," never
				// "is this Go struct field a slice") -- both sides must agree
				// without either needing information only the OTHER one has.
				// -1 (no index emitted) only for the genuinely ambiguous case
				// of a bare, bracket-less segment landing on 2+ real
				// occurrences -- no real declarative_oob_rules.go row does
				// this (every genuinely-repeating field access is always
				// "[0]"/"[*]"/"[predicate]" bracketed), so this is a narrow,
				// documented fallback, not a case either side needs to
				// resolve precisely. cdaXMLSegment's rule.firstOnly overrides
				// to 0 even in the 2+ case for the specific, table-documented
				// fields that are a singular alias for the first of a
				// genuinely repeatable raw tag (CDAEntry.EffectiveTime/.Value).
				idx := 0
				if _, isList := child.([]interface{}); isList {
					idx = -1
				}
				next = append(next, extendCDATranslateWalk(st, seg.key, idx, child, rule, hasRule))
			}
		}
		states = next
		if len(states) == 0 {
			return nil
		}
	}

	out := make([]CDAResolvedNode, 0, len(states))
	for _, st := range states {
		out = append(out, CDAResolvedNode{
			Value: st.typedNode,
			// Every segment is appended as ".key[idx]" unconditionally
			// (including the first), so both accumulate one leading "." --
			// stripped here rather than special-casing the first segment
			// throughout the walk.
			Breadcrumb:   strings.TrimPrefix(st.breadcrumb, "."),
			XMLPath:      strings.TrimPrefix(st.xmlPath, "."),
			Kind:         st.kind,
			Translatable: st.translatable,
		})
	}
	return out
}

// ResolveCDAPathTranslated returns the first match ResolveCDAPathsTranslated
// finds, or a zero-value (Translatable=false) result. Mirrors
// ResolveCDAPath's single-value convenience wrapper.
func ResolveCDAPathTranslated(root interface{}, path string, rootKind string) CDAResolvedNode {
	nodes := ResolveCDAPathsTranslated(root, path, rootKind)
	if len(nodes) == 0 {
		return CDAResolvedNode{Translatable: false}
	}
	return nodes[0]
}

// JoinCDAXMLPath combines an accumulated base XML path (possibly "" at the
// entry root) with a further relative XML path this translator produced,
// for callers threading CDA Coverage Audit recording through nested engine
// recursion (see services/cda_fhir/declarative_engine.go's applyRow, which
// needs a node's own XMLPath as the basePath for a FURTHER
// ResolveCDAPathsTranslated call resolving a child MappingRow.Fields row
// against that same scoped node).
func JoinCDAXMLPath(base, next string) string {
	switch {
	case base == "":
		return next
	case next == "":
		return base
	default:
		return base + "." + next
	}
}

// extendCDATranslateWalk advances st by one resolved segment (seg.key,
// concrete index idx -- or -1 when none applies, see the segmentPlain case
// above) whose value is child, applying rule (from the translation table)
// to extend the XML side. hasRule=false or rule.kind==xlUnmapped both result
// in translatable=false from this point on -- once untranslatable, a walk
// never recovers (there is no safe way to "resync" the XML side after a gap).
func extendCDATranslateWalk(st cdaTranslateWalkState, key string, idx int, child interface{}, rule cdaFieldRule, hasRule bool) cdaTranslateWalkState {
	next := cdaTranslateWalkState{
		typedNode:    child,
		breadcrumb:   st.breadcrumb + "." + breadcrumbSegment(key, idx),
		xmlPath:      st.xmlPath,
		translatable: st.translatable,
	}

	if !st.translatable || !hasRule || rule.kind == xlUnmapped {
		next.translatable = false
		return next
	}

	switch rule.kind {
	case xlAttr:
		// Terminal scalar (attribute or the element's own text -- coverage
		// tracks at element granularity, not attribute, so which of the two
		// doesn't matter here): xmlPath does not grow, no further struct
		// context to descend into.
		next.kind = ""
	case xlInlineStruct:
		// Sub-struct whose own fields live on the SAME raw XML node (e.g.
		// CDAValue.Code's fields are attributes directly on <value>, not a
		// nested <code> child) -- xmlPath does not grow, but recursion
		// continues in the sub-struct's own field-rule context.
		next.kind = rule.nextKind
	case xlSame, xlRenamed:
		next.xmlPath = st.xmlPath + "." + cdaXMLSegment(rule, key, idx)
		next.kind = rule.nextKind
	case xlActChild:
		// No wrapper element consumed here (e.g. CDAEntryRelationship.Entry
		// -- <entryRelationship> directly contains its act child, the
		// "entryRelationship[N]" segment was already appended by the PARENT
		// field's own xlRenamed rule). Only the data-dependent act tag itself
		// is appended -- always index 0: raw XML syntax only ever allows ONE
		// recognised act-type child per entryRelationship, so this is never
		// ambiguous, but the explicit index is still required (not a bare
		// "observation") to byte-match what a raw-XML-mirror walker
		// enumerates for that same single-occurrence key with no semantic
		// knowledge that it can only ever appear once (see
		// services/cda_coverage/inventory.go's walkEntryElements, which
		// indexes every key uniformly).
		entryType, ok := cdaActTypeOf(child)
		if !ok {
			next.translatable = false
			return next
		}
		next.xmlPath = st.xmlPath + "." + entryType + "[0]"
		next.kind = "CDAEntry"
	case xlRenamedActChild:
		// A REAL wrapper element is consumed first (e.g. CDAEntry.Components
		// -- <organizer><component><observation>...</component></organizer>,
		// unlike xlActChild's wrapper-less case), THEN the data-dependent act
		// tag nested inside it (same always-index-0 reasoning as xlActChild
		// above).
		entryType, ok := cdaActTypeOf(child)
		if !ok {
			next.translatable = false
			return next
		}
		next.xmlPath = st.xmlPath + "." + cdaXMLSegment(rule, key, idx) + "." + entryType + "[0]"
		next.kind = "CDAEntry"
	default:
		next.translatable = false
	}
	return next
}

// cdaXMLSegment formats the raw-XML segment(s) an xlSame/xlRenamed/
// xlRenamedActChild rule contributes. Most rules are a single renamed
// segment with the same concrete index the typed side resolved (or index 0
// when rule.firstOnly marks a typed field as a singular alias for the first
// occurrence of a repeatable raw tag, e.g. CDAEntry.EffectiveTime aliasing
// EffectiveTimes[0]). rule.multiSegment is for the case where several typed
// struct levels collapse into one field (e.g. Precondition's
// "precondition.criterion.value", or CDAHeader-rooted fields like
// CDAPatient.Names' "patient.name" -- both entry-level and header-level
// collapsed paths use this same rule kind). Every EARLIER level in the
// dotted xmlKey is forced to index 0 -- every documented real occurrence of
// an intermediate collapsed level is a singular, unrepeated wrapper (CDA
// XML's own <precondition>/<criterion>/<patient> etc. never repeat at that
// position; see cda/document/header_parser.go's nav()/SelectElement calls,
// always singular, for the header cases). Only the LAST dotted part gets the
// walk's own real idx -- needed because that FINAL segment can genuinely be
// a repeating raw tag reached through a collapsed prefix (e.g.
// CDAPatient.Names -> "patient.name", where <patient> is singular but
// <name> repeats): forcing [0] there too (the original, simpler rule) would
// silently collapse every names[1+]/languages[1+] onto the SAME key as
// names[0] -- a wrong path, worse than a missing one, per this file's own
// "never guess" principle. Provably a no-op for every multiSegment rule that
// predates this file's header-field work (Precondition/ReferenceRangeText/
// RelatedSubjectCode/Product): all four are singular Go fields, so the
// walk's own idx passed in is always 0 regardless of which segment it's
// applied to.
func cdaXMLSegment(rule cdaFieldRule, key string, idx int) string {
	xmlKey := rule.xmlKey
	if xmlKey == "" {
		xmlKey = key
	}
	if rule.firstOnly {
		idx = 0
	}
	if rule.multiSegment {
		parts := strings.Split(xmlKey, ".")
		last := len(parts) - 1
		for i, p := range parts {
			if i == last {
				parts[i] = breadcrumbSegment(p, idx)
			} else {
				parts[i] = p + "[0]"
			}
		}
		return strings.Join(parts, ".")
	}
	return breadcrumbSegment(xmlKey, idx)
}

// cdaActTypeOf reads node's own "entryType" field (the typed tree's record
// of which of entryActTypes -- cda/document/entry_parser.go's "observation"|
// "act"|"organizer"|"substanceAdministration"|"procedure"|"encounter"|
// "supply" -- this act actually was) and confirms it's one of the known,
// real act tags. This is the one piece of the whole translation that cannot
// be a static table lookup: which literal XML tag a <entryRelationship>/
// <component> wraps is a property of the DATA, not the path string.
func cdaActTypeOf(node interface{}) (entryType string, ok bool) {
	m, isMap := node.(map[string]interface{})
	if !isMap {
		return "", false
	}
	entryType, _ = m["entryType"].(string)
	return entryType, isKnownCDAActType(entryType)
}

// CDAEntryActTagPrefix returns the raw-XML act-tag wrapper segment (e.g.
// "encounter[0]", "observation[0]") that a SECTION ENTRY's own <entry>
// element wraps -- the exact same data-dependent relationship xlActChild
// resolves for a NESTED entryRelationship/component's act child (see that
// case above), but at the section-entry root instead of a nested node: raw
// XML's <entry> directly contains exactly one of the 7 act tags
// (cdaKnownActTypes), which the typed parser (cda/document/entry_parser.go)
// flattens away into CDAEntry's own top-level fields (Code, StatusCode,
// EffectiveTime, Value, ...). Every element-level coverage recording for a
// section entry's OWN fields must therefore start from THIS prefix as its
// basePath, not from the entry root directly -- otherwise the recorded key
// (e.g. "code[0]") never matches services/cda_coverage/inventory.go's
// walkEntryElements, which walks the UNFLATTENED raw XML and sees the
// wrapper as a real, present level (e.g. "encounter[0].code[0]").
//
// ok=false (missing/unrecognized entryType) means "don't guess" -- same
// fail-closed posture as xlActChild itself: the caller should record
// nothing for this entry's top-level fields rather than a wrong-shaped
// path that could never match anything in the inventory.
func CDAEntryActTagPrefix(entryMap map[string]interface{}) (prefix string, ok bool) {
	entryType, ok := cdaActTypeOf(entryMap)
	if !ok {
		return "", false
	}
	return entryType + "[0]", true
}

// breadcrumbSegment formats one resolved path segment as "key" (idx < 0,
// meaning no single concrete index applies -- a genuinely ambiguous plain
// segment over a multi-element array) or "key[idx]".
func breadcrumbSegment(key string, idx int) string {
	if idx < 0 {
		return key
	}
	return key + "[" + strconv.Itoa(idx) + "]"
}

package builder

import (
	"fmt"
	"testing"

	"github.com/beevik/etree"
)

func newRoot() *etree.Element {
	return etree.NewElement("entry")
}

func TestWriteAtXPath_PlainElementText(t *testing.T) {
	root := newRoot()
	WriteAtXPath(root, "act/code", "12345")

	el := root.FindElement("act/code")
	if el == nil {
		t.Fatal("expected act/code element to exist")
	}
	if el.Text() != "12345" {
		t.Errorf("got text %q, want %q", el.Text(), "12345")
	}
}

func TestWriteAtXPath_LeafAttribute(t *testing.T) {
	root := newRoot()
	WriteAtXPath(root, "act/code/@code", "48765-2")

	el := root.FindElement("act/code")
	if el == nil {
		t.Fatal("expected act/code element to exist")
	}
	if got := el.SelectAttrValue("code", ""); got != "48765-2" {
		t.Errorf("got attr %q, want %q", got, "48765-2")
	}
}

func TestWriteAtXPath_SelfAttributePredicate_FindOrCreate(t *testing.T) {
	root := newRoot()
	WriteAtXPath(root, "act/entryRelationship[@typeCode='SUBJ']/observation/value/@code", "73211009")
	WriteAtXPath(root, "act/entryRelationship[@typeCode='SUBJ']/observation/statusCode/@code", "completed")

	act := root.FindElement("act")
	if act == nil {
		t.Fatal("expected act element")
	}
	rels := act.SelectElements("entryRelationship")
	if len(rels) != 1 {
		t.Fatalf("expected exactly 1 entryRelationship (both writes should share it), got %d", len(rels))
	}
	if got := rels[0].SelectAttrValue("typeCode", ""); got != "SUBJ" {
		t.Errorf("entryRelationship typeCode = %q, want SUBJ", got)
	}

	obs := rels[0].SelectElement("observation")
	if obs == nil {
		t.Fatal("expected nested observation")
	}
	if got := obs.FindElement("value").SelectAttrValue("code", ""); got != "73211009" {
		t.Errorf("value/@code = %q, want 73211009", got)
	}
	if got := obs.FindElement("statusCode").SelectAttrValue("code", ""); got != "completed" {
		t.Errorf("statusCode/@code = %q, want completed", got)
	}
}

func TestWriteAtXPath_DistinctPredicateValues_CreateSeparateSiblings(t *testing.T) {
	root := newRoot()
	WriteAtXPath(root, "act/entryRelationship[@typeCode='SUBJ']/observation/value/@code", "111")
	WriteAtXPath(root, "act/entryRelationship[@typeCode='MFST']/observation/value/@code", "222")

	act := root.FindElement("act")
	rels := act.SelectElements("entryRelationship")
	if len(rels) != 2 {
		t.Fatalf("expected 2 distinct entryRelationship siblings (SUBJ, MFST), got %d", len(rels))
	}
}

func TestWriteAtXPath_ChildAttributePredicate(t *testing.T) {
	root := newRoot()
	// Mirrors socialHistory's smokingStatus xpath shape:
	// "observation[code/@code='ASSERTION']/value/@code"
	WriteAtXPath(root, "observation[code/@code='ASSERTION']/value/@code", "449868002")

	obs := root.SelectElement("observation")
	if obs == nil {
		t.Fatal("expected observation element")
	}
	code := obs.SelectElement("code")
	if code == nil || code.SelectAttrValue("code", "") != "ASSERTION" {
		t.Fatal("expected observation/code[@code='ASSERTION'] to have been created")
	}
	if got := obs.FindElement("value").SelectAttrValue("code", ""); got != "449868002" {
		t.Errorf("value/@code = %q, want 449868002", got)
	}
}

// TestWriteAtXPath_MultiConditionSameNestedChild_OneElementBothAttrs is the
// Tobacco Use regression case: a predicate with TWO comma-AND conditions
// that both target the SAME nested child ("code/@code" and
// "code/@codeSystem") must produce ONE <code> element carrying both
// attributes — a real schema validator found this producing two separate
// sibling <code> elements instead, since each condition was independently
// doing its own find-or-create pass against a child that didn't have the
// OTHER attribute yet.
func TestWriteAtXPath_MultiConditionSameNestedChild_OneElementBothAttrs(t *testing.T) {
	root := newRoot()
	path := "observation[code/@code='11367-0',code/@codeSystem='2.16.840.1.113883.6.1']/value/@code"
	WriteAtXPath(root, path, "8517006")

	obs := root.SelectElement("observation")
	if obs == nil {
		t.Fatal("expected observation element")
	}
	codes := obs.SelectElements("code")
	if len(codes) != 1 {
		t.Fatalf("expected exactly one <code> element, got %d", len(codes))
	}
	if got := codes[0].SelectAttrValue("code", ""); got != "11367-0" {
		t.Errorf("code/@code = %q, want 11367-0", got)
	}
	if got := codes[0].SelectAttrValue("codeSystem", ""); got != "2.16.840.1.113883.6.1" {
		t.Errorf("code/@codeSystem = %q, want 2.16.840.1.113883.6.1", got)
	}
}

// TestWriteAtXPath_MultiConditionSameNestedChild_SecondFieldReusesElement
// verifies a SECOND field targeting the same multi-condition predicated
// path (e.g. socialHistory's smokingStatusEffectiveTime, alongside
// smokingStatus) correctly finds and reuses the SAME observation/code the
// first field's write already created, rather than creating a duplicate.
func TestWriteAtXPath_MultiConditionSameNestedChild_SecondFieldReusesElement(t *testing.T) {
	root := newRoot()
	path := "observation[code/@code='11367-0',code/@codeSystem='2.16.840.1.113883.6.1']"
	WriteAtXPath(root, path+"/value/@code", "8517006")
	WriteAtXPath(root, path+"/effectiveTime/low/@value", "20200315")

	observations := root.SelectElements("observation")
	if len(observations) != 1 {
		t.Fatalf("expected exactly one <observation> (second field reuses it), got %d", len(observations))
	}
	obs := observations[0]
	if obs.FindElement("value") == nil || obs.FindElement("effectiveTime/low") == nil {
		t.Errorf("expected both fields' writes on the SAME observation, got: %+v", obs)
	}
}

func TestWriteAtXPath_EmptyValue_CreatesAnchorOnly(t *testing.T) {
	root := newRoot()
	el := WriteAtXPath(root, "act", "")
	if el == nil || el.Tag != "act" {
		t.Fatal("expected act element to be created and returned")
	}
	if el.Text() != "" {
		t.Errorf("expected no text set, got %q", el.Text())
	}
}

// TestWriteAtXPath_SamePredicateSiblings_DisambiguatedByNestedTemplateId
// mirrors Allergy's real structure: Severity and Status Observations both
// nest under entryRelationship[@typeCode='SUBJ',@inversionInd='true'] — the
// SAME predicate — distinguished only by their own nested templateId. Three
// fields writing through this shape must produce THREE separate
// entryRelationship siblings, not merge into one or overwrite each other.
func TestWriteAtXPath_SamePredicateSiblings_DisambiguatedByNestedTemplateId(t *testing.T) {
	root := newRoot()
	base := "observation/entryRelationship[@typeCode='SUBJ',@inversionInd='true']/observation[templateId/@root='%s']/value/@code"

	WriteAtXPath(root, fmt.Sprintf(base,"TID-SEVERITY"), "moderate")
	WriteAtXPath(root, fmt.Sprintf(base,"TID-STATUS"), "active")
	WriteAtXPath(root, fmt.Sprintf(base,"TID-CRITICALITY"), "high")

	obs := root.FindElement("observation")
	rels := obs.SelectElements("entryRelationship")
	if len(rels) != 3 {
		t.Fatalf("expected 3 distinct entryRelationship siblings, got %d", len(rels))
	}

	seen := map[string]string{} // nested templateId root -> value/@code
	for _, rel := range rels {
		if rel.SelectAttrValue("typeCode", "") != "SUBJ" || rel.SelectAttrValue("inversionInd", "") != "true" {
			t.Errorf("entryRelationship missing expected typeCode=SUBJ/inversionInd=true attrs: %v", rel.Attr)
		}
		nested := rel.SelectElement("observation")
		root := nested.SelectElement("templateId").SelectAttrValue("root", "")
		seen[root] = nested.SelectElement("value").SelectAttrValue("code", "")
	}

	want := map[string]string{"TID-SEVERITY": "moderate", "TID-STATUS": "active", "TID-CRITICALITY": "high"}
	for k, v := range want {
		if seen[k] != v {
			t.Errorf("nested observation with templateId root=%q: got value %q, want %q", k, seen[k], v)
		}
	}
}

// TestWriteAtXPath_NewMultiSegmentTerminal_ReusesExistingDisambiguatedSibling
// is the regression test for a real bug found via a live Test Pipeline run
// (2026-07): a field targeting a brand-new, not-yet-written multi-segment
// terminal (effectiveTime/low/@value) under an entryRelationship/observation
// pair an EARLIER field already created (value/@code, matching the SAME
// nested templateId predicate) must REUSE that existing pair, not spawn a
// duplicate sibling. The old lookahead required the ENTIRE remaining path
// (including the not-yet-existing terminal) to already exist before
// reusing a candidate — which can never be true the first time a new
// terminal is written — so it always concluded "no match" and duplicated.
func TestWriteAtXPath_NewMultiSegmentTerminal_ReusesExistingDisambiguatedSibling(t *testing.T) {
	root := newRoot()
	base := "act/entryRelationship[@typeCode='MFST',@inversionInd='true']/observation[templateId/@root='%s']"

	WriteAtXPath(root, fmt.Sprintf(base, "TID-REACTION")+"/value/@code", "247472004")
	// A later field, same predicate-disambiguated element, brand-new
	// multi-segment terminal that doesn't exist yet.
	WriteAtXPath(root, fmt.Sprintf(base, "TID-REACTION")+"/effectiveTime/low/@value", "20100604")

	act := root.FindElement("act")
	rels := act.SelectElements("entryRelationship")
	if len(rels) != 1 {
		t.Fatalf("expected exactly ONE entryRelationship (second field reuses it), got %d", len(rels))
	}
	obs := rels[0].SelectElement("observation")
	if obs == nil {
		t.Fatal("expected nested observation")
	}
	if obs.SelectElement("value").SelectAttrValue("code", "") != "247472004" {
		t.Errorf("expected value/@code=247472004 on the shared observation, got: %+v", obs)
	}
	if got := obs.FindElement("effectiveTime/low").SelectAttrValue("value", ""); got != "20100604" {
		t.Errorf("expected effectiveTime/low/@value=20100604 on the SAME shared observation, got %q", got)
	}
}

// TestWriteAtXPath_NewBareTagTerminal_ReusesExistingDisambiguatedSibling is
// the same regression as above but for a single-segment bare-tag terminal
// (e.g. Problem Status's "text") rather than a multi-segment one
// (effectiveTime/low) — both were affected by the same root cause.
func TestWriteAtXPath_NewBareTagTerminal_ReusesExistingDisambiguatedSibling(t *testing.T) {
	root := newRoot()
	base := "act/entryRelationship[@typeCode='REFR']/observation[templateId/@root='%s']"

	WriteAtXPath(root, fmt.Sprintf(base, "TID-STATUS")+"/value/@code", "55561003")
	WriteAtXPath(root, fmt.Sprintf(base, "TID-STATUS")+"/text", "Confirmed diagnosis.")

	act := root.FindElement("act")
	rels := act.SelectElements("entryRelationship")
	if len(rels) != 1 {
		t.Fatalf("expected exactly ONE entryRelationship (second field reuses it), got %d", len(rels))
	}
	obs := rels[0].SelectElement("observation")
	if obs.SelectElement("value").SelectAttrValue("code", "") != "55561003" {
		t.Errorf("expected value/@code=55561003, got: %+v", obs)
	}
	if obs.SelectElement("text").Text() != "Confirmed diagnosis." {
		t.Errorf("expected text on the SAME shared observation, got: %+v", obs)
	}
}

// TestWriteAtXPath_NewNestedPredicateTerminal_ReusesExistingSingularAncestor
// is the regression test for a real bug found via a live Test Pipeline run
// (2026-07): a field targeting a brand-new terminal that is ITSELF predicated
// (playingEntity[@classCode='PLC'], not just a bare tag) two levels below an
// already-uniquely-identified singular ancestor (participant[@typeCode=
// 'LOC']/participantRole[@classCode='SDLOC'] — there is only ever one such
// pair per encounter, no disambiguation needed) must reuse that ancestor, not
// spawn a duplicate. The old lookaheadPrefix (stopping at the LAST predicated
// segment in remaining) required playingEntity to already exist before
// reusing participant/participantRole — which can never be true the first
// time that field runs — so it always concluded "no match" and duplicated
// the entire participant/participantRole chain. Fixed by stopping at the
// FIRST predicated segment instead (the shallowest, and only, real
// discriminator).
func TestWriteAtXPath_NewNestedPredicateTerminal_ReusesExistingSingularAncestor(t *testing.T) {
	root := newRoot()
	base := "encounter/participant[@typeCode='LOC']/participantRole[@classCode='SDLOC']"

	WriteAtXPath(root, base+"/code/@code", "1160-1")
	WriteAtXPath(root, base+"/addr/city", "Springfield")
	// A later field, same already-unique participant/participantRole, a
	// brand-new terminal that is ITSELF predicated (not a bare tag).
	WriteAtXPath(root, base+"/playingEntity[@classCode='PLC']/name", "Springfield Medical Center Clinic")

	enc := root.FindElement("encounter")
	participants := enc.SelectElements("participant")
	if len(participants) != 1 {
		t.Fatalf("expected exactly ONE participant (third field reuses it), got %d", len(participants))
	}
	role := participants[0].SelectElement("participantRole")
	if role == nil {
		t.Fatal("expected participantRole")
	}
	if got := role.SelectElement("code").SelectAttrValue("code", ""); got != "1160-1" {
		t.Errorf("expected code/@code=1160-1 on the shared participantRole, got %q", got)
	}
	if got := role.FindElement("addr/city").Text(); got != "Springfield" {
		t.Errorf("expected addr/city=Springfield on the SAME shared participantRole, got %q", got)
	}
	pe := role.SelectElement("playingEntity")
	if pe == nil {
		t.Fatal("expected playingEntity on the SAME shared participantRole")
	}
	if pe.SelectAttrValue("classCode", "") != "PLC" {
		t.Errorf("expected playingEntity/@classCode=PLC, got: %+v", pe.Attr)
	}
	if got := pe.SelectElement("name").Text(); got != "Springfield Medical Center Clinic" {
		t.Errorf("expected playingEntity/name, got %q", got)
	}
}

// TestTryFindAtXPath_BacktracksPastWrongSibling verifies the read-only
// lookup used by StructuralTemplateAnchor processing doesn't dead-end on
// the first same-predicate sibling — it must keep searching until it finds
// the one whose nested templateId actually matches.
func TestTryFindAtXPath_BacktracksPastWrongSibling(t *testing.T) {
	root := newRoot()
	base := "observation/entryRelationship[@typeCode='SUBJ',@inversionInd='true']/observation[templateId/@root='%s']/value/@code"
	WriteAtXPath(root, fmt.Sprintf(base,"TID-A"), "first")
	WriteAtXPath(root, fmt.Sprintf(base,"TID-B"), "second")

	found, ok := TryFindAtXPath(root, "observation/entryRelationship[@typeCode='SUBJ',@inversionInd='true']/observation[templateId/@root='TID-B']")
	if !ok {
		t.Fatal("expected to find the second sibling's nested observation, got not-found")
	}
	if found.SelectElement("value").SelectAttrValue("code", "") != "second" {
		t.Errorf("found wrong node: value/@code = %q, want \"second\"", found.SelectElement("value").SelectAttrValue("code", ""))
	}

	if _, ok := TryFindAtXPath(root, "observation/entryRelationship[@typeCode='SUBJ',@inversionInd='true']/observation[templateId/@root='TID-NONEXISTENT']"); ok {
		t.Error("expected not-found for a templateId root that was never written")
	}
}

func TestReorderChildrenByTag_MovesListedTagsFirstPreservingRelativeOrder(t *testing.T) {
	el := etree.NewElement("manufacturedProduct")
	el.CreateElement("manufacturedMaterial").CreateAttr("marker", "material")
	el.CreateElement("id").CreateAttr("marker", "id1")
	el.CreateElement("templateId").CreateAttr("marker", "tid")
	el.CreateElement("id").CreateAttr("marker", "id2")

	reorderChildrenByTag(el, []string{"templateId", "id"})

	got := el.ChildElements()
	wantTags := []string{"templateId", "id", "id", "manufacturedMaterial"}
	if len(got) != len(wantTags) {
		t.Fatalf("expected %d children, got %d", len(wantTags), len(got))
	}
	for i, want := range wantTags {
		if got[i].Tag != want {
			t.Errorf("child %d: expected tag %q, got %q", i, want, got[i].Tag)
		}
	}
	// The two <id> elements must keep THEIR OWN original relative order
	// (id1 before id2) — a stable sort, not just "any id first".
	if got[1].SelectAttrValue("marker", "") != "id1" || got[2].SelectAttrValue("marker", "") != "id2" {
		t.Errorf("expected id1 before id2 (stable sort), got markers %q, %q", got[1].SelectAttrValue("marker", ""), got[2].SelectAttrValue("marker", ""))
	}
}

func TestReorderChildrenByTag_NoOpWhenAlreadyInOrder(t *testing.T) {
	el := etree.NewElement("act")
	el.CreateElement("templateId")
	el.CreateElement("code")
	el.CreateElement("statusCode")

	reorderChildrenByTag(el, []string{"templateId"})

	got := el.ChildElements()
	wantTags := []string{"templateId", "code", "statusCode"}
	for i, want := range wantTags {
		if got[i].Tag != want {
			t.Errorf("child %d: expected tag %q, got %q", i, want, got[i].Tag)
		}
	}
}

func TestSplitPathSegments_RespectsBracketsContainingSlash(t *testing.T) {
	segs := splitPathSegments("observation[code/@code='ASSERTION']/value/@code")
	want := []string{"observation[code/@code='ASSERTION']", "value", "@code"}
	if len(segs) != len(want) {
		t.Fatalf("got %d segments %v, want %d", len(segs), segs, len(want))
	}
	for i := range want {
		if segs[i] != want[i] {
			t.Errorf("segment %d = %q, want %q", i, segs[i], want[i])
		}
	}
}

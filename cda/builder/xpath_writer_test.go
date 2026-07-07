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

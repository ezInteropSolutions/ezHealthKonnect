package services

import (
	"reflect"
	"testing"
)

func TestSetFieldWithArrayIndicesSplicesLists(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	cp := func(v string) map[string]interface{} { return map[string]interface{}{"system": "phone", "value": v} }

	// A list written to an empty slot becomes successive elements.
	r := map[string]interface{}{}
	if err := s.setFieldWithArrayIndices(r, "telecom[0]", []map[string]interface{}{cp("1"), cp("2")}, nil); err != nil {
		t.Fatal(err)
	}
	want := []interface{}{cp("1"), cp("2")}
	if !reflect.DeepEqual(r["telecom"], want) {
		t.Fatalf("got %v", r["telecom"])
	}

	// A list written to an occupied slot is appended, overwriting nothing.
	if err := s.setFieldWithArrayIndices(r, "telecom[1]", []interface{}{cp("3"), nil}, nil); err != nil {
		t.Fatal(err)
	}
	want = []interface{}{cp("1"), cp("2"), cp("3")}
	if !reflect.DeepEqual(r["telecom"], want) {
		t.Fatalf("got %v", r["telecom"])
	}

	// A list to a slot beyond the end: the slot (padded) takes the first item.
	r2 := map[string]interface{}{}
	if err := s.setFieldWithArrayIndices(r2, "telecom[1]", []interface{}{cp("a"), cp("b")}, nil); err != nil {
		t.Fatal(err)
	}
	if got := r2["telecom"].([]interface{}); len(got) != 3 || !reflect.DeepEqual(got[1], cp("a")) || !reflect.DeepEqual(got[2], cp("b")) {
		t.Fatalf("got %v", got)
	}

	// A single value behaves as before: it replaces the slot.
	if err := s.setFieldWithArrayIndices(r, "telecom[0]", cp("x"), nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r["telecom"].([]interface{})[0], cp("x")) {
		t.Fatalf("got %v", r["telecom"])
	}

	// Nested paths are untouched.
	r3 := map[string]interface{}{}
	if err := s.setFieldWithArrayIndices(r3, "name[0].given[1]", "Jo", nil); err != nil {
		t.Fatal(err)
	}
	if g := r3["name"].([]interface{})[0].(map[string]interface{})["given"].([]interface{}); g[1] != "Jo" {
		t.Fatalf("got %v", r3)
	}
}

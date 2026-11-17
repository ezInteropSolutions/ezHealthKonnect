package parsers

import (
	"reflect"
	"testing"
)

func TestJSONParser(t *testing.T) {
	p := NewJSONParser()
	cases := []struct {
		name, in string
		want     map[string]interface{}
	}{
		{"object", `{"type":"identity.merged","data":{"retired":"EID-B"},"n":3}`,
			map[string]interface{}{"type": "identity.merged", "data": map[string]interface{}{"retired": "EID-B"}, "n": float64(3)}},
		{"array", `[1,"a"]`, map[string]interface{}{"items": []interface{}{float64(1), "a"}}},
		{"scalar", `"x"`, map[string]interface{}{"value": "x"}},
		{"invalid", `{"a":`, map[string]interface{}{}},
		{"two values", `{"a":1} {"b":2}`, map[string]interface{}{}},
	}
	for _, c := range cases {
		r := p.Parse(c.in)
		if !r.Success {
			t.Fatalf("%s: not successful", c.name)
		}
		got := r.ParsedJSON
		if got["raw"] != c.in {
			t.Errorf("%s: raw = %v", c.name, got["raw"])
		}
		delete(got, "raw")
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %#v want %#v", c.name, got, c.want)
		}
	}
}

func TestJSONParserKeepsContentRawKey(t *testing.T) {
	in := `{"raw":"theirs"}`
	got := NewJSONParser().Parse(in).ParsedJSON
	if got["raw"] != "theirs" || got["_raw_content"] != in {
		t.Fatalf("%#v", got)
	}
}

func TestJSONParserSkipsReservedKeys(t *testing.T) {
	got := NewJSONParser().Parse(`{"_format":"x","enhancedSegments":{},"type":"t"}`).ParsedJSON
	if _, ok := got["_format"]; ok {
		t.Fatal("_format copied")
	}
	if _, ok := got["enhancedSegments"]; ok {
		t.Fatal("enhancedSegments copied")
	}
	if got["type"] != "t" || got["raw"] == nil {
		t.Fatalf("%#v", got)
	}
}

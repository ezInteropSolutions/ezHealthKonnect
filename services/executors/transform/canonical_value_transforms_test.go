// services/executors/transform/canonical_value_transforms_test.go
package transform

import "testing"

func TestApplyCanonicalTransform_Trim(t *testing.T) {
	if got := applyCanonicalTransform("trim", "  hello  "); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestApplyCanonicalTransform_Uppercase(t *testing.T) {
	if got := applyCanonicalTransform("uppercase", "abc"); got != "ABC" {
		t.Errorf("got %q, want %q", got, "ABC")
	}
}

func TestApplyCanonicalTransform_Lowercase(t *testing.T) {
	if got := applyCanonicalTransform("lowercase", "ABC"); got != "abc" {
		t.Errorf("got %q, want %q", got, "abc")
	}
}

func TestApplyCanonicalTransform_DateToCDA_ISO(t *testing.T) {
	if got := applyCanonicalTransform("date_to_cda", "2024-01-15"); got != "20240115" {
		t.Errorf("got %q, want %q", got, "20240115")
	}
}

func TestApplyCanonicalTransform_DateToCDA_MDYZeroPadded(t *testing.T) {
	if got := applyCanonicalTransform("date_to_cda", "01/15/2024"); got != "20240115" {
		t.Errorf("got %q, want %q", got, "20240115")
	}
}

func TestApplyCanonicalTransform_DateToCDA_MDYNoPadding(t *testing.T) {
	if got := applyCanonicalTransform("date_to_cda", "1/5/2024"); got != "20240105" {
		t.Errorf("got %q, want %q", got, "20240105")
	}
}

func TestApplyCanonicalTransform_DateToCDA_UnparseableInput_PassesThroughUnchanged(t *testing.T) {
	if got := applyCanonicalTransform("date_to_cda", "not-a-date"); got != "not-a-date" {
		t.Errorf("got %q, want unchanged passthrough %q", got, "not-a-date")
	}
}

func TestApplyCanonicalTransform_DateTimeToCDA_RFC3339_NormalizesToUTC(t *testing.T) {
	// 2024-01-15T10:00:00-05:00 == 2024-01-15T15:00:00Z
	if got := applyCanonicalTransform("datetime_to_cda", "2024-01-15T10:00:00-05:00"); got != "20240115150000" {
		t.Errorf("got %q, want %q", got, "20240115150000")
	}
}

func TestApplyCanonicalTransform_DateTimeToCDA_ZonelessTFormat(t *testing.T) {
	if got := applyCanonicalTransform("datetime_to_cda", "2024-01-15T10:00:00"); got != "20240115100000" {
		t.Errorf("got %q, want %q", got, "20240115100000")
	}
}

func TestApplyCanonicalTransform_DateTimeToCDA_SpaceDelimited(t *testing.T) {
	if got := applyCanonicalTransform("datetime_to_cda", "2024-01-15 10:00:00"); got != "20240115100000" {
		t.Errorf("got %q, want %q", got, "20240115100000")
	}
}

func TestApplyCanonicalTransform_EmptyName_Passthrough(t *testing.T) {
	if got := applyCanonicalTransform("", "unchanged"); got != "unchanged" {
		t.Errorf("got %q, want %q", got, "unchanged")
	}
}

func TestApplyCanonicalTransform_EmptyValue_Passthrough(t *testing.T) {
	if got := applyCanonicalTransform("uppercase", ""); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestApplyCanonicalTransform_UnknownName_PassesThroughUnchanged(t *testing.T) {
	if got := applyCanonicalTransform("not_a_real_transform", "value"); got != "value" {
		t.Errorf("got %q, want unchanged passthrough %q", got, "value")
	}
}

func TestCanonicalTransformDescriptions_CoversEveryRegisteredTransform(t *testing.T) {
	descriptions := CanonicalTransformDescriptions()
	if len(descriptions) != len(canonicalValueTransforms) {
		t.Fatalf("got %d descriptions, want %d (one per registered transform)", len(descriptions), len(canonicalValueTransforms))
	}
	for name := range canonicalValueTransforms {
		if descriptions[name] == "" {
			t.Errorf("transform %q has no description", name)
		}
	}
}

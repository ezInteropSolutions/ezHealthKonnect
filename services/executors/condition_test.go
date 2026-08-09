package executors

import "testing"

func TestEvaluateCondition_Equals(t *testing.T) {
	data := map[string]interface{}{"country": "US"}
	met, err := EvaluateCondition(map[string]interface{}{"field": "country", "operator": "equals", "value": "US"}, data)
	if err != nil {
		t.Fatalf("EvaluateCondition failed: %v", err)
	}
	if !met {
		t.Error("expected equals condition to be met")
	}

	met, err = EvaluateCondition(map[string]interface{}{"field": "country", "operator": "equals", "value": "CA"}, data)
	if err != nil {
		t.Fatalf("EvaluateCondition failed: %v", err)
	}
	if met {
		t.Error("expected equals condition to NOT be met")
	}
}

func TestEvaluateCondition_ExistsAndNotExists(t *testing.T) {
	data := map[string]interface{}{"ssn": "123-45-6789"}

	met, _ := EvaluateCondition(map[string]interface{}{"field": "ssn", "operator": "exists"}, data)
	if !met {
		t.Error("expected exists=true for a present field")
	}

	met, _ = EvaluateCondition(map[string]interface{}{"field": "missingField", "operator": "exists"}, data)
	if met {
		t.Error("expected exists=false for a missing field")
	}

	met, _ = EvaluateCondition(map[string]interface{}{"field": "missingField", "operator": "not_exists"}, data)
	if !met {
		t.Error("expected not_exists=true for a missing field")
	}
}

func TestEvaluateCondition_NumericComparisons(t *testing.T) {
	data := map[string]interface{}{"age": 42.0}

	cases := []struct {
		operator string
		value    interface{}
		want     bool
	}{
		{"greater_than", 40, true},
		{"greater_than", 50, false},
		{"greater_than_or_equal", 42, true},
		{"less_than", 50, true},
		{"less_than_or_equal", 42, true},
		{"less_than_or_equal", 41, false},
	}
	for _, c := range cases {
		met, err := EvaluateCondition(map[string]interface{}{"field": "age", "operator": c.operator, "value": c.value}, data)
		if err != nil {
			t.Fatalf("EvaluateCondition(%s) failed: %v", c.operator, err)
		}
		if met != c.want {
			t.Errorf("EvaluateCondition(age %s %v) = %v, want %v", c.operator, c.value, met, c.want)
		}
	}
}

func TestEvaluateCondition_ContainsStartsEndsWith(t *testing.T) {
	data := map[string]interface{}{"name": "Jane Doe"}

	met, _ := EvaluateCondition(map[string]interface{}{"field": "name", "operator": "contains", "value": "Doe"}, data)
	if !met {
		t.Error("expected contains to match")
	}
	met, _ = EvaluateCondition(map[string]interface{}{"field": "name", "operator": "starts_with", "value": "Jane"}, data)
	if !met {
		t.Error("expected starts_with to match")
	}
	met, _ = EvaluateCondition(map[string]interface{}{"field": "name", "operator": "ends_with", "value": "Doe"}, data)
	if !met {
		t.Error("expected ends_with to match")
	}
}

func TestEvaluateCondition_RegexMatch(t *testing.T) {
	data := map[string]interface{}{"mrn": "MRN12345"}
	met, err := EvaluateCondition(map[string]interface{}{"field": "mrn", "operator": "regex_match", "value": "^MRN[0-9]+$"}, data)
	if err != nil {
		t.Fatalf("EvaluateCondition failed: %v", err)
	}
	if !met {
		t.Error("expected regex_match to match")
	}

	_, err = EvaluateCondition(map[string]interface{}{"field": "mrn", "operator": "regex_match", "value": "([invalid"}, data)
	if err == nil {
		t.Error("expected an error for an invalid regex pattern")
	}
}

func TestEvaluateCondition_InList(t *testing.T) {
	data := map[string]interface{}{"status": "cancelled"}
	met, err := EvaluateCondition(map[string]interface{}{
		"field": "status", "operator": "in_list",
		"value": []interface{}{"cancelled", "rejected"},
	}, data)
	if err != nil {
		t.Fatalf("EvaluateCondition failed: %v", err)
	}
	if !met {
		t.Error("expected in_list to match")
	}
}

func TestEvaluateCondition_CompareToField(t *testing.T) {
	data := map[string]interface{}{"expected": "US", "actual": "US"}
	met, err := EvaluateCondition(map[string]interface{}{"field": "actual", "operator": "equals", "compareToField": "expected"}, data)
	if err != nil {
		t.Fatalf("EvaluateCondition failed: %v", err)
	}
	if !met {
		t.Error("expected cross-field comparison to match")
	}
}

func TestEvaluateCondition_UnsupportedOperator(t *testing.T) {
	_, err := EvaluateCondition(map[string]interface{}{"field": "x", "operator": "bogus_operator", "value": 1}, map[string]interface{}{"x": 1})
	if err == nil {
		t.Error("expected an error for an unsupported operator")
	}
}

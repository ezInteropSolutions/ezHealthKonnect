package fhirpath_test

import (
	"testing"

	"ezhealthkonnect/fhir/r4/fhirpath"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func evalOnResource(t *testing.T, expr string, resource map[string]interface{}) bool {
	t.Helper()
	node, err := fhirpath.Compile(expr)
	require.NoError(t, err, "expression should compile: %s", expr)
	b, err := fhirpath.EvalBool(node, resource)
	require.NoError(t, err, "expression should evaluate: %s", expr)
	return b
}

func TestCompile_Syntax(t *testing.T) {
	cases := []string{
		"true",
		"1 + 1",
		"2 + 2 = 4",
		"gender",
		"name.exists()",
		"name.given.first()",
		"telecom.where(system = 'phone').exists()",
		"identifier.count() > 0",
		"(identifier.count() + name.count()) > 0",
		"a.empty() or b.empty()",
		"a.exists() and b.exists()",
		"type = 'document' implies (identifier.exists())",
		"fullUrl.contains('/_history/').not()",
		"text.`div`.exists()",
		"%resource.type = 'batch'",
		"entry.all(request.exists() = true)",
	}
	for _, expr := range cases {
		t.Run(expr, func(t *testing.T) {
			_, err := fhirpath.Compile(expr)
			assert.NoError(t, err, "expected %q to compile", expr)
		})
	}
}

func TestCompile_UnsupportedSyntax_FailsGracefully(t *testing.T) {
	cases := []string{
		"descendants().count() > 0",
		"member.resolve().exists()",
		"a.iif(empty(), true, false)",
		"a.ofType(Practitioner)",
		"entry.first().resource.is(Composition)",
		"fullUrl&resource.meta.versionId",
		"$this = 'x'",
		"'#' in (a | b)",
	}
	for _, expr := range cases {
		t.Run(expr, func(t *testing.T) {
			_, err := fhirpath.Compile(expr)
			assert.Error(t, err, "expected %q to fail to compile (outside supported subset)", expr)
		})
	}
}

func TestEval_FieldNavigationAndExistence(t *testing.T) {
	res := map[string]interface{}{
		"resourceType": "Patient",
		"name":         []interface{}{map[string]interface{}{"family": "Doe"}},
	}
	assert.True(t, evalOnResource(t, "name.exists()", res))
	assert.False(t, evalOnResource(t, "name.empty()", res))
	assert.False(t, evalOnResource(t, "telecom.exists()", res))
	assert.True(t, evalOnResource(t, "telecom.empty()", res))
}

func TestEval_WhereAndCount(t *testing.T) {
	res := map[string]interface{}{
		"telecom": []interface{}{
			map[string]interface{}{"system": "phone", "value": "555-1111"},
			map[string]interface{}{"system": "email", "value": "a@b.com"},
		},
	}
	assert.True(t, evalOnResource(t, "telecom.where(system = 'phone').exists()", res))
	assert.False(t, evalOnResource(t, "telecom.where(system = 'fax').exists()", res))
	assert.True(t, evalOnResource(t, "telecom.count() = 2", res))
}

func TestEval_AndOrImplies(t *testing.T) {
	res := map[string]interface{}{"type": "document", "identifier": map[string]interface{}{"system": "x"}}
	assert.True(t, evalOnResource(t, "type = 'document' implies identifier.exists()", res))

	res2 := map[string]interface{}{"type": "transaction"}
	// implies is vacuously true when the antecedent is false
	assert.True(t, evalOnResource(t, "type = 'document' implies identifier.exists()", res2))

	res3 := map[string]interface{}{"type": "document"}
	assert.False(t, evalOnResource(t, "type = 'document' implies identifier.exists()", res3))
}

func TestEval_EnvVarResource(t *testing.T) {
	res := map[string]interface{}{"resourceType": "Bundle", "type": "batch"}
	assert.True(t, evalOnResource(t, "%resource.type = 'batch'", res))
	assert.False(t, evalOnResource(t, "%resource.type = 'transaction'", res))
}

func TestEval_ArithmeticAndRelational(t *testing.T) {
	res := map[string]interface{}{
		"identifier": []interface{}{map[string]interface{}{"value": "1"}},
		"name":       []interface{}{},
	}
	assert.True(t, evalOnResource(t, "(identifier.count() + name.count()) > 0", res))

	res2 := map[string]interface{}{"identifier": []interface{}{}, "name": []interface{}{}}
	assert.False(t, evalOnResource(t, "(identifier.count() + name.count()) > 0", res2))
}

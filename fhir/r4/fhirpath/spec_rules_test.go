package fhirpath_test

// Regression fixtures using the REAL 37 FHIR R4 invariants pulled directly
// from hl7.org/fhir/R4/<type>.profile.json for the 17 resource types this
// pipeline emits (7 universal — same rule on almost every resource type —
// plus 30 resource-specific). These are the exact expressions found during
// the investigation that motivated this package, not paraphrased or
// hand-simplified, so a pass here is direct evidence the real spec rules
// this package exists to run actually work.

import (
	"testing"

	"ezhealthkonnect/fhir/r4/fhirpath"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// realRuleExpressions is every constraint key this session found across the
// 17 emitted resource types, with its verbatim expression text.
var realRuleExpressions = map[string]string{
	// universal (attached to almost every resource type)
	"dom-2": "contained.contained.empty()",
	"dom-3": "contained.where((('#'+id in (%resource.descendants().reference | %resource.descendants().as(canonical) | %resource.descendants().as(uri) | %resource.descendants().as(url))) or descendants().where(reference = '#').exists() or descendants().where(as(canonical) = '#').exists() or descendants().where(as(canonical) = '#').exists()).not()).trace('unmatched', id).empty()",
	"dom-4": "contained.meta.versionId.empty() and contained.meta.lastUpdated.empty()",
	"dom-5": "contained.meta.security.empty()",
	"dom-6": "text.`div`.exists()",
	"ele-1": "hasValue() or (children().count() > id.count())",
	"ext-1": "extension.exists() != value.exists()",
	// resource-specific
	"ait-1":  "verificationStatus.coding.where(system = 'http://terminology.hl7.org/CodeSystem/allergyintolerance-verification' and code = 'entered-in-error').exists() or clinicalStatus.exists()",
	"ait-2":  "verificationStatus.coding.where(system = 'http://terminology.hl7.org/CodeSystem/allergyintolerance-verification' and code = 'entered-in-error').empty() or clinicalStatus.empty()",
	"bdl-1":  "total.empty() or (type = 'searchset') or (type = 'history')",
	"bdl-2":  "entry.search.empty() or (type = 'searchset')",
	"bdl-3":  "entry.all(request.exists() = (%resource.type = 'batch' or %resource.type = 'transaction' or %resource.type = 'history'))",
	"bdl-4":  "entry.all(response.exists() = (%resource.type = 'batch-response' or %resource.type = 'transaction-response' or %resource.type = 'history'))",
	"bdl-5":  "resource.exists() or request.exists() or response.exists()",
	"bdl-7":  "(type = 'history') or entry.where(fullUrl.exists()).select(fullUrl&resource.meta.versionId).isDistinct()",
	"bdl-8":  "fullUrl.contains('/_history/').not()",
	"bdl-9":  "type = 'document' implies (identifier.system.exists() and identifier.value.exists())",
	"bdl-10": "type = 'document' implies (timestamp.hasValue())",
	"bdl-11": "type = 'document' implies entry.first().resource.is(Composition)",
	"bdl-12": "type = 'message' implies entry.first().resource.is(MessageHeader)",
	"cmp-1":  "text.exists() or entry.exists() or section.exists()",
	"cmp-2":  "emptyReason.empty() or entry.empty()",
	"con-1":  "summary.exists() or assessment.exists()",
	"con-2":  "code.exists() or detail.exists()",
	"con-3":  "clinicalStatus.exists() or verificationStatus.coding.where(system='http://terminology.hl7.org/CodeSystem/condition-ver-status' and code = 'entered-in-error').exists() or category.select($this='problem-list-item').empty()",
	"con-4":  "abatement.empty() or clinicalStatus.coding.where(system='http://terminology.hl7.org/CodeSystem/condition-clinical' and (code='resolved' or code='remission' or code='inactive')).exists()",
	"con-5":  "verificationStatus.coding.where(system='http://terminology.hl7.org/CodeSystem/condition-ver-status' and code='entered-in-error').empty() or clinicalStatus.empty()",
	"ctm-1":  "onBehalfOf.exists() implies (member.resolve().iif(empty(), true, ofType(Practitioner).exists()))",
	"gol-1":  "(detail.exists() and measure.exists()) or detail.exists().not()",
	"imm-1":  "documentType.exists() or reference.exists()",
	"obs-3":  "low.exists() or high.exists() or text.exists()",
	"obs-6":  "dataAbsentReason.empty() or value.empty()",
	"obs-7":  "value.empty() or component.code.where(coding.intersect(%resource.code.coding).exists()).empty()",
	"org-1":  "(identifier.count() + name.count()) > 0",
	"org-2":  "where(use = 'home').empty()",
	"org-3":  "where(use = 'home').empty()",
	"pat-1":  "name.exists() or telecom.exists() or address.exists() or organization.exists()",
}

// deferredRuleKeys are the 6 real rules (out of 37) that use FHIRPath
// features this package deliberately doesn't implement yet (descendants(),
// resolve(), iif(), ofType(), is(), the "&" concatenation operator, $this) —
// see doc.go. Compiling these MUST fail, not panic or silently misbehave,
// since a validator relies on that error signal to skip the rule gracefully.
var deferredRuleKeys = map[string]bool{
	"dom-3": true, "bdl-7": true, "bdl-11": true, "bdl-12": true, "con-3": true, "ctm-1": true,
}

func TestSpecRules_CompileStatusMatchesSurvey(t *testing.T) {
	for key, expr := range realRuleExpressions {
		key, expr := key, expr
		t.Run(key, func(t *testing.T) {
			_, err := fhirpath.Compile(expr)
			if deferredRuleKeys[key] {
				assert.Error(t, err, "%s uses deferred FHIRPath features and should fail to compile: %s", key, expr)
			} else {
				assert.NoError(t, err, "%s should compile with the supported subset: %s", key, expr)
			}
		})
	}
}

// The remaining tests prove actual evaluation correctness (not just
// successful parsing) for a representative spread of the real rules —
// different resource types, different function combinations.

func mustCompile(t *testing.T, key string) *fhirpath.Rule {
	t.Helper()
	expr := realRuleExpressions[key]
	rule, err := fhirpath.CompileRule(key, "error", "", expr)
	require.NoError(t, err, "%s must compile: %s", key, expr)
	return rule
}

func TestSpecRules_Ele1_HasValueOrChildren(t *testing.T) {
	rule := mustCompile(t, "ele-1")

	// hasValue() is always false here — the focus item is a map (an element
	// wrapper), and only a bare scalar satisfies hasValue() in this
	// codebase's plain-JSON resource representation (see hasValueFunc's own
	// comment). This case passes via the children-count fallback instead:
	// 2 children ("code","id") > 1 id.
	pass, err := rule.Evaluate(map[string]interface{}{"code": "final", "id": "x"})
	require.NoError(t, err)
	assert.True(t, pass)

	passChildren, err := rule.Evaluate(map[string]interface{}{"coding": []interface{}{"x"}}) // object with children, no id
	require.NoError(t, err)
	assert.True(t, passChildren)

	fail, err := rule.Evaluate(map[string]interface{}{"id": "only-id"}) // object, only id child
	require.NoError(t, err)
	assert.False(t, fail)
}

func TestSpecRules_Ext1_ExtensionXorValue(t *testing.T) {
	rule := mustCompile(t, "ext-1")

	onlyValue, err := rule.Evaluate(map[string]interface{}{"valueString": "x"})
	require.NoError(t, err)
	assert.True(t, onlyValue)

	both, err := rule.Evaluate(map[string]interface{}{
		"extension": []interface{}{map[string]interface{}{"url": "x"}},
		"value":     "y",
	})
	require.NoError(t, err)
	assert.False(t, both)
}

func TestSpecRules_Obs6_DataAbsentReasonXorValue(t *testing.T) {
	rule := mustCompile(t, "obs-6")

	pass, err := rule.Evaluate(map[string]interface{}{
		"resourceType": "Observation", "dataAbsentReason": map[string]interface{}{"text": "not done"},
	})
	require.NoError(t, err)
	assert.True(t, pass)

	fail, err := rule.Evaluate(map[string]interface{}{
		"resourceType": "Observation", "dataAbsentReason": map[string]interface{}{}, "valueString": "120",
	})
	require.NoError(t, err)
	assert.False(t, fail)
}

func TestSpecRules_Bdl1_TotalOnlyForSearchsetOrHistory(t *testing.T) {
	rule := mustCompile(t, "bdl-1")

	pass, err := rule.Evaluate(map[string]interface{}{"type": "searchset", "total": float64(5)})
	require.NoError(t, err)
	assert.True(t, pass)

	fail, err := rule.Evaluate(map[string]interface{}{"type": "transaction", "total": float64(5)})
	require.NoError(t, err)
	assert.False(t, fail)
}

func TestSpecRules_Bdl9_DocumentRequiresIdentifier(t *testing.T) {
	rule := mustCompile(t, "bdl-9")

	pass, err := rule.Evaluate(map[string]interface{}{
		"type": "document", "identifier": map[string]interface{}{"system": "urn:x", "value": "1"},
	})
	require.NoError(t, err)
	assert.True(t, pass)

	fail, err := rule.Evaluate(map[string]interface{}{"type": "document"})
	require.NoError(t, err)
	assert.False(t, fail)

	notApplicable, err := rule.Evaluate(map[string]interface{}{"type": "transaction"})
	require.NoError(t, err)
	assert.True(t, notApplicable)
}

func TestSpecRules_Pat1_DemographicsRequired(t *testing.T) {
	rule := mustCompile(t, "pat-1")

	pass, err := rule.Evaluate(map[string]interface{}{
		"resourceType": "Patient", "name": []interface{}{map[string]interface{}{"family": "Doe"}},
	})
	require.NoError(t, err)
	assert.True(t, pass)

	fail, err := rule.Evaluate(map[string]interface{}{"resourceType": "Patient"})
	require.NoError(t, err)
	assert.False(t, fail)
}

func TestSpecRules_Con4_AbatementRequiresResolvedStatus(t *testing.T) {
	rule := mustCompile(t, "con-4")

	pass, err := rule.Evaluate(map[string]interface{}{
		"abatementDateTime": "2024-01-01",
		"clinicalStatus": map[string]interface{}{
			"coding": []interface{}{map[string]interface{}{
				"system": "http://terminology.hl7.org/CodeSystem/condition-clinical", "code": "resolved",
			}},
		},
	})
	require.NoError(t, err)
	assert.True(t, pass)

	fail, err := rule.Evaluate(map[string]interface{}{
		"abatementDateTime": "2024-01-01",
		"clinicalStatus": map[string]interface{}{
			"coding": []interface{}{map[string]interface{}{
				"system": "http://terminology.hl7.org/CodeSystem/condition-clinical", "code": "active",
			}},
		},
	})
	require.NoError(t, err)
	assert.False(t, fail)

	notApplicable, err := rule.Evaluate(map[string]interface{}{})
	require.NoError(t, err)
	assert.True(t, notApplicable)
}

func TestSpecRules_Org1_IdentifierOrNameRequired(t *testing.T) {
	rule := mustCompile(t, "org-1")

	pass, err := rule.Evaluate(map[string]interface{}{
		"name": []interface{}{"Acme Health"},
	})
	require.NoError(t, err)
	assert.True(t, pass)

	fail, err := rule.Evaluate(map[string]interface{}{})
	require.NoError(t, err)
	assert.False(t, fail)
}

func TestSpecRules_Gol1_MeasureRequiresDetail(t *testing.T) {
	rule := mustCompile(t, "gol-1")

	passBoth, err := rule.Evaluate(map[string]interface{}{"detail": "x", "measure": "y"})
	require.NoError(t, err)
	assert.True(t, passBoth)

	passNeither, err := rule.Evaluate(map[string]interface{}{})
	require.NoError(t, err)
	assert.True(t, passNeither, "vacuously true — the rule only applies once detail is present")

	fail, err := rule.Evaluate(map[string]interface{}{"detail": "x"}) // detail without measure
	require.NoError(t, err)
	assert.False(t, fail)
}

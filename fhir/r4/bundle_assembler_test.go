package r4_test

import (
	"strings"
	"testing"

	"ezhealthkonnect/fhir/r4"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssembleEntries_AssignsAbsoluteUniqueFullURLs(t *testing.T) {
	resources := []map[string]interface{}{
		{"resourceType": "Patient", "id": "patient-1"},
		{"resourceType": "Condition", "id": "condition-1"},
		{"resourceType": "Condition", "id": "condition-1"}, // duplicate id, different resource
	}

	entries := r4.AssembleEntries(resources)
	require.Len(t, entries, 3)

	seen := map[string]bool{}
	for _, raw := range entries {
		entry := raw.(map[string]interface{})
		fullURL, _ := entry["fullUrl"].(string)
		assert.True(t, strings.HasPrefix(fullURL, "urn:uuid:"), "fullUrl %q must be an absolute urn:uuid:", fullURL)
		assert.False(t, seen[fullURL], "fullUrl %q must be unique", fullURL)
		seen[fullURL] = true
	}
}

func TestAssembleEntries_RewritesInternalReferences(t *testing.T) {
	resources := []map[string]interface{}{
		{"resourceType": "Patient", "id": "patient-1"},
		{
			"resourceType": "Condition",
			"id":           "condition-1",
			"subject":      map[string]interface{}{"reference": "Patient/patient-1"},
		},
	}

	entries := r4.AssembleEntries(resources)
	require.Len(t, entries, 2)

	var patientFullURL string
	for _, raw := range entries {
		entry := raw.(map[string]interface{})
		res := entry["resource"].(map[string]interface{})
		if res["resourceType"] == "Patient" {
			patientFullURL = entry["fullUrl"].(string)
		}
	}
	require.NotEmpty(t, patientFullURL)

	for _, raw := range entries {
		entry := raw.(map[string]interface{})
		res := entry["resource"].(map[string]interface{})
		if res["resourceType"] != "Condition" {
			continue
		}
		subject := res["subject"].(map[string]interface{})
		assert.Equal(t, patientFullURL, subject["reference"],
			"Condition.subject.reference must be rewritten to the Patient's urn:uuid: fullUrl")
	}
}

func TestRewriteReferences_LeavesUnknownReferencesUntouched(t *testing.T) {
	node := map[string]interface{}{
		"reference": "Practitioner/does-not-exist-in-bundle",
	}
	result := r4.RewriteReferences(node, map[string]string{
		"Patient/patient-1": "urn:uuid:abc",
	})
	resultMap := result.(map[string]interface{})
	assert.Equal(t, "Practitioner/does-not-exist-in-bundle", resultMap["reference"])
}

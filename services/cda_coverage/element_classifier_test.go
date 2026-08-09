// services/cda_coverage/element_classifier_test.go
package cdacoverage

import "testing"

func TestIsAdministrativeElementPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		// Participation wrappers -- whole subtree excluded, at any depth.
		{"author itself", "author[0]", true},
		{"performer itself", "performer[0]", true},
		{"informant itself", "informant[0]", true},
		{"participant itself", "participant[0]", true},
		{"dataEnterer itself", "dataEnterer[0]", true},
		{"custodian itself", "custodian[0]", true},
		{"deep under author", "author[0].assignedAuthor[0].addr[0].postalCode[0]", true},
		{"real-world nested case: author under entryRelationship/observation", "act[0].entryRelationship[0].observation[0].author[0].assignedAuthor[0].addr[0].postalCode[0]", true},
		{"performer's assignedEntity id", "performer[0].assignedEntity[0].id[0]", true},

		// CDA-infrastructure leaf tags -- only when they're the item itself.
		{"templateId itself", "encounter[0].templateId[0]", true},
		{"nullFlavor itself", "value[0].nullFlavor[0]", true},
		{"realmCode itself", "realmCode[0]", true},

		// Structural container/wrapper nodes -- only when they're the item
		// itself, not their clinical children.
		{"bare act wrapper", "act[0]", true},
		{"bare entryRelationship wrapper", "act[0].entryRelationship[0]", true},
		{"nested observation wrapper", "act[0].entryRelationship[0].observation[0]", true},
		{"bare component wrapper", "component[0]", true},
		{"bare organizer wrapper", "organizer[0]", true},
		{"bare substanceAdministration wrapper", "substanceAdministration[0]", true},
		{"bare procedure wrapper", "procedure[0]", true},
		{"bare encounter wrapper", "encounter[0]", true},
		{"bare supply wrapper", "supply[0]", true},
		{"bare entry wrapper", "entry[0]", true},
		{"bare patient wrapper (recordTarget.patientRole.patient itself)", "recordTarget[0].patientRole[0].patient[0]", true},
		{"bare location wrapper", "encounter[0].location[0]", true},
		{"bare healthCareFacility wrapper", "encounter[0].location[0].healthCareFacility[0]", true},
		{"patient's own child stays clinical", "recordTarget[0].patientRole[0].patient[0].administrativeGenderCode[0]", false},
		{"healthCareFacility's own code stays clinical", "encounter[0].location[0].healthCareFacility[0].code[0]", false},

		// Clinical fields -- never excluded, including "id" (explicitly
		// clinically relevant per user direction, unlike templateId).
		{"id is clinical", "id[0]", false},
		{"code is clinical", "code[0]", false},
		{"value is clinical", "value[0]", false},
		{"effectiveTime is clinical", "effectiveTime[0]", false},
		{"effectiveTime.low is clinical", "effectiveTime[0].low[0]", false},
		{"statusCode is clinical", "statusCode[0]", false},
		{"severity is clinical", "severity[0]", false},

		// The real-world proof this classifier exists for: a nested
		// supporting observation's OWN code/value/statusCode are still
		// clinical, even though the path crosses TWO structural wrapper
		// segments (entryRelationship, observation) to get there -- only
		// the wrapper segments themselves are excluded, not what's beneath
		// them.
		{"nested observation's own code stays clinical", "act[0].entryRelationship[0].observation[0].code[0]", false},
		{"nested observation's own value stays clinical", "act[0].entryRelationship[0].observation[0].value[0]", false},
		{"nested observation's own statusCode stays clinical", "act[0].entryRelationship[0].observation[0].statusCode[0]", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAdministrativeElementPath(tt.path)
			if got != tt.want {
				t.Errorf("isAdministrativeElementPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

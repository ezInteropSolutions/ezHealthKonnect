package ncpdp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const testSchemaDir = "schemas/script_2017071"

func TestSchemaLoader_LoadsRealSchema(t *testing.T) {
	loader, err := NewNCPDPSchemaLoader(testSchemaDir)
	require.NoError(t, err)

	spec := loader.Spec()
	require.NotNil(t, spec)
	require.Equal(t, "20170715", spec.Version)
	require.Equal(t, "SCRIPT", spec.MessageAttrs.TransactionDomain)

	hdr := spec.Header()
	require.NotNil(t, hdr, "expected Header group to resolve via HeaderGroupKey")
	require.NotNil(t, hdr.FieldByKey("to"))
	require.NotNil(t, hdr.FieldByKey("toQualifier"))

	tx, err := loader.GetTransaction("NewRx")
	require.NoError(t, err)
	require.NotNil(t, tx.GroupRefByKey("patient"))
	require.NotNil(t, tx.GroupRefByKey("prescriber"))
	require.NotNil(t, tx.GroupRefByKey("medicationPrescribed"))

	// Spot-check a couple of shared groups resolve with their expected
	// nested structure.
	medRx, err := loader.GetGroup("MedicationPrescribed")
	require.NoError(t, err)
	require.NotNil(t, medRx.FieldByKey("drugDescription"))
	require.NotNil(t, medRx.GroupRefByKey("drugCoded"))
	require.NotNil(t, medRx.GroupRefByKey("quantity"))
	require.Equal(t, []string{
		"DrugDescription", "DrugCoded", "Quantity", "WrittenDate",
		"Substitutions", "NumberOfRefills", "Sig", "RxFillIndicator", "OtherMedicationDate",
	}, medRx.ElementOrder)
}

func TestSchemaLoader_FailsFastOnUnknownGroupReference(t *testing.T) {
	groups := map[string]*NCPDPGroupDef{
		"A": {Key: "A", XMLElement: "A", Groups: []NCPDPGroupRef{{Key: "b", GroupKey: "DoesNotExist"}}},
	}
	err := validateGroupReferences(groups, map[string]*NCPDPTransactionDef{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "DoesNotExist")
}

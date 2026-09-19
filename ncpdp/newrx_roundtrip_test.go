package ncpdp_test

import (
	"os"
	"testing"

	"ezhealthkonnect/ncpdp"
	ncpdpbuilder "ezhealthkonnect/ncpdp/builder"

	"github.com/stretchr/testify/require"
)

// TestNewRx_RealSample_ParseExtractsRealFields proves ncpdp.ParseMessage
// against a complete, real, unedited SCRIPT v2017071 NewRx sample (sourced
// from dgoradia/ncpdp's own testdata/sample-newrx.xml, kept verbatim at
// testdata/real_samples/dgoradia_sample_newrx.xml — see its own sourceRefs
// entries in the schema files for provenance) — spot-checking real values
// across Header, Patient, Pharmacy, Prescriber, and MedicationPrescribed,
// not just "parsing didn't error."
func TestNewRx_RealSample_ParseExtractsRealFields(t *testing.T) {
	spec := loadRealSpec(t)
	raw := readRealSample(t)

	result, err := ncpdp.ParseMessage(spec, raw)
	require.NoError(t, err)
	require.Equal(t, "NewRx", result.TransactionType)

	require.Equal(t, "SCRIPT", result.MessageAttrs["TransactionDomain"])
	require.Equal(t, "20170715", result.MessageAttrs["DatatypesVersion"])

	require.Equal(t, "6557744", result.Header["to"])
	require.Equal(t, "P", result.Header["toQualifier"])
	require.Equal(t, "6128890368017", result.Header["from"])
	require.Equal(t, "D", result.Header["fromQualifier"])
	require.Equal(t, "app-515537252384789", result.Header["messageID"])
	require.Equal(t, "2022-09-24T19:27:22Z", result.Header["sentTime"])
	require.Equal(t, "515537246945306", result.Header["prescriberOrderNumber"])

	security, ok := result.Header["security"].(map[string]interface{})
	require.True(t, ok, "expected header.security to be present")
	sender, ok := security["sender"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "1105", sender["tertiaryIdentification"])
	receiver, ok := security["receiver"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "142", receiver["tertiaryIdentification"])

	senderSoftware, ok := result.Header["senderSoftware"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "Elation Health", senderSoftware["senderSoftwareDeveloper"])
	require.Equal(t, "ElationEMR", senderSoftware["senderSoftwareProduct"])
	require.Equal(t, "3.0", senderSoftware["senderSoftwareVersionRelease"])

	mailbox, ok := result.Header["mailbox"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "b4cb9f7038d849339060b7ccf1c1104d", mailbox["deliveredID"])

	// Patient
	patient := getMap(t, result.Body, "patient")
	humanPatient := getMap(t, patient, "humanPatient")
	name := getMap(t, humanPatient, "name")
	require.Equal(t, "Jenny", name["lastName"])
	require.Equal(t, "Craigling", name["firstName"])
	require.Equal(t, "F", humanPatient["gender"])
	dob := getMap(t, humanPatient, "dateOfBirth")
	require.Equal(t, "1984-09-09", dob["date"])
	patientAddr := getMap(t, humanPatient, "address")
	require.Equal(t, "2015 Favorite Ave", patientAddr["addressLine1"])
	require.Equal(t, "Apt 999", patientAddr["addressLine2"])
	require.Equal(t, "Miami", patientAddr["city"])
	require.Equal(t, "33333", patientAddr["postalCode"])
	patientPhones := getMap(t, humanPatient, "communicationNumbers")
	patientPrimaryPhone := getMap(t, patientPhones, "primaryTelephone")
	require.Equal(t, "3738235389", patientPrimaryPhone["number"])

	// Pharmacy
	pharmacy := getMap(t, result.Body, "pharmacy")
	require.Equal(t, "A+ Drugs", pharmacy["businessName"])
	pharmacyID := getMap(t, pharmacy, "identification")
	require.Equal(t, "6557744", pharmacyID["ncpdpid"])
	require.Equal(t, "1142138869", pharmacyID["npi"])

	// Prescriber
	prescriber := getMap(t, result.Body, "prescriber")
	nonVet := getMap(t, prescriber, "nonVeterinarian")
	prescriberID := getMap(t, nonVet, "identification")
	require.Equal(t, "BB8027505", prescriberID["deaNumber"])
	require.Equal(t, "1939842031", prescriberID["npi"])
	prescriberName := getMap(t, nonVet, "name")
	require.Equal(t, "Bless", prescriberName["lastName"])
	require.Equal(t, "Janine", prescriberName["firstName"])
	require.Equal(t, "MD", prescriberName["suffix"])
	practiceLoc := getMap(t, nonVet, "practiceLocation")
	require.Equal(t, "Hey Friend", practiceLoc["businessName"])
	prescriberPhones := getMap(t, nonVet, "communicationNumbers")
	require.Equal(t, "4593423649", getMap(t, prescriberPhones, "primaryTelephone")["number"])
	require.Equal(t, "4593423650", getMap(t, prescriberPhones, "fax")["number"])

	// MedicationPrescribed
	med := getMap(t, result.Body, "medicationPrescribed")
	require.Equal(t, "Ondansetron 8 mg Tab Disintegrating", med["drugDescription"])
	require.Equal(t, "0", med["substitutions"])
	require.Equal(t, "0", med["numberOfRefills"])
	require.Equal(t, "All Fill Statuses", med["rxFillIndicator"])

	drugCoded := getMap(t, med, "drugCoded")
	productCode := getMap(t, drugCoded, "productCode")
	require.Equal(t, "62135012230", productCode["code"])
	require.Equal(t, "ND", productCode["qualifier"])
	drugDBCode := getMap(t, drugCoded, "drugDBCode")
	require.Equal(t, "312087", drugDBCode["code"])
	require.Equal(t, "SCD", drugDBCode["qualifier"])

	quantity := getMap(t, med, "quantity")
	require.Equal(t, "15", quantity["value"])
	require.Equal(t, "38", quantity["codeListQualifier"])
	uom := getMap(t, quantity, "quantityUnitOfMeasure")
	require.Equal(t, "C48542", uom["code"])

	writtenDate := getMap(t, med, "writtenDate")
	require.Equal(t, "2022-09-24", writtenDate["date"])

	sig := getMap(t, med, "sig")
	require.Equal(t, "1 tablet orally every 8 hours as needed for nausea, let dissolve then swallow with saliva", sig["sigText"])

	otherMedDate := getMap(t, med, "otherMedicationDate")
	require.Equal(t, "EffectiveDate", otherMedDate["otherMedicationDateQualifier"])
	innerDate := getMap(t, otherMedDate, "otherMedicationDate")
	require.Equal(t, "2022-09-24", innerDate["date"])
}

// TestNewRx_RealSample_RoundTripsThroughBuildAndReparse proves
// ncpdp/builder.BuildDocument against the SAME real sample's parsed output,
// then re-parses the built XML and confirms every field survives the round
// trip byte-for-byte (as canonical values, not raw XML) — the Phase 1
// acceptance bar this plan set for every transaction type.
func TestNewRx_RealSample_RoundTripsThroughBuildAndReparse(t *testing.T) {
	spec := loadRealSpec(t)
	raw := readRealSample(t)

	original, err := ncpdp.ParseMessage(spec, raw)
	require.NoError(t, err)

	built, err := ncpdpbuilder.BuildDocument(spec, "NewRx", original.Header, original.Body)
	require.NoError(t, err)
	require.Contains(t, built, "<NewRx>")
	require.Contains(t, built, "TransactionDomain=\"SCRIPT\"")

	reparsed, err := ncpdp.ParseMessage(spec, built)
	require.NoError(t, err)

	require.Equal(t, original.TransactionType, reparsed.TransactionType)
	require.Equal(t, original.Header, reparsed.Header)
	require.Equal(t, original.Body, reparsed.Body)
}

func loadRealSpec(t *testing.T) *ncpdp.NCPDPSpecDef {
	t.Helper()
	loader, err := ncpdp.NewNCPDPSchemaLoader("schemas/script_2017071")
	require.NoError(t, err)
	return loader.Spec()
}

func readRealSample(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/real_samples/dgoradia_sample_newrx.xml")
	require.NoError(t, err)
	return string(data)
}

func getMap(t *testing.T, parent map[string]interface{}, key string) map[string]interface{} {
	t.Helper()
	v, ok := parent[key]
	require.True(t, ok, "expected key %q to be present", key)
	m, ok := v.(map[string]interface{})
	require.True(t, ok, "expected key %q to be a map, got %T", key, v)
	return m
}

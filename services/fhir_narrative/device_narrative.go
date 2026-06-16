// services/fhir_narrative/device_narrative.go
// Generates XHTML narrative for a FHIR R4 DeviceUseStatement resource.

package fhirnarrative

// GenerateDeviceNarrative produces FHIR-compliant XHTML for a DeviceUseStatement resource.
func GenerateDeviceNarrative(r map[string]interface{}) string {
	rows := tableRow("Device", fhirStr(fhirMap(r, "device"), "display"))
	rows += tableRow("Status", fhirStr(r, "status"))

	if timing := fhirMap(r, "timingPeriod"); timing != nil {
		rows += tableRow("Start", fhirStr(timing, "start"))
		rows += tableRow("End", fhirStr(timing, "end"))
	} else if dt := fhirStr(r, "timingDateTime"); dt != "" {
		rows += tableRow("Date Used", dt)
	}

	rows += tableRow("Body Site", ccText(r["bodySite"]))
	rows += tableRow("Reason", ccText(r["reasonCode"]))

	return wrapDiv(heading("Medical Equipment / Device") + buildTable(rows))
}

// ─────────────────────────────────────────────────────────────────────────────
// EDI X12 837P → FHIR mapping — the config here is transcribed verbatim into
// database/migrations/V237, exactly matching pas_fhir_builder_test.go's own
// "config here is the source of truth" discipline.
//
// Run: go test ./services/ -v -run TestEDI837PFHIRBuilder
// ─────────────────────────────────────────────────────────────────────────────

package services

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors/payload"
	"ezhealthkonnect/services/executors/validation"
)

// ─────────────────────────────────────────────────────────────────────────────
// Fixture — a realistic multi-claim, multi-line-item 837P shape, hand-built
// to match edi/loop_engine.go's own PARSE-direction output exactly (verified
// directly against that file's matchSegmentSequence/matchLoops, not assumed):
// NM1/REF/DTP/PER/AMT/CRC/QTY/MEA/K3 are arrays (schema MaxUse ">1", global to
// the segment definition — even where a given loop only ever has one real
// occurrence), everything else here (CLM/HI/SV1/N3/N4/DMG/SBR/PAT/HL) is a
// bare object (MaxUse "1"). One billing provider; two 2000B subscribers — the
// first is her own patient (SBR02 "18", one claim/two service lines/two
// diagnoses), the second is a dependent's parent (SBR02 "19", one 2000C child
// with her own claim/one service line/one diagnosis) — proving both the
// "subscriber is patient" and "dependent is patient" branches of the derive
// script, and genuine multi-claim/multi-line handling end to end.
// ─────────────────────────────────────────────────────────────────────────────

func edi837PFixtureLoops() map[string]interface{} {
	return map[string]interface{}{
		"2000A": []interface{}{
			map[string]interface{}{
				"HL": map[string]interface{}{"hierarchicalIdNumber": "1", "hierarchicalLevelCode": "20", "hierarchicalChildCode": "1"},
				"loops": map[string]interface{}{
					"2010AA": map[string]interface{}{
						"NM1": []interface{}{
							map[string]interface{}{"entityIdentifierCode": "85", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "ACME CLINIC", "identificationCodeQualifier": "XX", "identificationCode": "1234567890"},
						},
					},
					"2000B": []interface{}{
						// Subscriber 1: her own patient (self), one claim / two lines / two diagnoses.
						map[string]interface{}{
							"HL":  map[string]interface{}{"hierarchicalIdNumber": "2", "hierarchicalParentIdNumber": "1", "hierarchicalLevelCode": "22", "hierarchicalChildCode": "0"},
							"SBR": map[string]interface{}{"payerResponsibilitySequenceNumberCode": "P", "individualRelationshipCode": "18"},
							"loops": map[string]interface{}{
								"2010BA": map[string]interface{}{
									"NM1": []interface{}{
										map[string]interface{}{"entityIdentifierCode": "IL", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "SMITH", "nameFirst": "JANE", "identificationCodeQualifier": "MI", "identificationCode": "SUB123"},
									},
									"DMG": map[string]interface{}{"dateTimePeriodFormatQualifier": "D8", "birthDate": "19750322", "genderCode": "F"},
								},
								"2010BB": map[string]interface{}{
									"NM1": []interface{}{
										map[string]interface{}{"entityIdentifierCode": "PR", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "PAYER1", "identificationCodeQualifier": "PI", "identificationCode": "PAYER001"},
									},
								},
								"2300": []interface{}{
									map[string]interface{}{
										"CLM": map[string]interface{}{
											"patientControlNumber": "CLM-SUB-001", "totalClaimChargeAmount": "350.00",
											"healthCareServiceLocation": map[string]interface{}{"placeOfServiceCode": "11", "facilityCodeQualifier": "B", "claimFrequencyCode": "1"},
										},
										"HI": []interface{}{
											map[string]interface{}{
												"codes": []interface{}{
													map[string]interface{}{"code": map[string]interface{}{"qualifier": "ABK", "code": "Z00.00"}},
													map[string]interface{}{"code": map[string]interface{}{"qualifier": "ABF", "code": "J06.9"}},
												},
											},
										},
										"loops": map[string]interface{}{
											"2310B": map[string]interface{}{
												"NM1": []interface{}{
													map[string]interface{}{"entityIdentifierCode": "82", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "JONES", "nameFirst": "ROB", "identificationCodeQualifier": "XX", "identificationCode": "1112223330"},
												},
											},
											"2400": []interface{}{
												map[string]interface{}{
													"LX": map[string]interface{}{"assignedNumber": "1"},
													"SV1": map[string]interface{}{
														"procedureCode": map[string]interface{}{"qualifier": "HC", "code": "99213"},
														"lineItemChargeAmount": "150.00", "unitOfMeasurementCode": "UN", "serviceUnitCount": "1",
														"diagnosisCodePointer": map[string]interface{}{"pointer1": "1"},
													},
													"DTP": []interface{}{
														map[string]interface{}{"dateTimeQualifier": "472", "dateTimePeriodFormatQualifier": "D8", "datePeriod": "20260115"},
													},
												},
												map[string]interface{}{
													"LX": map[string]interface{}{"assignedNumber": "2"},
													"SV1": map[string]interface{}{
														"procedureCode": map[string]interface{}{"qualifier": "HC", "code": "85025"},
														"lineItemChargeAmount": "200.00", "unitOfMeasurementCode": "UN", "serviceUnitCount": "1",
														"diagnosisCodePointer": map[string]interface{}{"pointer1": "1", "pointer2": "2"},
													},
													"DTP": []interface{}{
														map[string]interface{}{"dateTimeQualifier": "472", "dateTimePeriodFormatQualifier": "D8", "datePeriod": "20260115"},
													},
												},
											},
										},
									},
								},
							},
						},
						// Subscriber 2: parent of a dependent — the dependent is the patient.
						map[string]interface{}{
							"HL":  map[string]interface{}{"hierarchicalIdNumber": "3", "hierarchicalParentIdNumber": "1", "hierarchicalLevelCode": "22", "hierarchicalChildCode": "1"},
							"SBR": map[string]interface{}{"payerResponsibilitySequenceNumberCode": "P", "individualRelationshipCode": "19"},
							"loops": map[string]interface{}{
								"2010BA": map[string]interface{}{
									"NM1": []interface{}{
										map[string]interface{}{"entityIdentifierCode": "IL", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "DOE", "nameFirst": "JOHN", "identificationCodeQualifier": "MI", "identificationCode": "SUB456"},
									},
								},
								"2010BB": map[string]interface{}{
									"NM1": []interface{}{
										map[string]interface{}{"entityIdentifierCode": "PR", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "PAYER1", "identificationCodeQualifier": "PI", "identificationCode": "PAYER001"},
									},
								},
								"2000C": []interface{}{
									map[string]interface{}{
										"HL":  map[string]interface{}{"hierarchicalIdNumber": "4", "hierarchicalParentIdNumber": "3", "hierarchicalLevelCode": "23", "hierarchicalChildCode": "0"},
										"PAT": map[string]interface{}{"unitOfMeasurementCode": "01"},
										"loops": map[string]interface{}{
											"2010CA": map[string]interface{}{
												"NM1": []interface{}{
													map[string]interface{}{"entityIdentifierCode": "QC", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "DOE", "nameFirst": "JUNIOR", "identificationCodeQualifier": "MI", "identificationCode": "DEP789"},
												},
												"DMG": map[string]interface{}{"dateTimePeriodFormatQualifier": "D8", "birthDate": "20150610", "genderCode": "M"},
											},
											"2300": []interface{}{
												map[string]interface{}{
													"CLM": map[string]interface{}{
														"patientControlNumber": "CLM-DEP-001", "totalClaimChargeAmount": "95.00",
														"healthCareServiceLocation": map[string]interface{}{"placeOfServiceCode": "11", "facilityCodeQualifier": "B", "claimFrequencyCode": "1"},
													},
													"HI": []interface{}{
														map[string]interface{}{
															"codes": []interface{}{
																map[string]interface{}{"code": map[string]interface{}{"qualifier": "ABK", "code": "J06.9"}},
															},
														},
													},
													"loops": map[string]interface{}{
														"2400": []interface{}{
															map[string]interface{}{
																"LX": map[string]interface{}{"assignedNumber": "1"},
																"SV1": map[string]interface{}{
																	"procedureCode": map[string]interface{}{"qualifier": "HC", "code": "99212"},
																	"lineItemChargeAmount": "95.00", "unitOfMeasurementCode": "UN", "serviceUnitCount": "1",
																	"diagnosisCodePointer": map[string]interface{}{"pointer1": "1"},
																},
																"DTP": []interface{}{
																	map[string]interface{}{"dateTimeQualifier": "472", "dateTimePeriodFormatQualifier": "D8", "datePeriod": "20260118"},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Derive 837P Claim Contexts — the one enrichment.script step, pure data
// reshaping only (no FHIR resource-shape or profile knowledge), mirroring PAS's
// own "Derive PAS Computed Fields" script's documented scope boundary exactly.
// ─────────────────────────────────────────────────────────────────────────────

const derive837PClaimContextsScript = `
// -- Derive 837P Claim Contexts --
var parsed = input.parsedEDI || {};
var loops = parsed.loops || {};

// edi.parse's own "transactionSet" config key is NOT read by the executor
// (services/executors/transform/edi_parse_executor.go's ediParseConfig has
// no such field) -- the real variant resolution happens inside the parser
// via the composite ST01+GS08 lookup (edi/loop_engine.go), keyed off the
// FILE's own real GS08, not any step config. A stray 837I file landing in
// this professional-only mailbox would otherwise silently run through this
// SV1-shaped derive logic and produce wrong/incomplete Claim resources with
// no error. Guard explicitly instead.
if (parsed.transactionSet !== "837P") {
  return ({ _claim_contexts: [], _billing_provider: {} });
}

function arr(v) {
  if (!v) return [];
  return Array.isArray(v) ? v : [v];
}
function first(v) {
  var a = arr(v);
  return a.length > 0 ? a[0] : {};
}

function genderToFHIR(code) {
  if (code === "M") return "male";
  if (code === "F") return "female";
  return "unknown";
}

function relationshipToFHIR(code) {
  var map = { "18": "self", "01": "spouse", "19": "child", "20": "employee" };
  return map[code] || "other";
}

var billingLevel = first(loops["2000A"]);
var billingProviderLoop = (billingLevel.loops || {})["2010AA"] || {};
var billingNM1 = first(billingProviderLoop.NM1);
var billingProvider = {
  npi: billingNM1.identificationCode || "",
  name: billingNM1.nameLastOrOrganizationName || ""
};

// HI is maxUse ">1" -- a claim can carry SEVERAL separate HI segment
// occurrences at the same 2300 level (one per qualifier-group; see
// edi/schemas/x12_005010/segments/HI.json's own maxUse-correction note for
// why, found via testing against real, unedited samples), each with its own
// up-to-12-entry codes[] repeat group. Flatten every occurrence's codes[]
// into one combined list before filtering by qualifier.
function allHICodes(hiList) {
  var instances = arr(hiList);
  var out = [];
  for (var i = 0; i < instances.length; i++) {
    var codes = (instances[i] && instances[i].codes) || [];
    for (var j = 0; j < codes.length; j++) out.push(codes[j]);
  }
  return out;
}

function extractDiagnoses(codes) {
  var out = [];
  for (var i = 0; i < codes.length; i++) {
    var c = (codes[i] && codes[i].code) || {};
    if (!c.code) continue;
    out.push({ code: c.code, sequence: out.length + 1 });
  }
  return out;
}

function diagnosisPointers(sv1) {
  var ptr = (sv1 && sv1.diagnosisCodePointer) || {};
  var out = [];
  ["pointer1", "pointer2", "pointer3", "pointer4"].forEach(function (key) {
    if (ptr[key]) out.push(parseInt(ptr[key], 10));
  });
  return out;
}

function extractServiceLines(claimLoops) {
  var lines = arr((claimLoops || {})["2400"]);
  var out = [];
  for (var i = 0; i < lines.length; i++) {
    var line = lines[i];
    var lx = line.LX || {};
    var sv1 = line.SV1 || {};
    var proc = sv1.procedureCode || {};
    var dtpList = arr(line.DTP);
    var svc = resolveServicedDate(dtpList);
    out.push({
      sequence: parseInt(lx.assignedNumber || String(i + 1), 10),
      procedureCode: proc.code || "",
      procedureSystem: (proc.qualifier === "HC") ? "http://www.ama-assn.org/go/cpt" : "",
      quantity: sv1.serviceUnitCount ? Number(sv1.serviceUnitCount) : 1,
      net: sv1.lineItemChargeAmount ? Number(sv1.lineItemChargeAmount) : 0,
      servicedDate: svc.servicedDate,
      servicedStart: svc.servicedStart,
      servicedEnd: svc.servicedEnd,
      diagnosisPointers: diagnosisPointers(sv1)
    });
  }
  return out;
}

// DTP02 (Date/Time Period Format Qualifier) "D8" is a single CCYYMMDD date;
// "RD8" is a CCYYMMDD-CCYYMMDD range. Claim.item.serviced[x] is a choice type
// (servicedDate | servicedPeriod) -- only one of the two shapes below is ever
// populated per DTP*472 occurrence, matching that choice. Found only by
// testing against a real, unedited 837 sample (databricks-industry-
// solutions/x12-edi-parser's own CC_837P_EDI.txt/CC_837I_EDI.txt test
// fixtures) carrying genuine RD8 ranges -- the synthetic Go-test fixture
// only ever used D8 dates, so this gap was invisible there.
function resolveServicedDate(dtpList) {
  for (var d = 0; d < dtpList.length; d++) {
    if (dtpList[d].dateTimeQualifier !== "472") continue;
    var period = dtpList[d].datePeriod || "";
    if (dtpList[d].dateTimePeriodFormatQualifier === "RD8" && period.indexOf("-") !== -1) {
      var parts = period.split("-");
      return { servicedDate: "", servicedStart: parts[0] || "", servicedEnd: parts[1] || "" };
    }
    return { servicedDate: period, servicedStart: "", servicedEnd: "" };
  }
  return { servicedDate: "", servicedStart: "", servicedEnd: "" };
}

// Covers every 2310A-F role that is genuinely a CARE TEAM MEMBER (a person or
// organization providing care) -- 2310A Referring (DN), 2310B Rendering
// (82), 2310D Supervising (DQ). 2310C Service Facility Location (77) is a
// PLACE, not a care team member -- mapped separately to Claim.facility (see
// extractFacility below), matching the base FHIR R4 Claim resource's own
// dedicated 0..1 Reference field for exactly this concept. 2310E/2310F
// Ambulance Pick-up/Drop-off Location (PW/45) are also genuinely locations
// (addresses, not provider entities) with no equivalent base-Claim field --
// a named, deliberate gap, not modeled in this pass. Found via X12.org's own
// official COB example (Example 3a), whose 2310C carried a real NPI that a
// prior version of this function silently dropped entirely.
function extractCareTeam(claimLoops) {
  var team = [];
  var roleLoops = [
    { key: "2310A", role: "referring" },
    { key: "2310B", role: "rendering" },
    { key: "2310D", role: "supervising" }
  ];
  for (var r = 0; r < roleLoops.length; r++) {
    var list = arr((claimLoops || {})[roleLoops[r].key]);
    for (var i = 0; i < list.length; i++) {
      var nm1 = first(list[i].NM1);
      if (nm1.identificationCode) {
        team.push({ npi: nm1.identificationCode, role: roleLoops[r].role, sequence: team.length + 1 });
      }
    }
  }
  return team;
}

// Claim.facility (base FHIR R4, 0..1 Reference) -- "Facility where the
// services were provided." A logical (identifier-only) reference, same
// pattern as careTeam[].provider -- no separate Location resource is built,
// same rationale as the careTeam design note above.
function extractFacility(claimLoops) {
  var loop = (claimLoops || {})["2310C"];
  if (!loop) return {};
  var nm1 = first(loop.NM1);
  if (!nm1.identificationCode) return {};
  return { facilityNpi: nm1.identificationCode, facilityName: nm1.nameLastOrOrganizationName || "" };
}

// Claim.created is required by the base FHIR R4 Claim resource but has no
// X12 837 equivalent (837 carries no "claim record created" timestamp) --
// the same "record processing time, not sourced from the message" default
// convention BHT03/BHT04 (creation date/time) already establish for the
// interchange itself. Computed once for the whole file, not per claim.
var nowISO = new Date().toISOString();

function buildClaimContext(patientInfo, subscriberInfo, payerInfo, claimLoop) {
  var clm = claimLoop.CLM || {};
  var svcLoc = clm.healthCareServiceLocation || {};
  var facility = extractFacility(claimLoop.loops);
  var claim = {
    patientControlNumber: clm.patientControlNumber || "",
    totalChargeAmount: clm.totalClaimChargeAmount ? Number(clm.totalClaimChargeAmount) : 0,
    placeOfServiceCode: svcLoc.placeOfServiceCode || "",
    createdAt: nowISO,
    diagnosisList: extractDiagnoses(allHICodes(claimLoop.HI)),
    serviceLines: extractServiceLines(claimLoop.loops),
    careTeam: extractCareTeam(claimLoop.loops)
  };
  if (facility.facilityNpi) {
    claim.facilityNpi = facility.facilityNpi;
    claim.facilityName = facility.facilityName;
  }
  return {
    patientInfo: patientInfo,
    subscriberInfo: subscriberInfo,
    payerInfo: payerInfo,
    claim: claim
  };
}

function personFromNM1AndDMG(nm1, dmg, relationshipCode) {
  var gender = dmg.genderCode || "";
  return {
    firstName: nm1.nameFirst || "",
    lastName: nm1.nameLastOrOrganizationName || "",
    memberId: nm1.identificationCode || "",
    dob: dmg.birthDate || "",
    genderFHIR: genderToFHIR(gender),
    relationshipCode: relationshipCode || "",
    relationshipFHIR: relationshipToFHIR(relationshipCode || "")
  };
}

var claimContexts = [];

var billingLoops = billingLevel.loops || {};
var subscriberLevels = arr(billingLoops["2000B"]);
for (var s = 0; s < subscriberLevels.length; s++) {
  var subLevel = subscriberLevels[s];
  var subLoops = subLevel.loops || {};
  var sbr = subLevel.SBR || {};

  var subNM1 = first((subLoops["2010BA"] || {}).NM1);
  var subDMG = (subLoops["2010BA"] || {}).DMG || {};
  var subscriberInfo = personFromNM1AndDMG(subNM1, subDMG, "18");

  var payerNM1 = first((subLoops["2010BB"] || {}).NM1);
  var payerInfo = {
    name: payerNM1.nameLastOrOrganizationName || "",
    payerId: payerNM1.identificationCode || ""
  };

  var dependentLevels = arr(subLoops["2000C"]);
  if (dependentLevels.length > 0) {
    for (var d = 0; d < dependentLevels.length; d++) {
      var depLevel = dependentLevels[d];
      var depLoops = depLevel.loops || {};
      var depNM1 = first((depLoops["2010CA"] || {}).NM1);
      var depDMG = (depLoops["2010CA"] || {}).DMG || {};
      var depPAT = depLevel.PAT || {};
      // Real, spec-compliant 837 data carries the dependent's own relationship
      // code on PAT01 (2000C) -- confirmed against X12.org's own official
      // "Ben Kildare Service" 837P example, which leaves the subscriber's own
      // SBR02 (2000B) BLANK whenever a 2000C dependent loop exists and puts
      // the real value on PAT01 instead (HL*2...SBR*P**2222-SJ*******CI [SBR02
      // blank] -> HL*3...PAT*19 [child]). Prefer PAT01; fall back to SBR02 for
      // trading partners that populate it there instead (flexible, not rigid).
      var patientInfo = personFromNM1AndDMG(depNM1, depDMG, depPAT.individualRelationshipCode || sbr.individualRelationshipCode);
      if (!patientInfo.memberId) patientInfo.memberId = subscriberInfo.memberId + "-DEP" + (d + 1);

      var depClaims = arr(depLoops["2300"]);
      for (var dc = 0; dc < depClaims.length; dc++) {
        claimContexts.push(buildClaimContext(patientInfo, subscriberInfo, payerInfo, depClaims[dc]));
      }
    }
  } else {
    var subClaims = arr(subLoops["2300"]);
    for (var sc = 0; sc < subClaims.length; sc++) {
      claimContexts.push(buildClaimContext(subscriberInfo, subscriberInfo, payerInfo, subClaims[sc]));
    }
  }
}

return ({
  _claim_contexts: claimContexts,
  _billing_provider: billingProvider
});
`

// ─────────────────────────────────────────────────────────────────────────────
// fhir.build configs — transcribed verbatim into V237's SQL.
// ─────────────────────────────────────────────────────────────────────────────

func organization837PBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Organization",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirOrganization",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "literalValue": "organization-billing"},
			map[string]interface{}{"targetPath": "active", "literalValue": "true"},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": "_billing_provider.npi"},
			map[string]interface{}{"targetPath": "name", "sourcePath": "_billing_provider.name"},
		},
	}
}

func patient837PBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Patient",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPatients",
		"rowsPath":     "_claim_contexts",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "patientInfo.memberId"},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/x12-member-id"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": "patientInfo.memberId"},
			map[string]interface{}{"targetPath": "name[0].family", "sourcePath": "patientInfo.lastName"},
			map[string]interface{}{"targetPath": "name[0].given[0]", "sourcePath": "patientInfo.firstName"},
			map[string]interface{}{"targetPath": "birthDate", "sourcePath": "patientInfo.dob", "transform": "x12_date_to_fhir_date"},
			map[string]interface{}{"targetPath": "gender", "sourcePath": "patientInfo.genderFHIR"},
		},
	}
}

func coverage837PBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Coverage",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirCoverages",
		"rowsPath":     "_claim_contexts",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "claim.patientControlNumber", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "coverage-"}},
			map[string]interface{}{"targetPath": "status", "literalValue": "active"},
			map[string]interface{}{"targetPath": "beneficiary.reference", "sourcePath": "patientInfo.memberId", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"}},
			map[string]interface{}{"targetPath": "subscriberId", "sourcePath": "subscriberInfo.memberId"},
			map[string]interface{}{
				"targetPath": "subscriber.reference", "sourcePath": "subscriberInfo.memberId", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"},
				"condition": map[string]interface{}{"field": "patientInfo.relationshipCode", "operator": "equals", "value": "18"},
			},
			map[string]interface{}{"targetPath": "relationship.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/subscriber-relationship"},
			map[string]interface{}{"targetPath": "relationship.coding[0].code", "sourcePath": "patientInfo.relationshipFHIR"},
			map[string]interface{}{"targetPath": "payor[0].identifier.system", "literalValue": "http://ezhealthkonnect.local/x12-payer-id"},
			map[string]interface{}{"targetPath": "payor[0].identifier.value", "sourcePath": "payerInfo.payerId"},
			map[string]interface{}{"targetPath": "payor[0].display", "sourcePath": "payerInfo.name"},
		},
	}
}

func claim837PBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Claim",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirClaims",
		"rowsPath":     "_claim_contexts",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "claim.patientControlNumber", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "claim-"}},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/x12-claim-control-number"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": "claim.patientControlNumber"},
			map[string]interface{}{"targetPath": "status", "literalValue": "active"},
			map[string]interface{}{"targetPath": "type.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/claim-type"},
			map[string]interface{}{"targetPath": "type.coding[0].code", "literalValue": "professional"},
			map[string]interface{}{"targetPath": "use", "literalValue": "claim"},
			map[string]interface{}{"targetPath": "created", "sourcePath": "claim.createdAt"},
			// priority is required by base FHIR R4 Claim but has no X12 837
			// equivalent (that's a PAS/preauthorization concept, not a claim-
			// submission one) — fixed to "normal", a named, honest simplification.
			map[string]interface{}{"targetPath": "priority.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/processpriority"},
			map[string]interface{}{"targetPath": "priority.coding[0].code", "literalValue": "normal"},
			map[string]interface{}{"targetPath": "patient.reference", "sourcePath": "patientInfo.memberId", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"}},
			map[string]interface{}{"targetPath": "provider.reference", "literalValue": "Organization/organization-billing"},
			map[string]interface{}{"targetPath": "insurer.identifier.system", "literalValue": "http://ezhealthkonnect.local/x12-payer-id"},
			map[string]interface{}{"targetPath": "insurer.identifier.value", "sourcePath": "payerInfo.payerId"},
			// Claim.facility (base FHIR, 0..1) -- 2310C Service Facility Location, a
			// logical (identifier-only) reference, same pattern as careTeam[].provider
			// below. Only written when the claim actually carries one (most
			// professional claims served at the billing provider's own address
			// don't populate 2310C at all) -- found via X12.org's own COB example.
			map[string]interface{}{
				"targetPath": "facility.identifier.system", "literalValue": "http://hl7.org/fhir/sid/us-npi",
				"condition": map[string]interface{}{"field": "claim.facilityNpi", "operator": "exists"},
			},
			map[string]interface{}{"targetPath": "facility.identifier.value", "sourcePath": "claim.facilityNpi"},
			map[string]interface{}{"targetPath": "total.value", "sourcePath": "claim.totalChargeAmount"},
			map[string]interface{}{"targetPath": "total.currency", "literalValue": "USD"},
			map[string]interface{}{"targetPath": "insurance[0].sequence", "literalValue": "1"},
			map[string]interface{}{"targetPath": "insurance[0].focal", "literalValue": "true"},
			map[string]interface{}{"targetPath": "insurance[0].coverage.reference", "sourcePath": "claim.patientControlNumber", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Coverage/coverage-"}},
		},
		"repeatingGroups": []interface{}{
			map[string]interface{}{
				"targetPath": "diagnosis",
				"rowsPath":   "claim.diagnosisList",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "diagnosisCodeableConcept.coding[0].system", "literalValue": "http://hl7.org/fhir/sid/icd-10-cm"},
					map[string]interface{}{"targetPath": "diagnosisCodeableConcept.coding[0].code", "sourcePath": "code"},
					map[string]interface{}{"targetPath": "sequence", "sourcePath": "sequence"},
				},
			},
			map[string]interface{}{
				"targetPath": "item",
				"rowsPath":   "claim.serviceLines",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "sequence", "sourcePath": "sequence"},
					map[string]interface{}{"targetPath": "productOrService.coding[0].system", "sourcePath": "procedureSystem"},
					map[string]interface{}{"targetPath": "productOrService.coding[0].code", "sourcePath": "procedureCode"},
					map[string]interface{}{"targetPath": "quantity.value", "sourcePath": "quantity"},
					map[string]interface{}{"targetPath": "net.value", "sourcePath": "net"},
					map[string]interface{}{"targetPath": "net.currency", "literalValue": "USD"},
					map[string]interface{}{"targetPath": "servicedDate", "sourcePath": "servicedDate", "transform": "x12_date_to_fhir_date"},
					map[string]interface{}{"targetPath": "servicedPeriod.start", "sourcePath": "servicedStart", "transform": "x12_date_to_fhir_date"},
					map[string]interface{}{"targetPath": "servicedPeriod.end", "sourcePath": "servicedEnd", "transform": "x12_date_to_fhir_date"},
				},
			},
			map[string]interface{}{
				"targetPath": "careTeam",
				"rowsPath":   "claim.careTeam",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "sequence", "sourcePath": "sequence"},
					map[string]interface{}{"targetPath": "role.coding[0].code", "sourcePath": "role"},
					map[string]interface{}{"targetPath": "provider.identifier.system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
					map[string]interface{}{"targetPath": "provider.identifier.value", "sourcePath": "npi"},
				},
			},
		},
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-EDI837P-FB-001: full chain (derive -> Organization -> 3x rowsPath
// fhir.build -> payload.builder fhir_bundle -> fhir_validation strict) builds
// a Bundle with 2 claims, 2 patients, 2 coverages, 1 organization, and passes
// strict FHIR validation with zero unexpected errors.
// ─────────────────────────────────────────────────────────────────────────────

func TestEDI837PFHIRBuilder_MultiClaimMultiLine_BuildsCleanValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": "837P",
			"loops":          edi837PFixtureLoops(),
		},
	}

	// Derive
	result := runScriptSvc(t, "derive_837p_claim_contexts", derive837PClaimContextsScript, data)
	deriveOut := svcStepOutput(t, result, "derive_837p_claim_contexts")
	for k, v := range result {
		data[k] = v
	}
	svcInjectStepOutput(data, "derive_837p_claim_contexts", deriveOut)
	if claimContexts, ok := deriveOut["_claim_contexts"]; ok {
		data["_claim_contexts"] = claimContexts
	}
	if billingProvider, ok := deriveOut["_billing_provider"]; ok {
		data["_billing_provider"] = billingProvider
	}

	claimContexts, _ := data["_claim_contexts"].([]interface{})
	if len(claimContexts) != 2 {
		t.Fatalf("expected 2 claim contexts (subscriber's own + dependent's), got %d: %+v", len(claimContexts), claimContexts)
	}

	// Organization (built once, single-resource mode)
	data = runFHIRBuild(t, "build_organization_fhir", organization837PBuildConfig(), data)

	// Patient / Coverage / Claim (rowsPath mode — one resource per claim context)
	data = runFHIRBuild(t, "build_patient_fhir", patient837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_coverage_fhir", coverage837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_claim_fhir", claim837PBuildConfig(), data)

	// Sanity-check the raw built arrays before bundling, so a failure here
	// points straight at the fhir.build config rather than surfacing only as
	// an opaque downstream validation error.
	message, ok := data["message"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data[\"message\"] to be a map, got %T", data["message"])
	}
	patients, ok := message["fhirPatients"].([]map[string]interface{})
	if !ok || len(patients) != 2 {
		t.Fatalf("expected 2 Patient resources, got %d (ok=%v)", len(patients), ok)
	}
	claims, ok := message["fhirClaims"].([]map[string]interface{})
	if !ok || len(claims) != 2 {
		t.Fatalf("expected 2 Claim resources, got %d (ok=%v)", len(claims), ok)
	}
	depClaim := claims[1]
	items, _ := depClaim["item"].([]interface{})
	if len(items) != 1 {
		t.Errorf("dependent's claim: expected 1 service line item, got %d", len(items))
	}
	subClaim := claims[0]
	subItems, _ := subClaim["item"].([]interface{})
	if len(subItems) != 2 {
		t.Errorf("subscriber's own claim: expected 2 service line items, got %d", len(subItems))
	}
	diags, _ := subClaim["diagnosis"].([]interface{})
	if len(diags) != 2 {
		t.Errorf("subscriber's own claim: expected 2 diagnoses, got %d", len(diags))
	}
	careTeam, _ := subClaim["careTeam"].([]interface{})
	if len(careTeam) != 1 {
		t.Errorf("subscriber's own claim: expected 1 careTeam entry (rendering provider), got %d", len(careTeam))
	}

	// payload.builder fhir_bundle — resourcePaths mixes the single Organization
	// path with 3 array-valued rowsPath outputs in the same list.
	pbExec := payload.NewPayloadBuilderExecutor(nil)
	pbStep := &models.TransformationStep{
		StepName:  "Assemble 837P FHIR Bundle",
		StepAlias: strPtrSvc("assemble_837p_fhir_bundle"),
		StepType:  "payload.builder",
		Enabled:   true,
		Config: map[string]interface{}{
			"mode": "fhir_bundle",
			"fhirBundle": map[string]interface{}{
				"bundleType": "collection",
				"resourcePaths": []interface{}{
					"message.fhirOrganization", "message.fhirPatients", "message.fhirCoverages", "message.fhirClaims",
				},
			},
		},
	}
	pbResult, err := pbExec.Execute(context.Background(), pbStep, data)
	if err != nil {
		t.Fatalf("payload.builder error: %v", err)
	}
	pbOut := svcStepOutput(t, pbResult, "assemble_837p_fhir_bundle")
	for k, v := range pbResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "assemble_837p_fhir_bundle", pbOut)

	bundleJSON, ok := pbOut["payload"].(string)
	if !ok {
		t.Fatalf("assemble_837p_fhir_bundle did not produce a payload string: %+v", pbOut)
	}
	var bundle map[string]interface{}
	if err := json.Unmarshal([]byte(bundleJSON), &bundle); err != nil {
		t.Fatalf("failed to unmarshal bundle JSON: %v", err)
	}
	entries, _ := bundle["entry"].([]interface{})
	// 1 Organization + 2 Patient + 2 Coverage + 2 Claim = 7 entries.
	if len(entries) != 7 {
		t.Errorf("expected 7 Bundle entries (1 Organization + 2 Patient + 2 Coverage + 2 Claim), got %d", len(entries))
	}

	// fhir_validation at strict level.
	vExec := validation.NewFHIRValidationExecutor()
	vStep := &models.TransformationStep{
		StepName:  "Validate 837P FHIR Bundle",
		StepAlias: strPtrSvc("validate_837p_bundle"),
		StepType:  "fhir_validation",
		Enabled:   true,
		Config: map[string]interface{}{
			"profile":          "base",
			"validation_level": "strict",
			"fhir_version":     "R4",
			"source_field":     "steps.assemble_837p_fhir_bundle.step_output.payload",
		},
	}
	vResult, err := vExec.Execute(context.Background(), vStep, data)
	if err != nil {
		t.Fatalf("validate_837p_bundle error: %v", err)
	}
	vOut := svcStepOutput(t, vResult, "validate_837p_bundle")

	errs := asStringSlice(vOut["errors"])
	var unexpected []string
	for _, s := range errs {
		// Same known, out-of-scope, verified-real gap pas_fhir_builder_test.go
		// already documents: schemas/fhir/R4/valuesets/ClaimTypes.gz is
		// compiled with "codes": [] and "fetchFailed": true — any Claim.type
		// code spuriously fails "invalid-code" until that ValueSet is
		// recompiled with real data, unrelated to this mapping.
		if strings.Contains(s, "invalid-code") && strings.Contains(s, "Claim.type") {
			continue
		}
		unexpected = append(unexpected, s)
	}
	if len(unexpected) > 0 {
		b, _ := json.MarshalIndent(bundle, "", "  ")
		t.Errorf("unexpected validation errors beyond the known ClaimTypes ValueSet gap:\n%s\nbundle: %s",
			strings.Join(unexpected, "\n"), b)
	}
}

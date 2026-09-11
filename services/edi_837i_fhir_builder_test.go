// ─────────────────────────────────────────────────────────────────────────────
// EDI X12 837I → FHIR mapping — the config here is transcribed verbatim into
// database/migrations/V238, same "config here is the source of truth"
// discipline as edi_837p_fhir_builder_test.go and pas_fhir_builder_test.go.
//
// Institutional differs from professional in real, structural ways (not just
// a renamed field) — confirmed directly against edi/schemas/x12_005010's own
// loops/2300-claim-institutional.json and segments/{CL1,SV2,HI}.json:
//   - SV2 replaces SV1 (serviceLineRevenueCode + procedureCode, no
//     diagnosisCodePointer — institutional service lines don't carry
//     item-level diagnosis pointers the way professional's SV1 does).
//   - CL1 (Institutional Claim Code — admission type/source, patient status)
//     has no professional equivalent -> Claim.supportingInfo[].
//   - HI is SHARED for diagnosis AND procedure codes on 837I (unlike 837P,
//     where HI only ever carries diagnoses) — distinguished purely by each
//     repetition's own qualifier (BK/ABK/BF/ABF/BJ/ABJ/BN/ABN/PR/APR =
//     diagnosis [ICD-9/ICD-10 pairs], BR/BBR/BQ/BBQ = procedure) ->
//     Claim.diagnosis[] and Claim.procedure[] respectively. HI's own
//     Present-on-Admission indicator (sub09) rides on diagnosis
//     repetitions -> Claim.diagnosis[].onAdmission.
//   - Care team roles differ: 2310A=Attending, 2310B=Operating Physician,
//     2310D=Rendering, 2310F=Referring (837P's 2310A/2310B are Referring/
//     Rendering — genuinely different role assignments, not a relabeling).
//
// Run: go test ./services/ -v -run TestEDI837IFHIRBuilder
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
// Fixture — one billing provider; a subscriber who is her own patient (an
// inpatient claim: 2 service lines, 2 diagnoses [principal + admitting, the
// second carrying Present-on-Admission], 1 principal procedure, attending +
// rendering care team, CL1 admission info) and a second subscriber whose
// dependent is the patient (an outpatient claim: 1 service line, 1 diagnosis,
// no procedure) — proving both branches of the derive script and genuine
// multi-claim/multi-line handling, same shape discipline as the 837P fixture.
// ─────────────────────────────────────────────────────────────────────────────

func edi837IFixtureLoops() map[string]interface{} {
	return map[string]interface{}{
		"2000A": []interface{}{
			map[string]interface{}{
				"HL": map[string]interface{}{"hierarchicalIdNumber": "1", "hierarchicalLevelCode": "20", "hierarchicalChildCode": "1"},
				"loops": map[string]interface{}{
					"2010AA": map[string]interface{}{
						"NM1": []interface{}{
							map[string]interface{}{"entityIdentifierCode": "85", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "GENERAL HOSPITAL", "identificationCodeQualifier": "XX", "identificationCode": "9876543210"},
						},
					},
					"2000B": []interface{}{
						// Subscriber 1: her own patient (self), inpatient claim.
						map[string]interface{}{
							"HL":  map[string]interface{}{"hierarchicalIdNumber": "2", "hierarchicalParentIdNumber": "1", "hierarchicalLevelCode": "22", "hierarchicalChildCode": "0"},
							"SBR": map[string]interface{}{"payerResponsibilitySequenceNumberCode": "P", "individualRelationshipCode": "18"},
							"loops": map[string]interface{}{
								"2010BA": map[string]interface{}{
									"NM1": []interface{}{
										map[string]interface{}{"entityIdentifierCode": "IL", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "WILSON", "nameFirst": "MARY", "identificationCodeQualifier": "MI", "identificationCode": "SUB789"},
									},
									"DMG": map[string]interface{}{"dateTimePeriodFormatQualifier": "D8", "birthDate": "19601005", "genderCode": "F"},
								},
								"2010BB": map[string]interface{}{
									"NM1": []interface{}{
										map[string]interface{}{"entityIdentifierCode": "PR", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "PAYER1", "identificationCodeQualifier": "PI", "identificationCode": "PAYER001"},
									},
								},
								"2300": []interface{}{
									map[string]interface{}{
										"CLM": map[string]interface{}{
											"patientControlNumber": "CLM-INPT-001", "totalClaimChargeAmount": "8200.00",
											"healthCareServiceLocation": map[string]interface{}{"placeOfServiceCode": "21", "facilityCodeQualifier": "B", "claimFrequencyCode": "1"},
										},
										"CL1": map[string]interface{}{"admissionTypeCode": "3", "admissionSourceCode": "1", "patientStatusCode": "01"},
										// 2 separate HI occurrences (diagnosis codes, then the procedure code) --
										// the real-world shape HI.json's own maxUse=">1" correction models;
										// proves allHICodes' own cross-instance flattening, not just a
										// single-instance array wrapper.
										"HI": []interface{}{
											map[string]interface{}{
												"codes": []interface{}{
													map[string]interface{}{"code": map[string]interface{}{"qualifier": "ABK", "code": "I21.4"}},
													map[string]interface{}{"code": map[string]interface{}{"qualifier": "ABF", "code": "I10", "presentOnAdmissionIndicator": "Y"}},
												},
											},
											map[string]interface{}{
												"codes": []interface{}{
													map[string]interface{}{"code": map[string]interface{}{"qualifier": "BBR", "code": "0270"}},
												},
											},
										},
										"loops": map[string]interface{}{
											"2310A": map[string]interface{}{
												"NM1": []interface{}{
													map[string]interface{}{"entityIdentifierCode": "71", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "CARDLIN", "nameFirst": "PAUL", "identificationCodeQualifier": "XX", "identificationCode": "2223334440"},
												},
											},
											"2310D": map[string]interface{}{
												"NM1": []interface{}{
													map[string]interface{}{"entityIdentifierCode": "82", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "RENFRO", "nameFirst": "SUE", "identificationCodeQualifier": "XX", "identificationCode": "3334445550"},
												},
											},
											"2400": []interface{}{
												map[string]interface{}{
													"LX": map[string]interface{}{"assignedNumber": "1"},
													"SV2": map[string]interface{}{
														"serviceLineRevenueCode": "0250",
														"procedureCode":          map[string]interface{}{"qualifier": "HC", "code": "99223"},
														"lineItemChargeAmount": "5000.00", "unitOfMeasurementCode": "UN", "serviceUnitCount": "1",
													},
													"DTP": []interface{}{
														map[string]interface{}{"dateTimeQualifier": "472", "dateTimePeriodFormatQualifier": "D8", "datePeriod": "20260210"},
													},
												},
												map[string]interface{}{
													"LX": map[string]interface{}{"assignedNumber": "2"},
													"SV2": map[string]interface{}{
														"serviceLineRevenueCode": "0270",
														"procedureCode":          map[string]interface{}{"qualifier": "HC", "code": "99232"},
														"lineItemChargeAmount": "3200.00", "unitOfMeasurementCode": "UN", "serviceUnitCount": "1",
													},
													"DTP": []interface{}{
														map[string]interface{}{"dateTimeQualifier": "472", "dateTimePeriodFormatQualifier": "D8", "datePeriod": "20260211"},
													},
												},
											},
										},
									},
								},
							},
						},
						// Subscriber 2: parent of a dependent — outpatient claim.
						map[string]interface{}{
							"HL":  map[string]interface{}{"hierarchicalIdNumber": "3", "hierarchicalParentIdNumber": "1", "hierarchicalLevelCode": "22", "hierarchicalChildCode": "1"},
							"SBR": map[string]interface{}{"payerResponsibilitySequenceNumberCode": "P", "individualRelationshipCode": "19"},
							"loops": map[string]interface{}{
								"2010BA": map[string]interface{}{
									"NM1": []interface{}{
										map[string]interface{}{"entityIdentifierCode": "IL", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "WILSON", "nameFirst": "TOM", "identificationCodeQualifier": "MI", "identificationCode": "SUB790"},
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
													map[string]interface{}{"entityIdentifierCode": "QC", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "WILSON", "nameFirst": "AMY", "identificationCodeQualifier": "MI", "identificationCode": "DEP321"},
												},
												"DMG": map[string]interface{}{"dateTimePeriodFormatQualifier": "D8", "birthDate": "20100415", "genderCode": "F"},
											},
											"2300": []interface{}{
												map[string]interface{}{
													"CLM": map[string]interface{}{
														"patientControlNumber": "CLM-OUTPT-001", "totalClaimChargeAmount": "450.00",
														"healthCareServiceLocation": map[string]interface{}{"placeOfServiceCode": "22", "facilityCodeQualifier": "B", "claimFrequencyCode": "1"},
													},
													"CL1": map[string]interface{}{"admissionTypeCode": "9", "admissionSourceCode": "9", "patientStatusCode": "01"},
													"HI": []interface{}{
														map[string]interface{}{
															"codes": []interface{}{
																map[string]interface{}{"code": map[string]interface{}{"qualifier": "ABK", "code": "S52.501A"}},
															},
														},
													},
													"loops": map[string]interface{}{
														"2400": []interface{}{
															map[string]interface{}{
																"LX": map[string]interface{}{"assignedNumber": "1"},
																"SV2": map[string]interface{}{
																	"serviceLineRevenueCode": "0450",
																	"procedureCode":          map[string]interface{}{"qualifier": "HC", "code": "99283"},
																	"lineItemChargeAmount": "450.00", "unitOfMeasurementCode": "UN", "serviceUnitCount": "1",
																},
																"DTP": []interface{}{
																	map[string]interface{}{"dateTimeQualifier": "472", "dateTimePeriodFormatQualifier": "D8", "datePeriod": "20260215"},
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
// Derive 837I Claim Contexts — same enrichment.script scope discipline as
// 837P's own derive script; differs in reading SV2/CL1 and splitting HI's
// shared diagnosis/procedure qualifiers.
// ─────────────────────────────────────────────────────────────────────────────

const derive837IClaimContextsScript = `
// -- Derive 837I Claim Contexts --
var parsed = input.parsedEDI || {};
var loops = parsed.loops || {};

if (parsed.transactionSet !== "837I") {
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

// HI is SHARED for diagnosis and procedure codes on 837I, distinguished
// purely by each repetition's own qualifier. Diagnosis qualifiers come in
// ICD-9/ICD-10 pairs (BK/ABK = principal, BF/ABF = other, BJ/ABJ =
// admitting, BN/ABN = external cause of injury, PR/APR = patient's reason
// for visit) -- an X12.org real-world sample (the "Jones Hospital" 005010X223
// example, ICD-9-CM era) caught a real gap here: an earlier version of
// extractDiagnoses only matched the "AB"-prefixed ICD-10 forms, silently
// dropping every ICD-9 diagnosis (BK/BF/etc). Procedure qualifiers are
// BR/BBR (principal) and BQ/BBQ (other). Value/occurrence/condition codes
// (BE/BH/BG) and DRG (DR) are not modeled in this pass (named
// simplification, matching 835's own "core fields, not exhaustive"
// precedent) -- excluded automatically since they're not in either
// whitelist below.
var DIAGNOSIS_HI_QUALIFIERS = ["BK", "ABK", "BF", "ABF", "BJ", "ABJ", "BN", "ABN", "PR", "APR"];
var PROCEDURE_HI_QUALIFIERS = ["BR", "BBR", "BQ", "BBQ"];
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
    var q = c.qualifier || "";
    if (DIAGNOSIS_HI_QUALIFIERS.indexOf(q) === -1) continue;
    var entry = { code: c.code, sequence: out.length + 1 };
    // Omit the key entirely (not an empty string) when absent -- an empty
    // onAdmission.coding[0].code sourcePath still resolves as "not empty
    // data" for a LITERAL system field with no sourcePath of its own, so the
    // Claim config's own condition (checking this field "exists") only works
    // correctly when the key is truly missing from the row, not blank.
    if (c.presentOnAdmissionIndicator) { entry.onAdmission = c.presentOnAdmissionIndicator; }
    out.push(entry);
  }
  return out;
}

function extractProcedures(codes) {
  var out = [];
  for (var i = 0; i < codes.length; i++) {
    var c = (codes[i] && codes[i].code) || {};
    if (!c.code) continue;
    var q = c.qualifier || "";
    if (PROCEDURE_HI_QUALIFIERS.indexOf(q) === -1) continue;
    out.push({ code: c.code, sequence: out.length + 1 });
  }
  return out;
}

function extractInstitutionalInfo(cl1) {
  var out = [];
  var seq = 0;
  if (!cl1) return out;
  if (cl1.admissionTypeCode) { seq++; out.push({ category: "admissiontype", code: cl1.admissionTypeCode, sequence: seq }); }
  if (cl1.admissionSourceCode) { seq++; out.push({ category: "admissionsource", code: cl1.admissionSourceCode, sequence: seq }); }
  if (cl1.patientStatusCode) { seq++; out.push({ category: "patientstatus", code: cl1.patientStatusCode, sequence: seq }); }
  return out;
}

function extractServiceLines(claimLoops) {
  var lines = arr((claimLoops || {})["2400"]);
  var out = [];
  for (var i = 0; i < lines.length; i++) {
    var line = lines[i];
    var lx = line.LX || {};
    var sv2 = line.SV2 || {};
    var proc = sv2.procedureCode || {};
    var dtpList = arr(line.DTP);
    var svc = resolveServicedDate(dtpList);
    var entry = {
      sequence: parseInt(lx.assignedNumber || String(i + 1), 10),
      revenueCode: sv2.serviceLineRevenueCode || "",
      quantity: sv2.serviceUnitCount ? Number(sv2.serviceUnitCount) : 1,
      net: sv2.lineItemChargeAmount ? Number(sv2.lineItemChargeAmount) : 0,
      servicedDate: svc.servicedDate,
      servicedStart: svc.servicedStart,
      servicedEnd: svc.servicedEnd
    };
    // Claim.item.productOrService is ALWAYS anchored on the revenue code
    // (Claim config's own productOrService.coding[0], built directly off
    // revenueCode) -- the one identifier EVERY real institutional line
    // carries, and the field an official X12.org example (005010X223A2
    // Example 1a) shows populated on every line even when a procedure code
    // is ALSO present. SV202 (procedure code) is genuinely OPTIONAL on real
    // institutional claims -- present on many ancillary/outpatient lines
    // (often required under CMS OPPS rules for specific revenue codes),
    // absent on others (room & board, some inpatient DRG-based lines; a
    // real, unedited sample checked this round had NO procedure code on any
    // of its 9 lines). When present, it's added as a SECOND coding in the
    // SAME productOrService CodeableConcept (Claim config's own
    // productOrService.coding[1]) -- never a replacement for the revenue-
    // code coding. Keys omitted entirely (not set to "") when absent,
    // matching the same "a condition checking a field 'exists' must see a
    // truly missing key, not an empty string" convention onAdmission
    // already established.
    if (proc.code) {
      entry.procedureCode = proc.code;
      entry.procedureSystem = (proc.qualifier === "HC") ? "http://www.ama-assn.org/go/cpt" : "";
    }
    out.push(entry);
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

// Institutional care-team roles are genuinely different assignments from
// professional's own 2310A/2310B (Referring/Rendering) -- 2310A=Attending,
// 2310B=Operating Physician, 2310D=Rendering, 2310F=Referring.
function extractCareTeam(claimLoops) {
  var team = [];
  var roleLoops = [
    { key: "2310A", role: "attending" },
    { key: "2310B", role: "operating" },
    { key: "2310D", role: "rendering" },
    { key: "2310F", role: "referring" }
  ];
  for (var r = 0; r < roleLoops.length; r++) {
    var loop = (claimLoops || {})[roleLoops[r].key];
    if (!loop) continue;
    var nm1 = first(loop.NM1);
    if (nm1.identificationCode) {
      team.push({ npi: nm1.identificationCode, role: roleLoops[r].role, sequence: team.length + 1 });
    }
  }
  return team;
}

// Claim.facility (base FHIR, 0..1 Reference) -- 2310E Service Facility
// Location on 837I (a genuinely different loop position from 837P's own
// 2310C, since institutional's role numbering differs -- see this file's own
// header comment). Same logical (identifier-only) reference pattern as
// careTeam[].provider; not a care-team member. See
// edi_837p_fhir_builder_test.go's own extractFacility for the full rationale
// (found via X12.org's official COB example on the professional side).
function extractFacility(claimLoops) {
  var loop = (claimLoops || {})["2310E"];
  if (!loop) return {};
  var nm1 = first(loop.NM1);
  if (!nm1.identificationCode) return {};
  return { facilityNpi: nm1.identificationCode, facilityName: nm1.nameLastOrOrganizationName || "" };
}

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
    procedureList: extractProcedures(allHICodes(claimLoop.HI)),
    institutionalInfo: extractInstitutionalInfo(claimLoop.CL1),
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
      // See edi_837p_fhir_builder_test.go's own dependent-relationship comment
      // -- same real-world PAT01-vs-SBR02 correction applies here (the 2000C
      // loop and PAT segment are identical shape across 837P/837I).
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
// fhir.build configs — transcribed verbatim into V238's SQL. Organization/
// Patient/Coverage are IDENTICAL in shape to 837P's own configs (same
// underlying row structure: patientInfo/subscriberInfo/payerInfo/claim) —
// only Claim differs (type code, revenue on item, onAdmission on diagnosis,
// procedure[] and supportingInfo[] repeatingGroups).
// ─────────────────────────────────────────────────────────────────────────────

func organization837IBuildConfig() map[string]interface{} {
	return organization837PBuildConfig()
}

func patient837IBuildConfig() map[string]interface{} {
	return patient837PBuildConfig()
}

func coverage837IBuildConfig() map[string]interface{} {
	return coverage837PBuildConfig()
}

func claim837IBuildConfig() map[string]interface{} {
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
			map[string]interface{}{"targetPath": "type.coding[0].code", "literalValue": "institutional"},
			map[string]interface{}{"targetPath": "use", "literalValue": "claim"},
			map[string]interface{}{"targetPath": "created", "sourcePath": "claim.createdAt"},
			map[string]interface{}{"targetPath": "priority.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/processpriority"},
			map[string]interface{}{"targetPath": "priority.coding[0].code", "literalValue": "normal"},
			map[string]interface{}{"targetPath": "patient.reference", "sourcePath": "patientInfo.memberId", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"}},
			map[string]interface{}{"targetPath": "provider.reference", "literalValue": "Organization/organization-billing"},
			map[string]interface{}{"targetPath": "insurer.identifier.system", "literalValue": "http://ezhealthkonnect.local/x12-payer-id"},
			map[string]interface{}{"targetPath": "insurer.identifier.value", "sourcePath": "payerInfo.payerId"},
			// Claim.facility -- see edi_837p_fhir_builder_test.go's own note; here
			// sourced from 2310E (institutional's own Service Facility Location
			// position, distinct from 837P's 2310C).
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
					map[string]interface{}{
						"targetPath": "onAdmission.coding[0].system", "literalValue": "https://codesystem.x12.org/005010/1352",
						"condition": map[string]interface{}{"field": "onAdmission", "operator": "exists"},
					},
					map[string]interface{}{"targetPath": "onAdmission.coding[0].code", "sourcePath": "onAdmission"},
				},
			},
			map[string]interface{}{
				"targetPath": "procedure",
				"rowsPath":   "claim.procedureList",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "procedureCodeableConcept.coding[0].system", "literalValue": "http://www.cms.gov/Medicare/Coding/ICD10"},
					map[string]interface{}{"targetPath": "procedureCodeableConcept.coding[0].code", "sourcePath": "code"},
					map[string]interface{}{"targetPath": "sequence", "sourcePath": "sequence"},
				},
			},
			map[string]interface{}{
				"targetPath": "supportingInfo",
				"rowsPath":   "claim.institutionalInfo",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "sequence", "sourcePath": "sequence"},
					map[string]interface{}{"targetPath": "category.coding[0].code", "sourcePath": "category"},
					map[string]interface{}{"targetPath": "code.coding[0].code", "sourcePath": "code"},
				},
			},
			map[string]interface{}{
				"targetPath": "item",
				"rowsPath":   "claim.serviceLines",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "sequence", "sourcePath": "sequence"},
					map[string]interface{}{"targetPath": "revenue.coding[0].system", "literalValue": "https://codesystem.x12.org/005010/234"},
					map[string]interface{}{"targetPath": "revenue.coding[0].code", "sourcePath": "revenueCode"},
					// productOrService is ALWAYS anchored on the revenue code (coding[0])
					// -- the one identifier every real institutional line carries -- with
					// the HCPCS/CPT code (when SV202 is actually present) added as a
					// SECOND coding (coding[1]), never as a replacement. Both coding[1]
					// fields use sourcePath (not literalValue), so they naturally resolve
					// to nothing and get skipped when procedureCode/procedureSystem are
					// absent from the row -- no explicit condition needed.
					map[string]interface{}{"targetPath": "productOrService.coding[0].system", "literalValue": "https://codesystem.x12.org/005010/234"},
					map[string]interface{}{"targetPath": "productOrService.coding[0].code", "sourcePath": "revenueCode"},
					map[string]interface{}{"targetPath": "productOrService.coding[1].system", "sourcePath": "procedureSystem"},
					map[string]interface{}{"targetPath": "productOrService.coding[1].code", "sourcePath": "procedureCode"},
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
// TC-EDI837I-FB-001: full chain builds a Bundle with 2 claims (inpatient +
// outpatient), asserts institutional-specific shapes (revenue codes,
// onAdmission, procedure[], supportingInfo[], attending/operating/rendering
// care team roles), and passes strict FHIR validation with zero unexpected
// errors.
// ─────────────────────────────────────────────────────────────────────────────

func TestEDI837IFHIRBuilder_MultiClaimMultiLine_BuildsCleanValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": "837I",
			"loops":          edi837IFixtureLoops(),
		},
	}

	result := runScriptSvc(t, "derive_837i_claim_contexts", derive837IClaimContextsScript, data)
	deriveOut := svcStepOutput(t, result, "derive_837i_claim_contexts")
	for k, v := range result {
		data[k] = v
	}
	svcInjectStepOutput(data, "derive_837i_claim_contexts", deriveOut)
	if claimContexts, ok := deriveOut["_claim_contexts"]; ok {
		data["_claim_contexts"] = claimContexts
	}
	if billingProvider, ok := deriveOut["_billing_provider"]; ok {
		data["_billing_provider"] = billingProvider
	}

	claimContexts, _ := data["_claim_contexts"].([]interface{})
	if len(claimContexts) != 2 {
		t.Fatalf("expected 2 claim contexts (subscriber's own inpatient + dependent's outpatient), got %d: %+v", len(claimContexts), claimContexts)
	}

	data = runFHIRBuild(t, "build_organization_fhir", organization837IBuildConfig(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patient837IBuildConfig(), data)
	data = runFHIRBuild(t, "build_coverage_fhir", coverage837IBuildConfig(), data)
	data = runFHIRBuild(t, "build_claim_fhir", claim837IBuildConfig(), data)

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

	inptClaim := claims[0]
	items, _ := inptClaim["item"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("inpatient claim: expected 2 service line items, got %d", len(items))
	}
	firstItem := items[0].(map[string]interface{})
	revenue, ok := firstItem["revenue"].(map[string]interface{})
	if !ok {
		t.Fatalf("inpatient claim item[0]: expected a revenue CodeableConcept, got %T", firstItem["revenue"])
	}
	revCoding := revenue["coding"].([]interface{})[0].(map[string]interface{})
	if revCoding["code"] != "0250" {
		t.Errorf("inpatient claim item[0].revenue.coding[0].code = %v, want 0250", revCoding["code"])
	}

	diags, _ := inptClaim["diagnosis"].([]interface{})
	if len(diags) != 2 {
		t.Fatalf("inpatient claim: expected 2 diagnoses, got %d", len(diags))
	}
	secondDiag := diags[1].(map[string]interface{})
	onAdmission, ok := secondDiag["onAdmission"].(map[string]interface{})
	if !ok {
		t.Fatalf("inpatient claim diagnosis[1]: expected onAdmission CodeableConcept, got %T", secondDiag["onAdmission"])
	}
	onAdmCoding := onAdmission["coding"].([]interface{})[0].(map[string]interface{})
	if onAdmCoding["code"] != "Y" {
		t.Errorf("inpatient claim diagnosis[1].onAdmission.coding[0].code = %v, want Y", onAdmCoding["code"])
	}
	firstDiag := diags[0].(map[string]interface{})
	if _, hasOnAdmission := firstDiag["onAdmission"]; hasOnAdmission {
		t.Errorf("inpatient claim diagnosis[0]: expected no onAdmission (X12 sample had none), got %v", firstDiag["onAdmission"])
	}

	procs, _ := inptClaim["procedure"].([]interface{})
	if len(procs) != 1 {
		t.Fatalf("inpatient claim: expected 1 procedure, got %d", len(procs))
	}
	procCoding := procs[0].(map[string]interface{})["procedureCodeableConcept"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})
	if procCoding["code"] != "0270" {
		t.Errorf("inpatient claim procedure[0] code = %v, want 0270", procCoding["code"])
	}

	supportingInfo, _ := inptClaim["supportingInfo"].([]interface{})
	if len(supportingInfo) != 3 {
		t.Fatalf("inpatient claim: expected 3 supportingInfo entries (admission type/source, patient status), got %d", len(supportingInfo))
	}

	careTeam, _ := inptClaim["careTeam"].([]interface{})
	if len(careTeam) != 2 {
		t.Fatalf("inpatient claim: expected 2 careTeam entries (attending + rendering), got %d", len(careTeam))
	}
	firstRole := careTeam[0].(map[string]interface{})["role"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})["code"]
	if firstRole != "attending" {
		t.Errorf("inpatient claim careTeam[0].role = %v, want attending", firstRole)
	}

	outptClaim := claims[1]
	outptItems, _ := outptClaim["item"].([]interface{})
	if len(outptItems) != 1 {
		t.Errorf("outpatient claim: expected 1 service line item, got %d", len(outptItems))
	}
	outptProcs, _ := outptClaim["procedure"].([]interface{})
	if len(outptProcs) != 0 {
		t.Errorf("outpatient claim: expected 0 procedures (sample has none), got %d", len(outptProcs))
	}

	// payload.builder fhir_bundle
	pbExec := payload.NewPayloadBuilderExecutor(nil)
	pbStep := &models.TransformationStep{
		StepName:  "Assemble 837I FHIR Bundle",
		StepAlias: strPtrSvc("assemble_837i_fhir_bundle"),
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
	pbOut := svcStepOutput(t, pbResult, "assemble_837i_fhir_bundle")
	for k, v := range pbResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "assemble_837i_fhir_bundle", pbOut)

	bundleJSON, ok := pbOut["payload"].(string)
	if !ok {
		t.Fatalf("assemble_837i_fhir_bundle did not produce a payload string: %+v", pbOut)
	}
	var bundle map[string]interface{}
	if err := json.Unmarshal([]byte(bundleJSON), &bundle); err != nil {
		t.Fatalf("failed to unmarshal bundle JSON: %v", err)
	}
	entries, _ := bundle["entry"].([]interface{})
	if len(entries) != 7 {
		t.Errorf("expected 7 Bundle entries (1 Organization + 2 Patient + 2 Coverage + 2 Claim), got %d", len(entries))
	}

	vExec := validation.NewFHIRValidationExecutor()
	vStep := &models.TransformationStep{
		StepName:  "Validate 837I FHIR Bundle",
		StepAlias: strPtrSvc("validate_837i_bundle"),
		StepType:  "fhir_validation",
		Enabled:   true,
		Config: map[string]interface{}{
			"profile":          "base",
			"validation_level": "strict",
			"fhir_version":     "R4",
			"source_field":     "steps.assemble_837i_fhir_bundle.step_output.payload",
		},
	}
	vResult, err := vExec.Execute(context.Background(), vStep, data)
	if err != nil {
		t.Fatalf("validate_837i_bundle error: %v", err)
	}
	vOut := svcStepOutput(t, vResult, "validate_837i_bundle")

	errs := asStringSlice(vOut["errors"])
	var unexpected []string
	for _, s := range errs {
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

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
	"fmt"
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
// A prior step's plain top-level output field (edi.parse's own "parsedEDI",
// per its outputField config) is NOT exposed to a later step's own "input"
// at the top level -- the real pipeline engine nests it under
// input.message.parsedEDI instead (confirmed by direct instrumentation
// against a real Test Pipeline run; this project's own prior finding
// documents the same "message." prefix requirement for fhir.build/hl7.build
// sourcePath strings, and it turns out to apply here too). Prefer that real
// shape; fall back to the bare top-level key for Go-level test harnesses
// that construct { parsedEDI: ... } directly without a "message" wrapper
// (as every *_fhir_builder_test.go and edi_837_real_parser_roundtrip_test.go
// fixture in this codebase already does).
var parsed = (input.message && input.message.parsedEDI) || input.parsedEDI || {};
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
  return ({ _claim_contexts: [], _coverage_contexts: [], _billing_providers: [] });
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

// X12 837 allows MULTIPLE 2000A (Billing Provider) hierarchical levels in one
// file -- e.g. a clearinghouse batching claims from several billing practices
// into one interchange. Every real sample checked into this repo only ever
// carries one, but the spec allows more, so every one is walked rather than
// assumed away (the pre-fix version of this script used first(loops["2000A"])
// and silently attributed every claim in the file to whichever provider
// happened to be listed first). id is NPI-anchored (the one identifier every
// real billing provider carries) with a positional fallback only for the rare
// NPI-less case -- flexible, not rigid, matching this script's own PAT01/SBR02
// fallback convention elsewhere.
function billingProviderId(npi, idx) {
  return "organization-billing-" + (npi || String(idx + 1));
}
var billingProviderLevels = arr(loops["2000A"]);
var billingProviders = [];
for (var bpIdx = 0; bpIdx < billingProviderLevels.length; bpIdx++) {
  var bpProviderLoop = (billingProviderLevels[bpIdx].loops || {})["2010AA"] || {};
  var bpNM1 = first(bpProviderLoop.NM1);
  var bpNpi = bpNM1.identificationCode || "";
  billingProviders.push({
    id: billingProviderId(bpNpi, bpIdx),
    npi: bpNpi,
    name: bpNM1.nameLastOrOrganizationName || ""
  });
}

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
// NOTE: 837P's own HI never carries a Present-on-Admission indicator (that's
// an institutional-only concept, see 837I's own extractDiagnoses), so no
// on_admission key is ever added here -- unlike 837I, which does.

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
      procedure_code: proc.code || "",
      procedure_system: (proc.qualifier === "HC") ? "http://www.ama-assn.org/go/cpt" : "",
      quantity: sv1.serviceUnitCount ? Number(sv1.serviceUnitCount) : 1,
      net: sv1.lineItemChargeAmount ? Number(sv1.lineItemChargeAmount) : 0,
      serviced_date: svc.servicedDate,
      serviced_start: svc.servicedStart,
      serviced_end: svc.servicedEnd,
      diagnosis_pointers: diagnosisPointers(sv1)
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
// Provider identification prefers NM1's own NPI (identificationCodeQualifier
// "XX"), but real, spec-compliant claims sometimes carry NO NM1 identifier at
// all for a pre-NPI-era provider, identifying them via a SEPARATE REF segment
// instead (e.g. REF*1G = Provider UPIN Number) -- confirmed directly against
// X12.org's own official "Jones Hospital" 837I example (same shape applies to
// 837P's own 2310A/B/D). Previously this NM1-only check silently produced an
// EMPTY care team for exactly this real, non-fabricated case. The REF
// qualifier itself (1G/0B/G2/...) is carried through as part of the
// identifier system URI rather than assumed to always be UPIN, since any of
// several secondary qualifiers are possible here per the base X12 REF01 code
// list.
function providerIdentifier(entity) {
  if (entity.identificationCode) {
    return { identifier_system: "http://hl7.org/fhir/sid/us-npi", identifier_value: entity.identificationCode };
  }
  var ref = first(entity.REF);
  if (ref.referenceIdentification) {
    return {
      identifier_system: "http://ezhealthkonnect.local/x12-ref-qualifier/" + (ref.referenceIdentificationQualifier || "unknown"),
      identifier_value: ref.referenceIdentification
    };
  }
  return null;
}

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
      var ident = providerIdentifier({ identificationCode: nm1.identificationCode, REF: list[i].REF });
      if (ident) {
        team.push({ identifier_system: ident.identifier_system, identifier_value: ident.identifier_value, role: roleLoops[r].role, sequence: team.length + 1 });
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
  return { facility_npi: nm1.identificationCode, facility_name: nm1.nameLastOrOrganizationName || "" };
}

// X12 837's claim-level loop 2320 (Other Subscriber Information, repeat up to
// 10) carries additional payers beyond the primary (2010BB) for Coordination
// of Benefits -- the actual other payer's identity lives on the nested 2330B
// (Other Payer Name) sub-loop's own NM1. 2330A (Other Subscriber Name) and
// 2330C-G (the other payer's own referring/rendering/service-facility/
// supervising/billing providers) are not modeled in this pass -- named
// simplification, matching this codebase's own "core fields, not exhaustive"
// precedent. A row with no real payer identifier is skipped, same convention
// extractCareTeam already uses. Sequence/priority is taken from encountered
// order (2320 occurrences are, in practice, always listed in priority order
// by real trading partners), not SBR01's own priority code -- reading that
// too would add complexity for a case this codebase's real samples never
// exercise.
function extractOtherPayers(claimLoops) {
  var out = [];
  var list = arr((claimLoops || {})["2320"]);
  for (var i = 0; i < list.length; i++) {
    var payerNM1 = first(((list[i].loops || {})["2330B"] || {}).NM1);
    if (!payerNM1.identificationCode) continue;
    out.push({ payer_id: payerNM1.identificationCode, payer_name: payerNM1.nameLastOrOrganizationName || "" });
  }
  return out;
}

// Builds Claim.insurance[] rows AND the flattened per-payer Coverage-resource
// rows from the SAME id formula in one pass, so the two stay correlated by
// construction instead of via two independently-maintained computations
// (mirroring this codebase's own established "same derived/fixed id on both
// sides" pattern, e.g. the Da Vinci PAS template's Organization/Practitioner
// correlation). The primary payer's own coverage_id ("coverage-" +
// patientControlNumber) is UNCHANGED from this script's pre-COB shape -- only
// secondary/tertiary payers get a "-2"/"-3"/... suffix -- so an existing
// single-payer claim keeps producing the exact same Coverage id it always has.
function buildInsuranceAndCoverage(payerInfo, patientInfo, subscriberInfo, claimLoops, pcn) {
  var baseId = "coverage-" + pcn;
  var insuranceList = [{ sequence: 1, focal: true, coverage_id: baseId }];
  var coverageRows = [{
    patient_info: patientInfo, subscriber_info: subscriberInfo, payer_info: payerInfo,
    coverage_id: baseId, sequence: 1, focal: true
  }];
  var others = extractOtherPayers(claimLoops);
  for (var i = 0; i < others.length; i++) {
    var seq = i + 2;
    var cid = baseId + "-" + seq;
    insuranceList.push({ sequence: seq, focal: false, coverage_id: cid });
    coverageRows.push({
      patient_info: patientInfo, subscriber_info: subscriberInfo,
      payer_info: { name: others[i].payer_name, payer_id: others[i].payer_id },
      coverage_id: cid, sequence: seq, focal: false
    });
  }
  return { insurance_list: insuranceList, coverage_rows: coverageRows };
}

// Claim.created is required by the base FHIR R4 Claim resource but has no
// X12 837 equivalent (837 carries no "claim record created" timestamp) --
// the same "record processing time, not sourced from the message" default
// convention BHT03/BHT04 (creation date/time) already establish for the
// interchange itself. Computed once for the whole file, not per claim.
var nowISO = new Date().toISOString();

// NOTE: the returned object's own property names are snake_case throughout
// (patient_info, total_charge_amount, etc.), NOT the camelCase a reader might
// expect from the rest of this script's own internal variable names. This is
// REQUIRED, not a style choice: a later fhir.build step can only reach this
// enrichment.script's own return value via the "steps.<alias>.step_output.<key>"
// reference path (BaseExecutor.SetStepOutputWithDetails stashes it in
// inputData["_stepOutput"], which executeStepWithContext extracts into that
// per-step snapshot and then deletes from what actually flows forward as
// plain inputData -- it never reaches a later step at the top level or under
// "message"). That snapshot is ALWAYS passed through
// models.OutputNormalizer.NormalizeStepOutput first, which snake_cases every
// key it doesn't already recognize as snake_case -- so a later step's own
// sourcePath/rowsPath config strings (see claim837PBuildConfig et al.) MUST
// address these fields by their post-normalization names. Matching that
// reality here, instead of writing camelCase and forcing every config
// sourcePath to reference a silently-renamed key, is what keeps the script
// and the config it feeds honest about what data shape actually flows
// between them. Found only by a real browser Test-Pipeline run -- the
// Go-level tests' own svcInjectStepOutput helper happens to preserve
// whatever casing the script returns unchanged, so a camelCase/snake_case
// mismatch here was invisible there.
function buildClaimContext(patientInfo, subscriberInfo, payerInfo, claimLoop, billingProviderId) {
  var clm = claimLoop.CLM || {};
  var svcLoc = clm.healthCareServiceLocation || {};
  var facility = extractFacility(claimLoop.loops);
  var pcn = clm.patientControlNumber || "";
  var insCov = buildInsuranceAndCoverage(payerInfo, patientInfo, subscriberInfo, claimLoop.loops, pcn);
  var claim = {
    patient_control_number: pcn,
    total_charge_amount: clm.totalClaimChargeAmount ? Number(clm.totalClaimChargeAmount) : 0,
    place_of_service_code: svcLoc.placeOfServiceCode || "",
    created_at: nowISO,
    billing_provider_id: billingProviderId,
    diagnosis_list: extractDiagnoses(allHICodes(claimLoop.HI)),
    service_lines: extractServiceLines(claimLoop.loops),
    care_team: extractCareTeam(claimLoop.loops),
    insurance_list: insCov.insurance_list
  };
  if (facility.facility_npi) {
    claim.facility_npi = facility.facility_npi;
    claim.facility_name = facility.facility_name;
  }
  return {
    context: {
      patient_info: patientInfo,
      subscriber_info: subscriberInfo,
      payer_info: payerInfo,
      claim: claim
    },
    coverage_rows: insCov.coverage_rows
  };
}

function personFromNM1AndDMG(nm1, dmg, relationshipCode) {
  var gender = dmg.genderCode || "";
  return {
    first_name: nm1.nameFirst || "",
    last_name: nm1.nameLastOrOrganizationName || "",
    member_id: nm1.identificationCode || "",
    dob: dmg.birthDate || "",
    gender_fhir: genderToFHIR(gender),
    relationship_code: relationshipCode || "",
    relationship_fhir: relationshipToFHIR(relationshipCode || "")
  };
}

var claimContexts = [];
var coverageContexts = [];

function pushClaimContext(built) {
  claimContexts.push(built.context);
  for (var cr = 0; cr < built.coverage_rows.length; cr++) coverageContexts.push(built.coverage_rows[cr]);
}

for (var bp = 0; bp < billingProviderLevels.length; bp++) {
  var bpId = billingProviders[bp].id;
  var billingLoops = billingProviderLevels[bp].loops || {};
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
      payer_id: payerNM1.identificationCode || ""
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
        if (!patientInfo.member_id) patientInfo.member_id = subscriberInfo.member_id + "-DEP" + (d + 1);

        var depClaims = arr(depLoops["2300"]);
        for (var dc = 0; dc < depClaims.length; dc++) {
          pushClaimContext(buildClaimContext(patientInfo, subscriberInfo, payerInfo, depClaims[dc], bpId));
        }
      }
    } else {
      var subClaims = arr(subLoops["2300"]);
      for (var sc = 0; sc < subClaims.length; sc++) {
        pushClaimContext(buildClaimContext(subscriberInfo, subscriberInfo, payerInfo, subClaims[sc], bpId));
      }
    }
  }
}

return ({
  _claim_contexts: claimContexts,
  _coverage_contexts: coverageContexts,
  _billing_providers: billingProviders
});
`

// ─────────────────────────────────────────────────────────────────────────────
// fhir.build configs — transcribed verbatim into V237's SQL.
// ─────────────────────────────────────────────────────────────────────────────

// The derive script's own OWN return value (_billing_provider, _claim_contexts)
// is NEVER merged onto the plain top-level inputData a later step's fhir.build
// sourcePath/rowsPath resolves against -- BaseExecutor.SetStepOutputWithDetails
// writes it ONLY into inputData["_stepOutput"], and executeStepWithContext
// (services/transformation_pipeline_helpers.go) extracts that into the
// per-step "steps.<alias>.step_output" snapshot for display/cross-step
// referencing, then DELETES "_stepOutput" from the data that actually flows
// to the next step. So a later step must reference it via the same
// "steps.<alias>.step_output.<key>" path other parts of this codebase already
// use for cross-step references (e.g. fhir_validation's own "source_field":
// "steps.<alias>.step_output.payload" pattern) -- a bare top-level
// "_claim_contexts" (what this used before) or a naive "message._claim_contexts"
// prefix (what edi.parse's own OWN plain-assigned outputField needs instead,
// a DIFFERENT mechanism -- see derive837PClaimContextsScript's own "input.message.parsedEDI"
// comment) both silently resolve to nothing against a REAL pipeline run.
// Found only by a real browser Test-Pipeline run (not the Go-level tests,
// whose own svcInjectStepOutput test helper happens to ALSO populate this
// exact "steps.<alias>.step_output" shape, so this bug was invisible there).
const derive837PStepAlias = "derive_837p_claim_contexts"

func organization837PBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Organization",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirOrganizations",
		"rowsPath":     "steps." + derive837PStepAlias + ".step_output._billing_providers",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "id"},
			map[string]interface{}{"targetPath": "active", "literalValue": "true"},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": "npi"},
			map[string]interface{}{"targetPath": "name", "sourcePath": "name"},
		},
	}
}

func patient837PBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Patient",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPatients",
		"rowsPath":     "steps." + derive837PStepAlias + ".step_output._claim_contexts",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "patient_info.member_id"},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/x12-member-id"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": "patient_info.member_id"},
			map[string]interface{}{"targetPath": "name[0].family", "sourcePath": "patient_info.last_name"},
			map[string]interface{}{"targetPath": "name[0].given[0]", "sourcePath": "patient_info.first_name"},
			map[string]interface{}{"targetPath": "birthDate", "sourcePath": "patient_info.dob", "transform": "x12_date_to_fhir_date"},
			map[string]interface{}{"targetPath": "gender", "sourcePath": "patient_info.gender_fhir"},
		},
	}
}

func coverage837PBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Coverage",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirCoverages",
		// COB: one row per claim x payer pair (not one per claim) -- a claim with
		// a secondary payer needs its OWN real Coverage resource, since base FHIR
		// Claim.insurance[].coverage references a distinct Coverage per payor
		// relationship. coverage_id is pre-computed per row by the derive script
		// (buildInsuranceAndCoverage) so no transform is needed here.
		"rowsPath": "steps." + derive837PStepAlias + ".step_output._coverage_contexts",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "coverage_id"},
			map[string]interface{}{"targetPath": "status", "literalValue": "active"},
			map[string]interface{}{"targetPath": "beneficiary.reference", "sourcePath": "patient_info.member_id", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"}},
			map[string]interface{}{"targetPath": "subscriberId", "sourcePath": "subscriber_info.member_id"},
			map[string]interface{}{
				"targetPath": "subscriber.reference", "sourcePath": "subscriber_info.member_id", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"},
				"condition": map[string]interface{}{"field": "patient_info.relationship_code", "operator": "equals", "value": "18"},
			},
			map[string]interface{}{"targetPath": "relationship.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/subscriber-relationship"},
			map[string]interface{}{"targetPath": "relationship.coding[0].code", "sourcePath": "patient_info.relationship_fhir"},
			map[string]interface{}{"targetPath": "payor[0].identifier.system", "literalValue": "http://ezhealthkonnect.local/x12-payer-id"},
			map[string]interface{}{"targetPath": "payor[0].identifier.value", "sourcePath": "payer_info.payer_id"},
			map[string]interface{}{"targetPath": "payor[0].display", "sourcePath": "payer_info.name"},
		},
	}
}

func claim837PBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Claim",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirClaims",
		"rowsPath":     "steps." + derive837PStepAlias + ".step_output._claim_contexts",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "claim.patient_control_number", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "claim-"}},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/x12-claim-control-number"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": "claim.patient_control_number"},
			map[string]interface{}{"targetPath": "status", "literalValue": "active"},
			map[string]interface{}{"targetPath": "type.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/claim-type"},
			map[string]interface{}{"targetPath": "type.coding[0].code", "literalValue": "professional"},
			map[string]interface{}{"targetPath": "use", "literalValue": "claim"},
			map[string]interface{}{"targetPath": "created", "sourcePath": "claim.created_at"},
			// priority is required by base FHIR R4 Claim but has no X12 837
			// equivalent (that's a PAS/preauthorization concept, not a claim-
			// submission one) — fixed to "normal", a named, honest simplification.
			map[string]interface{}{"targetPath": "priority.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/processpriority"},
			map[string]interface{}{"targetPath": "priority.coding[0].code", "literalValue": "normal"},
			map[string]interface{}{"targetPath": "patient.reference", "sourcePath": "patient_info.member_id", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"}},
			// Row-scoped, not a fixed literal -- a file with multiple 2000A billing
			// providers must have each claim reference its OWN provider, not always
			// the same one.
			map[string]interface{}{"targetPath": "provider.reference", "sourcePath": "claim.billing_provider_id", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Organization/"}},
			map[string]interface{}{"targetPath": "insurer.identifier.system", "literalValue": "http://ezhealthkonnect.local/x12-payer-id"},
			map[string]interface{}{"targetPath": "insurer.identifier.value", "sourcePath": "payer_info.payer_id"},
			// Claim.facility (base FHIR, 0..1) -- 2310C Service Facility Location, a
			// logical (identifier-only) reference, same pattern as careTeam[].provider
			// below. Only written when the claim actually carries one (most
			// professional claims served at the billing provider's own address
			// don't populate 2310C at all) -- found via X12.org's own COB example.
			map[string]interface{}{
				"targetPath": "facility.identifier.system", "literalValue": "http://hl7.org/fhir/sid/us-npi",
				"condition": map[string]interface{}{"field": "claim.facility_npi", "operator": "exists"},
			},
			map[string]interface{}{"targetPath": "facility.identifier.value", "sourcePath": "claim.facility_npi"},
			map[string]interface{}{"targetPath": "total.value", "sourcePath": "claim.total_charge_amount"},
			map[string]interface{}{"targetPath": "total.currency", "literalValue": "USD"},
		},
		"repeatingGroups": []interface{}{
			map[string]interface{}{
				"targetPath": "diagnosis",
				"rowsPath":   "claim.diagnosis_list",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "diagnosisCodeableConcept.coding[0].system", "literalValue": "http://hl7.org/fhir/sid/icd-10-cm"},
					map[string]interface{}{"targetPath": "diagnosisCodeableConcept.coding[0].code", "sourcePath": "code"},
					map[string]interface{}{"targetPath": "sequence", "sourcePath": "sequence"},
				},
			},
			map[string]interface{}{
				"targetPath": "item",
				"rowsPath":   "claim.service_lines",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "sequence", "sourcePath": "sequence"},
					map[string]interface{}{"targetPath": "productOrService.coding[0].system", "sourcePath": "procedure_system"},
					map[string]interface{}{"targetPath": "productOrService.coding[0].code", "sourcePath": "procedure_code"},
					map[string]interface{}{"targetPath": "quantity.value", "sourcePath": "quantity"},
					map[string]interface{}{"targetPath": "net.value", "sourcePath": "net"},
					map[string]interface{}{"targetPath": "net.currency", "literalValue": "USD"},
					map[string]interface{}{"targetPath": "servicedDate", "sourcePath": "serviced_date", "transform": "x12_date_to_fhir_date"},
					map[string]interface{}{"targetPath": "servicedPeriod.start", "sourcePath": "serviced_start", "transform": "x12_date_to_fhir_date"},
					map[string]interface{}{"targetPath": "servicedPeriod.end", "sourcePath": "serviced_end", "transform": "x12_date_to_fhir_date"},
				},
			},
			map[string]interface{}{
				"targetPath": "careTeam",
				"rowsPath":   "claim.care_team",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "sequence", "sourcePath": "sequence"},
					map[string]interface{}{"targetPath": "role.coding[0].code", "sourcePath": "role"},
					map[string]interface{}{"targetPath": "provider.identifier.system", "sourcePath": "identifier_system"},
					map[string]interface{}{"targetPath": "provider.identifier.value", "sourcePath": "identifier_value"},
				},
			},
			// COB: one entry per payer (primary always sequence 1/focal true; each
			// real 2320 occurrence appended after). Fully pre-flattened by the
			// derive script (buildInsuranceAndCoverage) -- no condition needed here,
			// matching how diagnosis/item/careTeam already work.
			map[string]interface{}{
				"targetPath": "insurance",
				"rowsPath":   "claim.insurance_list",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "sequence", "sourcePath": "sequence"},
					map[string]interface{}{"targetPath": "focal", "sourcePath": "focal"},
					map[string]interface{}{"targetPath": "coverage.reference", "sourcePath": "coverage_id", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Coverage/"}},
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
	if billingProviders, ok := deriveOut["_billing_providers"]; ok {
		data["_billing_providers"] = billingProviders
	}

	claimContexts, _ := data["_claim_contexts"].([]interface{})
	if len(claimContexts) != 2 {
		t.Fatalf("expected 2 claim contexts (subscriber's own + dependent's), got %d: %+v", len(claimContexts), claimContexts)
	}

	// Organization (rowsPath mode -- one resource per distinct 2000A billing
	// provider; this fixture has exactly one)
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
	organizations, ok := message["fhirOrganizations"].([]map[string]interface{})
	if !ok || len(organizations) != 1 {
		t.Fatalf("expected 1 Organization resource, got %d (ok=%v)", len(organizations), ok)
	}
	if organizations[0]["id"] != "organization-billing-1234567890" {
		t.Errorf("expected NPI-derived Organization id, got %v", organizations[0]["id"])
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
	if provider, _ := subClaim["provider"].(map[string]interface{}); provider["reference"] != "Organization/organization-billing-1234567890" {
		t.Errorf("expected Claim.provider.reference to point at the row's own billing provider, got %v", subClaim["provider"])
	}
	// No COB payer in this fixture -- exactly the primary insurance entry.
	insurance, _ := subClaim["insurance"].([]interface{})
	if len(insurance) != 1 {
		t.Errorf("subscriber's own claim: expected 1 insurance entry (primary payer only), got %d", len(insurance))
	}

	// payload.builder fhir_bundle — resourcePaths mixes the array-valued
	// Organization output with the 3 other array-valued rowsPath outputs in
	// the same list (Organization moved from single-resource to rowsPath mode
	// to support multiple 2000A billing providers per file).
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
					"message.fhirOrganizations", "message.fhirPatients", "message.fhirCoverages", "message.fhirClaims",
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

// ─────────────────────────────────────────────────────────────────────────────
// Multi-billing-provider (2000A repeats) — no real sample in
// edi/testdata/real_samples/ carries 2+ top-level 2000A hierarchical levels
// (confirmed by grep across every real fixture there), so this scenario is
// self-authored, same precedent as the RD8-date-range case elsewhere in this
// codebase's own real-parser round-trip tests.
// ─────────────────────────────────────────────────────────────────────────────

func edi837PMultiProviderFixtureLoops() map[string]interface{} {
	return map[string]interface{}{
		"2000A": []interface{}{
			map[string]interface{}{
				"HL": map[string]interface{}{"hierarchicalIdNumber": "1", "hierarchicalLevelCode": "20", "hierarchicalChildCode": "1"},
				"loops": map[string]interface{}{
					"2010AA": map[string]interface{}{
						"NM1": []interface{}{
							map[string]interface{}{"entityIdentifierCode": "85", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "FIRST CLINIC", "identificationCodeQualifier": "XX", "identificationCode": "1111111111"},
						},
					},
					"2000B": []interface{}{
						map[string]interface{}{
							"HL":  map[string]interface{}{"hierarchicalIdNumber": "2", "hierarchicalParentIdNumber": "1", "hierarchicalLevelCode": "22", "hierarchicalChildCode": "0"},
							"SBR": map[string]interface{}{"payerResponsibilitySequenceNumberCode": "P", "individualRelationshipCode": "18"},
							"loops": map[string]interface{}{
								"2010BA": map[string]interface{}{
									"NM1": []interface{}{
										map[string]interface{}{"entityIdentifierCode": "IL", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "ALPHA", "nameFirst": "ANNA", "identificationCodeQualifier": "MI", "identificationCode": "SUB-A"},
									},
								},
								"2010BB": map[string]interface{}{
									"NM1": []interface{}{
										map[string]interface{}{"entityIdentifierCode": "PR", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "PAYER1", "identificationCodeQualifier": "PI", "identificationCode": "PAYER001"},
									},
								},
								"2300": []interface{}{
									map[string]interface{}{
										"CLM": map[string]interface{}{
											"patientControlNumber": "CLM-A-001", "totalClaimChargeAmount": "100.00",
											"healthCareServiceLocation": map[string]interface{}{"placeOfServiceCode": "11", "facilityCodeQualifier": "B", "claimFrequencyCode": "1"},
										},
										"HI": []interface{}{
											map[string]interface{}{"codes": []interface{}{map[string]interface{}{"code": map[string]interface{}{"qualifier": "ABK", "code": "Z00.00"}}}},
										},
										"loops": map[string]interface{}{
											"2400": []interface{}{
												map[string]interface{}{
													"LX": map[string]interface{}{"assignedNumber": "1"},
													"SV1": map[string]interface{}{
														"procedureCode": map[string]interface{}{"qualifier": "HC", "code": "99213"},
														"lineItemChargeAmount": "100.00", "unitOfMeasurementCode": "UN", "serviceUnitCount": "1",
														"diagnosisCodePointer": map[string]interface{}{"pointer1": "1"},
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
					},
				},
			},
			map[string]interface{}{
				"HL": map[string]interface{}{"hierarchicalIdNumber": "5", "hierarchicalLevelCode": "20", "hierarchicalChildCode": "1"},
				"loops": map[string]interface{}{
					"2010AA": map[string]interface{}{
						"NM1": []interface{}{
							map[string]interface{}{"entityIdentifierCode": "85", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "SECOND CLINIC", "identificationCodeQualifier": "XX", "identificationCode": "2222222222"},
						},
					},
					"2000B": []interface{}{
						map[string]interface{}{
							"HL":  map[string]interface{}{"hierarchicalIdNumber": "6", "hierarchicalParentIdNumber": "5", "hierarchicalLevelCode": "22", "hierarchicalChildCode": "0"},
							"SBR": map[string]interface{}{"payerResponsibilitySequenceNumberCode": "P", "individualRelationshipCode": "18"},
							"loops": map[string]interface{}{
								"2010BA": map[string]interface{}{
									"NM1": []interface{}{
										map[string]interface{}{"entityIdentifierCode": "IL", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "BETA", "nameFirst": "BOB", "identificationCodeQualifier": "MI", "identificationCode": "SUB-B"},
									},
								},
								"2010BB": map[string]interface{}{
									"NM1": []interface{}{
										map[string]interface{}{"entityIdentifierCode": "PR", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "PAYER2", "identificationCodeQualifier": "PI", "identificationCode": "PAYER002"},
									},
								},
								"2300": []interface{}{
									map[string]interface{}{
										"CLM": map[string]interface{}{
											"patientControlNumber": "CLM-B-001", "totalClaimChargeAmount": "200.00",
											"healthCareServiceLocation": map[string]interface{}{"placeOfServiceCode": "11", "facilityCodeQualifier": "B", "claimFrequencyCode": "1"},
										},
										"HI": []interface{}{
											map[string]interface{}{"codes": []interface{}{map[string]interface{}{"code": map[string]interface{}{"qualifier": "ABK", "code": "J06.9"}}}},
										},
										"loops": map[string]interface{}{
											"2400": []interface{}{
												map[string]interface{}{
													"LX": map[string]interface{}{"assignedNumber": "1"},
													"SV1": map[string]interface{}{
														"procedureCode": map[string]interface{}{"qualifier": "HC", "code": "99214"},
														"lineItemChargeAmount": "200.00", "unitOfMeasurementCode": "UN", "serviceUnitCount": "1",
														"diagnosisCodePointer": map[string]interface{}{"pointer1": "1"},
													},
													"DTP": []interface{}{
														map[string]interface{}{"dateTimeQualifier": "472", "dateTimePeriodFormatQualifier": "D8", "datePeriod": "20260116"},
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

func TestEDI837PFHIRBuilder_MultiBillingProvider_BuildsDistinctOrganizationsPerProvider(t *testing.T) {
	initFHIRRegistrySvc(t)

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": "837P",
			"loops":          edi837PMultiProviderFixtureLoops(),
		},
	}

	result := runScriptSvc(t, "derive_837p_claim_contexts", derive837PClaimContextsScript, data)
	deriveOut := svcStepOutput(t, result, "derive_837p_claim_contexts")
	for k, v := range result {
		data[k] = v
	}
	svcInjectStepOutput(data, "derive_837p_claim_contexts", deriveOut)

	claimContexts, _ := deriveOut["_claim_contexts"].([]interface{})
	if len(claimContexts) != 2 {
		t.Fatalf("expected 2 claim contexts (one per billing provider's own subscriber), got %d", len(claimContexts))
	}
	billingProviders, _ := deriveOut["_billing_providers"].([]interface{})
	if len(billingProviders) != 2 {
		t.Fatalf("expected 2 distinct billing providers, got %d", len(billingProviders))
	}

	data = runFHIRBuild(t, "build_organization_fhir", organization837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_claim_fhir", claim837PBuildConfig(), data)

	message := data["message"].(map[string]interface{})
	organizations, _ := message["fhirOrganizations"].([]map[string]interface{})
	if len(organizations) != 2 {
		t.Fatalf("expected 2 Organization resources, got %d", len(organizations))
	}
	orgIDs := map[string]bool{}
	for _, o := range organizations {
		orgIDs[fmt.Sprintf("%v", o["id"])] = true
	}
	if !orgIDs["organization-billing-1111111111"] || !orgIDs["organization-billing-2222222222"] {
		t.Errorf("expected both NPI-derived Organization ids, got %+v", orgIDs)
	}

	claims, _ := message["fhirClaims"].([]map[string]interface{})
	if len(claims) != 2 {
		t.Fatalf("expected 2 Claim resources, got %d", len(claims))
	}
	seenRefs := map[string]bool{}
	for _, c := range claims {
		provider, _ := c["provider"].(map[string]interface{})
		seenRefs[fmt.Sprintf("%v", provider["reference"])] = true
	}
	if !seenRefs["Organization/organization-billing-1111111111"] || !seenRefs["Organization/organization-billing-2222222222"] {
		t.Errorf("expected each claim to reference its OWN billing provider, got refs: %+v", seenRefs)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// COB secondary payer (2320/2330B) — synthetic addition to the existing
// multi-claim fixture, proving the derive script's own buildInsuranceAndCoverage
// independent of the real X12.org COB sample (see
// edi_837_real_parser_roundtrip_test.go's own COB test for that real-data proof).
// ─────────────────────────────────────────────────────────────────────────────

func TestEDI837PFHIRBuilder_COBSecondaryPayer_BuildsSecondCoverageAndInsuranceEntry(t *testing.T) {
	initFHIRRegistrySvc(t)

	loops := edi837PFixtureLoops()
	billingLevel := loops["2000A"].([]interface{})[0].(map[string]interface{})
	subLevels := billingLevel["loops"].(map[string]interface{})["2000B"].([]interface{})
	subClaim := subLevels[0].(map[string]interface{})["loops"].(map[string]interface{})["2300"].([]interface{})[0].(map[string]interface{})
	claimLoops := subClaim["loops"].(map[string]interface{})
	claimLoops["2320"] = []interface{}{
		map[string]interface{}{
			"SBR": map[string]interface{}{"payerResponsibilitySequenceNumberCode": "S"},
			"loops": map[string]interface{}{
				"2330B": map[string]interface{}{
					"NM1": []interface{}{
						map[string]interface{}{"entityIdentifierCode": "PR", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "SECONDARY PAYER", "identificationCodeQualifier": "PI", "identificationCode": "PAYER999"},
					},
				},
			},
		},
	}

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": "837P",
			"loops":          loops,
		},
	}

	result := runScriptSvc(t, "derive_837p_claim_contexts", derive837PClaimContextsScript, data)
	deriveOut := svcStepOutput(t, result, "derive_837p_claim_contexts")
	for k, v := range result {
		data[k] = v
	}
	svcInjectStepOutput(data, "derive_837p_claim_contexts", deriveOut)

	data = runFHIRBuild(t, "build_coverage_fhir", coverage837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_claim_fhir", claim837PBuildConfig(), data)

	message := data["message"].(map[string]interface{})
	coverages, _ := message["fhirCoverages"].([]map[string]interface{})
	// 2 claims total (subscriber's own + dependent's) -- subscriber's own now
	// has 2 payers (primary + secondary), dependent's still has 1 -- 3
	// Coverage resources total.
	if len(coverages) != 3 {
		t.Fatalf("expected 3 Coverage resources (primary+secondary for subscriber's claim, primary for dependent's), got %d", len(coverages))
	}
	var secondaryCoverage map[string]interface{}
	for _, c := range coverages {
		if c["id"] == "coverage-CLM-SUB-001-2" {
			secondaryCoverage = c
		}
	}
	if secondaryCoverage == nil {
		t.Fatalf("expected a secondary Coverage with id coverage-CLM-SUB-001-2, got: %+v", coverages)
	}
	payor, _ := secondaryCoverage["payor"].([]interface{})
	if len(payor) == 0 {
		t.Fatalf("expected secondary Coverage.payor to be populated")
	}
	payorMap := payor[0].(map[string]interface{})
	identifier, _ := payorMap["identifier"].(map[string]interface{})
	if identifier["value"] != "PAYER999" {
		t.Errorf("expected secondary payer id PAYER999, got %v", identifier["value"])
	}

	claims, _ := message["fhirClaims"].([]map[string]interface{})
	var subClaimOut map[string]interface{}
	for _, c := range claims {
		if c["id"] == "claim-CLM-SUB-001" {
			subClaimOut = c
		}
	}
	if subClaimOut == nil {
		t.Fatalf("expected claim-CLM-SUB-001 in built Claims, got: %+v", claims)
	}
	insurance, _ := subClaimOut["insurance"].([]interface{})
	if len(insurance) != 2 {
		t.Fatalf("expected 2 insurance entries (primary+secondary), got %d", len(insurance))
	}
}

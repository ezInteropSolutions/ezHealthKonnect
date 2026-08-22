-- V212: Rebuild the Da Vinci PAS template's Zone 2 (FHIR resource
-- construction) on the codebase's own generic, schema-driven FHIR builder
-- engine (fhir.build + payload.builder's fhir_bundle mode) instead of four
-- hand-written enrichment.script steps. Does NOT edit V91/V93/V210/V211 in
-- place -- rewrites the already-stored interface_templates row, same
-- pattern as those migrations.
--
-- Why: build_patient/build_coverage/build_provider/assemble_pas_bundle were
-- four separate hand-written JS functions, one per FHIR resource -- exactly
-- the "one hand-written function per resource type" anti-pattern this
-- repo's CLAUDE.md calls hardcoded even when each script looks clean alone.
-- It directly caused a real bug: assemble_pas_bundle's Bundle entries used
-- urn:uuid: fullUrls while every internal reference used ResourceType/id
-- form (a base FHIR Bundle-spec violation), because each script invented
-- its own convention independently with no shared validation.
--
-- This template already had the right generic infrastructure available and
-- was never wired to it:
--   - fhir.build (services/executors/transform/fhir_build_executor.go)
--     builds one FHIR resource from declarative config (fields[]/
--     repeatingGroups[], sourcePath -> fallbackPaths -> literalValue
--     precedence) -- already proven in production for Patient/Practitioner/
--     Organization-shaped resources (V198__FHIR_Build_Demo_Additional_
--     Resources.sql).
--   - payload.builder's fhir_bundle mode (services/executors/payload/
--     payload_builder_executor.go) assembles already-built resources into a
--     Bundle via fhir/r4/bundle_assembler.go's AssembleEntries, which
--     assigns real urn:uuid: fullUrls and rewrites every internal reference
--     (arbitrary nesting depth) to match -- structurally eliminating the
--     fullUrl/reference-form bug instead of patching around it.
--
-- Design notes (full rationale in the approved plan):
--   - Zone 1's single _pas_envelope.provider.name field is split into
--     firstName/lastName (mirroring how patient.firstName/.lastName already
--     work) -- no name-splitting transform exists in fhir.build's registry,
--     so this is solved by not needing one, not by working around it.
--   - A new, intentionally tiny "Derive PAS Computed Fields" script
--     (sequence 95) computes the handful of things config genuinely cannot
--     express (gender whitelist+default, urgency->priority mapping,
--     diagnosis-code array normalization, service-date defaults, generated
--     IDs) -- zero FHIR resource-shape or profile knowledge, not a
--     reversion to the anti-pattern this migration removes.
--   - A new tiny "Stamp Bundle Profile" script (sequence 135) sets
--     Bundle.meta.profile (the one thing payload.builder's fhir_bundle mode
--     has no config key for) and converts its JSON-string output back to a
--     map for downstream steps.
--   - validate_pas_bundle (140) and submit_to_payer (150) are repointed
--     from steps.assemble_pas_bundle.step_output.pas_bundle (the old
--     script's output) to steps.stamp_bundle_profile.step_output.pas_bundle.
--
-- Proven correct before being transcribed here: services/pas_fhir_builder_test.go
-- runs this exact step chain (same config, byte-for-byte) via the real
-- executors and asserts the resulting bundle validates at strict level with
-- zero wrong-reference-form errors and zero other unexpected errors.

UPDATE interface_templates
SET pipeline_config = jsonb_set(
    pipeline_config,
    '{execution_groups}',
    (
        SELECT jsonb_agg(fixed.grp ORDER BY (fixed.grp ->> 'sequence')::int)
        FROM (
            -- Kept groups (everything except the 4 removed Zone-2 steps),
            -- with 3 surgical patches: pas_envelope's provider name field
            -- split, and validate_pas_bundle / submit_to_payer repointed to
            -- the new stamp_bundle_profile output.
            SELECT
                CASE
                    WHEN g -> 'steps' -> 0 ->> 'step_alias' = 'pas_envelope' THEN
                        jsonb_set(
                            g,
                            '{steps,0,config,mappings}',
                            (
                                SELECT jsonb_agg(m.mapping)
                                FROM (
                                    SELECT CASE
                                        WHEN mapping ->> 'lhs' = '_pas_envelope.provider.name' THEN NULL
                                        ELSE mapping
                                    END AS mapping
                                    FROM jsonb_array_elements(g -> 'steps' -> 0 -> 'config' -> 'mappings') mapping
                                ) m
                                WHERE m.mapping IS NOT NULL
                            ) || jsonb_build_array(
                                jsonb_build_object(
                                    'lhs', '_pas_envelope.provider.firstName',
                                    'rhs', '',
                                    '_label', 'Provider First Name',
                                    '_required', false,
                                    '_hint', 'e.g. Alice'
                                ),
                                jsonb_build_object(
                                    'lhs', '_pas_envelope.provider.lastName',
                                    'rhs', '',
                                    '_label', 'Provider Last Name',
                                    '_required', false,
                                    '_hint', 'e.g. Johnson'
                                )
                            )
                        )
                    WHEN g -> 'steps' -> 0 ->> 'step_alias' = 'validate_pas_bundle' THEN
                        jsonb_set(
                            g,
                            '{steps,0,config,source_field}',
                            '"steps.stamp_bundle_profile.step_output.pas_bundle"'::jsonb
                        )
                    WHEN g -> 'steps' -> 0 ->> 'step_alias' = 'submit_to_payer' THEN
                        jsonb_set(
                            g,
                            '{steps,0,config,bodyRef}',
                            '"steps.stamp_bundle_profile.step_output.pas_bundle"'::jsonb
                        )
                    ELSE g
                END AS grp
            FROM jsonb_array_elements(pipeline_config -> 'execution_groups') g
            WHERE g -> 'steps' -> 0 ->> 'step_alias' NOT IN
                ('build_patient', 'build_coverage', 'build_provider', 'assemble_pas_bundle')

            UNION ALL

            -- New Zone 2 groups: derive -> 5x fhir.build -> payload.builder
            -- assemble (reusing the "assemble_pas_bundle" alias -- safe,
            -- the old group using that alias was excluded above) -> stamp
            -- profile. Config below is copied verbatim from
            -- services/pas_fhir_builder_test.go, which proves it correct
            -- against the real executors before this migration existed.
            SELECT jsonb_array_elements($groups$[{"sequence":95,"label":"Zone 2 — Derive Computed Fields","description":"Pre-built and locked. Pure data derivation only (gender whitelist, priority mapping, diagnosis code normalization, service date defaults, generated IDs) — no FHIR resource-shape or profile knowledge. Every fhir.build step below reads its output via _pas_derived.*.","steps":[{"step_name":"Derive PAS Computed Fields","step_alias":"derive_pas_fields","step_type":"enrichment.script","sequence":95,"enabled":true,"required":true,"description":"Computes values fhir.build's declarative config cannot express: gender whitelist (default unknown), urgency->priority mapping (default normal), diagnosis code array normalization, service date defaults, provider full name (Organization.name fallback), and generated claim/bundle IDs.","config":{"_zone":"pas_core","_locked":true,"script":"// -- Derive PAS Computed Fields --\n// Pure data derivation only -- no FHIR resource shape or profile knowledge.\nvar env = input._pas_envelope || {};\nvar pat = env.patient || {};\nvar cov = env.coverage || {};\nvar req = env.request || {};\n\nvar gender = (pat.gender || \"unknown\").toLowerCase();\nif (gender !== \"male\" && gender !== \"female\" && gender !== \"other\" && gender !== \"unknown\") {\n  gender = \"unknown\";\n}\n\nvar priority = (req.urgency === \"urgent\") ? \"stat\" : \"normal\";\n\nvar diagCodes = req.diagnosisCodes || [];\nif (typeof diagCodes === \"string\") {\n  try { diagCodes = JSON.parse(diagCodes); } catch (e) { diagCodes = [diagCodes]; }\n}\nvar diagnosisCodes = diagCodes.map(function(code) { return { code: String(code) }; });\n\nvar today = new Date().toISOString().substring(0, 10);\nvar serviceStartDate = req.serviceStartDate || today;\nvar serviceEndDate = req.serviceEndDate || serviceStartDate;\n\nvar quantity = req.quantity ? Number(req.quantity) : 1;\n\nvar coverageClasses = [];\nif (cov.planId) {\n  coverageClasses.push({ type: \"plan\", value: cov.planId, name: \"Health Plan\" });\n}\nif (cov.groupNumber) {\n  coverageClasses.push({ type: \"group\", value: cov.groupNumber, name: \"\" });\n}\n\nvar ts = new Date().toISOString();\nvar claimId = \"claim-\" + Date.now();\nvar bundleId = \"pas-bundle-\" + Date.now();\n\nvar prov = env.provider || {};\nvar providerFullName = ((prov.firstName || \"\") + \" \" + (prov.lastName || \"\")).trim();\n\nreturn ({\n  _pas_derived: {\n    gender: gender,\n    priority: priority,\n    diagnosisCodes: diagnosisCodes,\n    serviceStartDate: serviceStartDate,\n    serviceEndDate: serviceEndDate,\n    quantity: quantity,\n    coverageClasses: coverageClasses,\n    claimId: claimId,\n    bundleId: bundleId,\n    createdAt: ts,\n    providerFullName: providerFullName\n  }\n});\n"}}]},{"sequence":100,"label":"Zone 2 — Build FHIR Patient","description":"Pre-built and locked. Builds a FHIR R4 Patient resource via the fhir.build engine (config-driven, generic — zero new code per resource type), conforming to the Da Vinci PAS Subscriber profile. Output: message.fhirPatient.","steps":[{"step_name":"Build FHIR Patient","step_alias":"build_patient_fhir","step_type":"fhir.build","sequence":100,"enabled":true,"required":true,"description":"Builds the Patient resource from _pas_envelope.patient.* and _pas_derived.gender. Profile: Da Vinci PAS Subscriber.","config":{"_zone":"pas_core","_locked":true,"resourceType":"Patient","profile":"base","version":"R4","outputField":"message.fhirPatient","fields":[{"targetPath":"id","sourcePath":"_pas_envelope.patient.memberId"},{"targetPath":"meta.profile[0]","literalValue":"http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-subscriber"},{"targetPath":"identifier[0].type.coding[0].system","literalValue":"http://terminology.hl7.org/CodeSystem/v2-0203"},{"targetPath":"identifier[0].type.coding[0].code","literalValue":"MB"},{"targetPath":"identifier[0].type.coding[0].display","literalValue":"Member Number"},{"targetPath":"identifier[0].system","literalValue":"urn:oid:2.16.840.1.113883.4.6"},{"targetPath":"identifier[0].value","sourcePath":"_pas_envelope.patient.memberId"},{"targetPath":"name[0].use","literalValue":"official"},{"targetPath":"name[0].family","sourcePath":"_pas_envelope.patient.lastName"},{"targetPath":"name[0].given[0]","sourcePath":"_pas_envelope.patient.firstName"},{"targetPath":"birthDate","sourcePath":"_pas_envelope.patient.dob"},{"targetPath":"gender","sourcePath":"_pas_derived.gender"}]}}]},{"sequence":105,"label":"Zone 2 — Build FHIR Coverage","description":"Pre-built and locked. Builds the FHIR R4 Coverage resource via fhir.build, conforming to the Da Vinci PAS Coverage profile. Output: message.fhirCoverage.","steps":[{"step_name":"Build FHIR Coverage","step_alias":"build_coverage_fhir","step_type":"fhir.build","sequence":105,"enabled":true,"required":true,"description":"Builds the Coverage resource linking Patient to payer. Coverage.class[] (plan/group) is built via repeatingGroups off _pas_derived.coverageClasses (0-2 conditionally-present entries, precomputed upstream). Profile: Da Vinci PAS Coverage.","config":{"_zone":"pas_core","_locked":true,"resourceType":"Coverage","profile":"base","version":"R4","outputField":"message.fhirCoverage","fields":[{"targetPath":"id","literalValue":"coverage-1"},{"targetPath":"meta.profile[0]","literalValue":"http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-coverage"},{"targetPath":"identifier[0].system","literalValue":"http://hl7.org/fhir/sid/us-npi"},{"targetPath":"identifier[0].value","sourcePath":"_pas_envelope.coverage.payerId"},{"targetPath":"status","literalValue":"active"},{"targetPath":"subscriber.reference","sourcePath":"_pas_envelope.patient.memberId","transform":"string_prefix","valueMap":{"prefix":"Patient/"}},{"targetPath":"subscriberId","sourcePath":"_pas_envelope.patient.memberId"},{"targetPath":"beneficiary.reference","sourcePath":"_pas_envelope.patient.memberId","transform":"string_prefix","valueMap":{"prefix":"Patient/"}},{"targetPath":"relationship.coding[0].system","literalValue":"http://terminology.hl7.org/CodeSystem/subscriber-relationship"},{"targetPath":"relationship.coding[0].code","literalValue":"self"},{"targetPath":"payor[0].identifier.system","literalValue":"http://hl7.org/fhir/sid/us-npi"},{"targetPath":"payor[0].identifier.value","sourcePath":"_pas_envelope.coverage.payerId"}],"repeatingGroups":[{"targetPath":"class","rowsPath":"_pas_derived.coverageClasses","fields":[{"targetPath":"type.coding[0].system","literalValue":"http://terminology.hl7.org/CodeSystem/coverage-class"},{"targetPath":"type.coding[0].code","sourcePath":"type"},{"targetPath":"value","sourcePath":"value"},{"targetPath":"name","sourcePath":"name"}]}]}}]},{"sequence":110,"label":"Zone 2 — Build FHIR Practitioner","description":"Pre-built and locked. Builds the FHIR R4 Practitioner resource (ordering clinician) via fhir.build. Output: message.fhirPractitioner.","steps":[{"step_name":"Build FHIR Practitioner","step_alias":"build_practitioner_fhir","step_type":"fhir.build","sequence":110,"enabled":true,"required":true,"description":"Builds the Practitioner resource from _pas_envelope.provider.firstName/lastName/npi. Referenced on Claim.item.provider.","config":{"_zone":"pas_core","_locked":true,"resourceType":"Practitioner","profile":"base","version":"R4","outputField":"message.fhirPractitioner","fields":[{"targetPath":"id","literalValue":"practitioner-1"},{"targetPath":"meta.profile[0]","literalValue":"http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-practitioner"},{"targetPath":"identifier[0].system","literalValue":"http://hl7.org/fhir/sid/us-npi"},{"targetPath":"identifier[0].value","sourcePath":"_pas_envelope.provider.npi"},{"targetPath":"name[0].family","sourcePath":"_pas_envelope.provider.lastName"},{"targetPath":"name[0].given[0]","sourcePath":"_pas_envelope.provider.firstName"}]}}]},{"sequence":115,"label":"Zone 2 — Build FHIR Organization","description":"Pre-built and locked. Builds the FHIR R4 Organization resource (billing/requesting entity) via fhir.build. Output: message.fhirOrganization.","steps":[{"step_name":"Build FHIR Organization","step_alias":"build_organization_fhir","step_type":"fhir.build","sequence":115,"enabled":true,"required":true,"description":"Builds the Organization resource. identifier falls back from facilityNpi to the clinician's own npi (native fallbackPaths, no script needed). name falls back from organizationName to the derived provider full name to a literal default (three-tier fallback).","config":{"_zone":"pas_core","_locked":true,"resourceType":"Organization","profile":"base","version":"R4","outputField":"message.fhirOrganization","fields":[{"targetPath":"id","literalValue":"organization-1"},{"targetPath":"meta.profile[0]","literalValue":"http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-requestor"},{"targetPath":"active","literalValue":"true"},{"targetPath":"identifier[0].system","literalValue":"http://hl7.org/fhir/sid/us-npi"},{"targetPath":"identifier[0].value","sourcePath":"_pas_envelope.provider.facilityNpi","fallbackPaths":["_pas_envelope.provider.npi"]},{"targetPath":"name","sourcePath":"_pas_envelope.provider.organizationName","fallbackPaths":["_pas_derived.providerFullName"],"literalValue":"Requesting Organization"}]}}]},{"sequence":120,"label":"Zone 2 — Build FHIR Claim","description":"Pre-built and locked. Builds the FHIR R4 Claim resource via fhir.build, conforming to the Da Vinci PAS Claim profile. Output: message.fhirClaim.","steps":[{"step_name":"Build FHIR Claim","step_alias":"build_claim_fhir","step_type":"fhir.build","sequence":120,"enabled":true,"required":true,"description":"Builds the Claim resource. Claim.use is a fixed literalValue \"preauthorization\" (IG-required). References to Coverage/Practitioner/Organization use the SAME fixed IDs those resources' own fhir.build steps set (literalValue \"Coverage/coverage-1\" etc.), guaranteeing correlation by construction. diagnosis[] built via repeatingGroups off _pas_derived.diagnosisCodes.","config":{"_zone":"pas_core","_locked":true,"resourceType":"Claim","profile":"base","version":"R4","outputField":"message.fhirClaim","fields":[{"targetPath":"id","sourcePath":"_pas_derived.claimId"},{"targetPath":"meta.profile[0]","literalValue":"http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-claim"},{"targetPath":"identifier[0].system","literalValue":"urn:ietf:rfc:3986"},{"targetPath":"identifier[0].value","sourcePath":"_pas_derived.claimId"},{"targetPath":"status","literalValue":"active"},{"targetPath":"type.coding[0].system","literalValue":"http://terminology.hl7.org/CodeSystem/claim-type"},{"targetPath":"type.coding[0].code","literalValue":"professional"},{"targetPath":"use","literalValue":"preauthorization"},{"targetPath":"patient.reference","sourcePath":"_pas_envelope.patient.memberId","transform":"string_prefix","valueMap":{"prefix":"Patient/"}},{"targetPath":"created","sourcePath":"_pas_derived.createdAt"},{"targetPath":"insurer.identifier.system","literalValue":"http://hl7.org/fhir/sid/us-npi"},{"targetPath":"insurer.identifier.value","sourcePath":"_pas_envelope.coverage.payerId"},{"targetPath":"provider.reference","literalValue":"Organization/organization-1"},{"targetPath":"priority.coding[0].system","literalValue":"http://terminology.hl7.org/CodeSystem/processpriority"},{"targetPath":"priority.coding[0].code","sourcePath":"_pas_derived.priority"},{"targetPath":"insurance[0].sequence","literalValue":"1"},{"targetPath":"insurance[0].focal","literalValue":"true"},{"targetPath":"insurance[0].coverage.reference","literalValue":"Coverage/coverage-1"},{"targetPath":"item[0].sequence","literalValue":"1"},{"targetPath":"item[0].extension[0].url","literalValue":"http://hl7.org/fhir/us/davinci-pas/StructureDefinition/extension-serviceItemRequestedDate"},{"targetPath":"item[0].extension[0].valuePeriod.start","sourcePath":"_pas_derived.serviceStartDate"},{"targetPath":"item[0].extension[0].valuePeriod.end","sourcePath":"_pas_derived.serviceEndDate"},{"targetPath":"item[0].category.coding[0].system","literalValue":"https://codesystem.x12.org/005010/1365"},{"targetPath":"item[0].category.coding[0].code","literalValue":"1"},{"targetPath":"item[0].category.coding[0].display","literalValue":"Medical Care"},{"targetPath":"item[0].productOrService.coding[0].system","literalValue":"http://www.ama-assn.org/go/cpt"},{"targetPath":"item[0].productOrService.coding[0].code","sourcePath":"_pas_envelope.request.serviceCode"},{"targetPath":"item[0].quantity.value","sourcePath":"_pas_derived.quantity","transform":"cda_decimal_string_to_number"},{"targetPath":"item[0].provider.reference","literalValue":"Practitioner/practitioner-1"}],"repeatingGroups":[{"targetPath":"diagnosis","rowsPath":"_pas_derived.diagnosisCodes","fields":[{"targetPath":"diagnosisCodeableConcept.coding[0].system","literalValue":"http://hl7.org/fhir/sid/icd-10-cm"},{"targetPath":"diagnosisCodeableConcept.coding[0].code","sourcePath":"code"}]}]}}]},{"sequence":130,"label":"Zone 2 — Assemble Da Vinci PAS Request Bundle","description":"Pre-built and locked. Assembles the 5 built resources into a Da Vinci PAS-conformant FHIR R4 Bundle using payload.builder's fhir_bundle mode, which calls fhir/r4/bundle_assembler.go's AssembleEntries to auto-assign urn:uuid: fullUrls and rewrite every internal reference to match — structurally eliminating the fullUrl/reference-form mismatch the previous hand-written assembly script had. Output: steps.assemble_pas_bundle.step_output.payload (a JSON string).","steps":[{"step_name":"Assemble PAS Bundle","step_alias":"assemble_pas_bundle","step_type":"payload.builder","sequence":130,"enabled":true,"required":true,"description":"Lists the 5 built resources as resourcePaths; AssembleEntries handles fullUrl assignment and reference rewriting automatically — every cross-reference (Claim.patient, Claim.insurance[0].coverage, Claim.provider, Claim.item[0].provider, Coverage.subscriber, Coverage.beneficiary) is guaranteed consistent, not hand-coordinated.","config":{"_zone":"pas_core","_locked":true,"mode":"fhir_bundle","fhirBundle":{"bundleType":"collection","resourcePaths":["message.fhirClaim","message.fhirPatient","message.fhirCoverage","message.fhirPractitioner","message.fhirOrganization"]}}}]},{"sequence":135,"label":"Zone 2 — Stamp Bundle Profile","description":"Pre-built and locked. payload.builder's fhir_bundle mode has no config key for Bundle.meta.profile and outputs an already-marshaled JSON string, not a map — this step sets the required Da Vinci PAS request-bundle profile URL and converts the string back to a map for downstream steps (validate_pas_bundle, submit_to_payer). Output: steps.stamp_bundle_profile.step_output.pas_bundle.","steps":[{"step_name":"Stamp Bundle Profile","step_alias":"stamp_bundle_profile","step_type":"enrichment.script","sequence":135,"enabled":true,"required":true,"description":"Sets Bundle.meta.profile to the Da Vinci PAS request-bundle profile URL and re-parses the assembled bundle into a map. Downstream steps (validate_pas_bundle, submit_to_payer) reference steps.stamp_bundle_profile.step_output.pas_bundle instead of the old steps.assemble_pas_bundle.step_output.pas_bundle.","config":{"_zone":"pas_core","_locked":true,"script":"var bundle = JSON.parse(input.steps.assemble_pas_bundle.step_output.payload);\nbundle.meta = { profile: [\"http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-pas-request-bundle\"] };\nreturn ({ pas_bundle: bundle });\n"}}]}]$groups$::jsonb) AS grp
        ) fixed
    )
)
WHERE slug = 'davinci-pas-r4';

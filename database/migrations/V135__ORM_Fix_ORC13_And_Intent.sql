-- V135: Fix ORM ServiceRequest field mappings — remove ORC.13 and correct ORC.1 → intent
--
-- Two template-quality issues identified via FHIR validation:
--
-- 1. ORC.13 ("Enterer's Location" PL / "Order Status Modifier" in v2.7+) was mapped to
--    ServiceRequest.language, ServiceRequest.instantiatesCanonical, and ServiceRequest.status
--    depending on template version. None of these targets accept a raw PL/CE composite:
--      • language     — requires BCP-47 code (e.g. "en-US")
--      • instantiatesCanonical — requires a canonical URL
--      • status       — already provided by ORC.5; two sources collide
--    A composite value like "^^^^OFFICE^^^^^Office" in Resource.language causes the FHIR
--    validator to use it as displayLanguage, cascading false failures across every value-set
--    lookup on the entire resource.
--
-- 2. ORC.1 (Order Control Code, HL7 Table 0119) was mapped to ServiceRequest.status in
--    several templates. The correct FHIR target is ServiceRequest.intent — ORC.1 codes
--    describe the order's intent/disposition (NW=order, CA=proposal, etc.), not its status.
--    ServiceRequest.intent is required (1..1) in FHIR R4; without it the resource fails
--    structural validation even when every other field is correct.
--
-- Both fixes apply to ORM^O01, OML^O21, and OMG^O19 OOB templates.

-- ── Step 1: Remove ORC.13 from all ORM ServiceRequest field mapping arrays ────────────────

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,ServiceRequest,mappings}',
    (
        SELECT COALESCE(jsonb_agg(elem ORDER BY elem->>'hl7Path'), '[]'::jsonb)
        FROM jsonb_array_elements(
            template_config -> 'resources' -> 'ServiceRequest' -> 'mappings'
        ) AS elem
        WHERE elem->>'hl7Path' != 'ORC.13'
    )
)
WHERE message_type IN ('ORM^O01', 'OML^O21', 'OMG^O19')
  AND is_default = true
  AND template_config -> 'resources' -> 'ServiceRequest' -> 'mappings' IS NOT NULL;

-- ── Step 2: Re-point ORC.1 → ServiceRequest.intent (with HL7 Table 0119 valueMap) ────────
-- ORC.1 entries that currently target 'ServiceRequest.status' are pointing at the wrong
-- FHIR field.  Replace their fhirPath and inject the order-control-to-intent valueMap.
-- Entries that already target 'intent' are left untouched by the WHERE clause.

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,ServiceRequest,mappings}',
    (
        SELECT COALESCE(jsonb_agg(
            CASE
                WHEN (elem->>'hl7Path') = 'ORC.1'
                 AND (elem->>'fhirPath') = 'ServiceRequest.status'
                THEN
                    elem
                    -- point to the correct FHIR field
                    || jsonb_build_object('fhirPath', 'ServiceRequest.intent')
                    -- inject the HL7 Table 0119 → FHIR RequestIntent valueMap
                    || jsonb_build_object('valueMap', '{
                        "NW":  "order",
                        "CA":  "proposal",
                        "OC":  "proposal",
                        "HD":  "proposal",
                        "RP":  "order",
                        "SC":  "order",
                        "IP":  "order",
                        "CM":  "order",
                        "DC":  "proposal",
                        "DE":  "proposal",
                        "DF":  "proposal",
                        "DR":  "proposal",
                        "FU":  "plan",
                        "LI":  "plan",
                        "PA":  "plan",
                        "RE":  "reflex-order",
                        "RL":  "order",
                        "RO":  "reflex-order",
                        "RQ":  "proposal",
                        "RR":  "order",
                        "RU":  "order",
                        "SN":  "order",
                        "SS":  "order",
                        "UA":  "order",
                        "UN":  "order",
                        "UR":  "order",
                        "UX":  "order",
                        "XO":  "order",
                        "XR":  "order"
                    }'::jsonb)
                ELSE elem
            END
            ORDER BY elem->>'hl7Path'
        ), '[]'::jsonb)
        FROM jsonb_array_elements(
            template_config -> 'resources' -> 'ServiceRequest' -> 'mappings'
        ) AS elem
    )
)
WHERE message_type IN ('ORM^O01', 'OML^O21', 'OMG^O19')
  AND is_default = true
  AND template_config -> 'resources' -> 'ServiceRequest' -> 'mappings' IS NOT NULL
  AND EXISTS (
      SELECT 1
      FROM jsonb_array_elements(
          template_config -> 'resources' -> 'ServiceRequest' -> 'mappings'
      ) AS elem
      WHERE elem->>'hl7Path' = 'ORC.1'
        AND elem->>'fhirPath' = 'ServiceRequest.status'
  );

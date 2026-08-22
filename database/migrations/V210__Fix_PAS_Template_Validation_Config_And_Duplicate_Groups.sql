-- V210: Fix three bugs in the Da Vinci PAS interface template (davinci-pas-r4)
-- seeded by V91/V93. Does NOT edit V91/V93 in place (would break Flyway
-- checksums) — instead rewrites the already-stored interface_templates row.
--
-- Bug 1 — validate_pas_bundle (seq 140) config used the wrong JSON key for
--   validation depth: "level" instead of the Go executor's actual field,
--   services/executors/validation/fhir_validation_executor.go's
--   fhirValidationConfig.ValidationLevel `json:"validation_level"`. Because
--   json.Unmarshal silently drops unmatched keys, "level" was never read at
--   all — the step always ran at the struct's own default ("standard"),
--   regardless of what "level" was set to. fhir/r4/validator.go gates ALL
--   constraint-predicate checking (the six PAS rules in fhir/r4/pas_profiles.go:
--   pas-claim-1/2/3, pas-bundle-1/2/3) behind opts.Level >= LevelStrict
--   (ValidateResource ~L165, ValidateBundle ~L202/213), so those rules never
--   fired. Fix: rename level -> validation_level and set it to "strict".
--
-- Bug 2 — same step's config used "required_resource_types" where the
--   executor's struct expects RequiredResources []string
--   `json:"required_resources"` — another silently-dropped key, making the
--   required-resource-type check a permanent no-op. Fix: rename to
--   required_resources (value unchanged: ["Claim","Patient","Coverage"]).
--
-- Bug 3 — V93's UPDATE (pipeline_config.execution_groups || <2 new groups>)
--   ended up applied multiple times against existing environments' seeded
--   data, leaving 3 duplicate copies each of the "Receive PA Request"
--   (receive_request, connector.inbound, seq 5) and "Deliver PA Decision"
--   (deliver_decision, connector.outbound, seq 295) execution_groups — 15
--   groups total instead of 9. An interface built from this template would
--   inherit 3 duplicate inbound listeners (port-binding conflict) and 3
--   duplicate outbound deliveries of the PA decision. Fix: dedupe to exactly
--   one execution_group per step_alias, keeping each group's first
--   occurrence, and re-sort the array by each group's own "sequence" field.

UPDATE interface_templates
SET pipeline_config = jsonb_set(
    pipeline_config,
    '{execution_groups}',
    (
        SELECT jsonb_agg(fixed.grp ORDER BY (fixed.grp ->> 'sequence')::int)
        FROM (
            SELECT DISTINCT ON (deduped.alias)
                CASE
                    WHEN deduped.alias = 'validate_pas_bundle' THEN
                        jsonb_set(
                            deduped.grp,
                            '{steps,0,config}',
                            ((deduped.grp -> 'steps' -> 0 -> 'config') - 'level' - 'required_resource_types')
                                || jsonb_build_object(
                                       'validation_level', 'strict',
                                       'required_resources',
                                       deduped.grp -> 'steps' -> 0 -> 'config' -> 'required_resource_types'
                                   )
                        )
                    ELSE deduped.grp
                END AS grp
            FROM (
                SELECT
                    elem.grp,
                    elem.grp -> 'steps' -> 0 ->> 'step_alias' AS alias,
                    elem.ord
                FROM jsonb_array_elements(pipeline_config -> 'execution_groups') WITH ORDINALITY AS elem(grp, ord)
            ) deduped
            ORDER BY deduped.alias, deduped.ord
        ) fixed
    )
)
WHERE slug = 'davinci-pas-r4';

-- V211: Fix two functional bugs and one resilience gap in the Da Vinci PAS
-- interface template (davinci-pas-r4), found while auditing the template's
-- reliability after V210. Does NOT edit V91/V92/V93/V210 in place — rewrites
-- the already-stored interface_templates row, same pattern as V210.
--
-- Bug 5 — route_on_decision's "set_variable" action does nothing.
--   services/executors/control/conditional_executor.go's SwitchCaseExecutor
--   has no "set_variable" case; every one of the 10 set_variable actions
--   (2 per branch x 5 branches: AA/AD/CP/PE/default) falls into the
--   `default:` branch, which only logs "Unsupported action... skipping" and
--   never writes _pa_status/_pa_status_label. The executor already has a
--   working "set_value" action (targetField/value keys) that does exactly
--   this. Fix: rewrite each {action:"set_variable", variable:"X", value:"Y"}
--   to {action:"set_value", targetField:"X", value:"Y"}.
--
-- Bug 6 — deliver_decision sends the ORIGINAL inbound PA request instead of
--   the payer's decision. V93 set contentField to
--   "steps.parse_response.step_output", but the parser step's real alias is
--   parse_claim_response (V91), not parse_response. outbound_connector_executor.go
--   resolves the wrong alias to nil and silently falls back to sending the
--   entire original inbound message to the caller's callback URL — no error
--   raised anywhere. Fix: correct the alias.
--
-- Resilience gap — deliver_decision has no retry. No step in this template
--   sets retry/errorHandling config, so a transient failure delivering the
--   PA decision to the caller's callback URL is a silent, permanent loss of
--   that decision (one attempt, no retry, no dead-letter). submit_to_payer
--   already has its own independent HTTP-level retry + circuit breaker via
--   APIEnrichmentConfig and needs no change. deliver_decision does not have
--   an equivalent, so this adds pipeline-level retry using the engine's
--   standard retry config shape (services/executors/retry_utils.go
--   ParseRetryConfig: retry.enabled/maxRetries/delayMs/backoffMultiplier/
--   maxDelayMs), confirmed honored by the live execution path
--   (transformation_pipeline_helpers.go's ExecutePipeline wraps every step
--   in executors.ExecuteWithRetry using this exact config).

-- ── Helper functions (dropped again at the end of this migration) ─────────
CREATE OR REPLACE FUNCTION _v211_fix_pas_action(action jsonb) RETURNS jsonb AS $fn$
    SELECT CASE
        WHEN action->>'action' = 'set_variable' THEN
            (action - 'action' - 'variable')
                || jsonb_build_object('action', 'set_value', 'targetField', action->'variable')
        ELSE action
    END;
$fn$ LANGUAGE sql IMMUTABLE;

CREATE OR REPLACE FUNCTION _v211_fix_pas_actions(actions jsonb) RETURNS jsonb AS $fn$
    SELECT COALESCE(jsonb_agg(_v211_fix_pas_action(a.value) ORDER BY a.ord), '[]'::jsonb)
    FROM jsonb_array_elements(actions) WITH ORDINALITY AS a(value, ord);
$fn$ LANGUAGE sql IMMUTABLE;

CREATE OR REPLACE FUNCTION _v211_fix_pas_case(c jsonb) RETURNS jsonb AS $fn$
    SELECT jsonb_set(c, '{actions}', _v211_fix_pas_actions(c->'actions'));
$fn$ LANGUAGE sql IMMUTABLE;

CREATE OR REPLACE FUNCTION _v211_fix_pas_cases(cases jsonb) RETURNS jsonb AS $fn$
    SELECT COALESCE(jsonb_agg(_v211_fix_pas_case(c.value) ORDER BY c.ord), '[]'::jsonb)
    FROM jsonb_array_elements(cases) WITH ORDINALITY AS c(value, ord);
$fn$ LANGUAGE sql IMMUTABLE;

-- ── The fix ─────────────────────────────────────────────────────────────
UPDATE interface_templates
SET pipeline_config = jsonb_set(
    pipeline_config,
    '{execution_groups}',
    (
        SELECT jsonb_agg(
            CASE
                WHEN g -> 'steps' -> 0 ->> 'step_alias' = 'route_on_decision' THEN
                    jsonb_set(
                        g,
                        '{steps,0,config}',
                        jsonb_set(
                            jsonb_set(
                                g -> 'steps' -> 0 -> 'config',
                                '{cases}',
                                _v211_fix_pas_cases(g -> 'steps' -> 0 -> 'config' -> 'cases')
                            ),
                            '{default,actions}',
                            _v211_fix_pas_actions(g -> 'steps' -> 0 -> 'config' -> 'default' -> 'actions')
                        )
                    )
                WHEN g -> 'steps' -> 0 ->> 'step_alias' = 'deliver_decision' THEN
                    jsonb_set(
                        g,
                        '{steps,0,config}',
                        (g -> 'steps' -> 0 -> 'config') || jsonb_build_object(
                            'contentField', 'steps.parse_claim_response.step_output',
                            'retry', jsonb_build_object(
                                'enabled', true,
                                'maxRetries', 3,
                                'delayMs', 1000,
                                'backoffMultiplier', 2.0,
                                'maxDelayMs', 60000
                            )
                        )
                    )
                ELSE g
            END
            ORDER BY ord
        )
        FROM jsonb_array_elements(pipeline_config -> 'execution_groups') WITH ORDINALITY AS t(g, ord)
    )
)
WHERE slug = 'davinci-pas-r4';

DROP FUNCTION IF EXISTS _v211_fix_pas_cases(jsonb);
DROP FUNCTION IF EXISTS _v211_fix_pas_case(jsonb);
DROP FUNCTION IF EXISTS _v211_fix_pas_actions(jsonb);
DROP FUNCTION IF EXISTS _v211_fix_pas_action(jsonb);

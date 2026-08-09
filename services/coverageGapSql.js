'use strict';
/**
 * services/coverageGapSql.js
 *
 * Shared SQL fragment for "does this message have a CDA Coverage Audit gap"
 * — used by both controllers/MessageController.js (shared-table fallback
 * query) and services/InterfaceTableManager.js (dedicated per-interface
 * table query) so the definition of "gap" can't drift between the two.
 *
 * Dedupes to the LATEST report per destination (step_id) first — a
 * re-delivered message shouldn't show a stale gap from an earlier attempt —
 * mirroring the same DISTINCT ON (ca.step_id) ... ORDER BY ca.step_id,
 * ca.created_at DESC pattern controllers/MessageController.js's
 * getCoverageAudit handler already uses.
 */

/**
 * @param {string} messageIdExpr - SQL expression for the row's message_id
 *   column (e.g. "mpe.message_id" or a sanitized dedicated-table alias).
 * @returns {string} an EXISTS(...) boolean SQL expression, no trailing alias.
 */
function hasCoverageGapSql(messageIdExpr) {
    return `EXISTS (
        SELECT 1 FROM (
            SELECT DISTINCT ON (ca.step_id) ca.category_stats
            FROM cda_coverage_audits ca
            WHERE ca.message_id = ${messageIdExpr}
            ORDER BY ca.step_id, ca.created_at DESC
        ) latest
        CROSS JOIN LATERAL jsonb_array_elements(latest.category_stats->'categories') cat
        WHERE jsonb_array_length(cat->'missed') > 0
    )`;
}

module.exports = { hasCoverageGapSql };

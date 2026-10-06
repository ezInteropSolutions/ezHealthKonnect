-- V273: Coverage Audit — source_format column (generalizing beyond CDA)
-- Applied: 2026-09-26
--
-- Coverage Audit is being generalized to report on HL7 v2, FHIR, EDI X12,
-- and NCPDP (SCRIPT + Telecom D.0) source messages, not just CDA/CCD (see
-- services/cda_coverage/registry.go and CLAUDE.md's Coverage Audit
-- generalization section). Per an explicit product decision, this is
-- additive reuse, not a rename: cda_coverage_audits and
-- interfaces.cda_coverage_audit_config keep their CDA-flavored names and are
-- shared by every format going forward. This migration adds the one new
-- column needed to tell rows apart by which format's engine actually built
-- them.

-- ============================================================
-- COVERAGE AUDITS — SOURCE FORMAT DISCRIMINATOR
-- ============================================================
-- DEFAULT 'ccda' is a backfill safety net for every row inserted before this
-- migration (the only format this table ever recorded until now) — every new
-- INSERT (services/cda_coverage/report.go's SaveReport) sets this column
-- explicitly from the message envelope's own "_format" value, never relying
-- on the default going forward.
ALTER TABLE cda_coverage_audits
    ADD COLUMN IF NOT EXISTS source_format VARCHAR(30) NOT NULL DEFAULT 'ccda';

CREATE INDEX IF NOT EXISTS idx_cda_coverage_audits_source_format
    ON cda_coverage_audits(source_format);

COMMENT ON COLUMN cda_coverage_audits.source_format IS
    'Which source engine this row''s coverage inventory was built against — the services/coverage.FormatAdapter.FormatKey() registered for the message''s own "_format" (e.g. "ccda", "hl7v2", "fhir", "edi_x12", "ncpdp_script", "ncpdp_telecom"). Defaults to ''ccda'' as a backfill for every row inserted before this column existed, when Coverage Audit only ever supported CDA. Kept on this CDA-named table by explicit product decision (additive reuse, not a rename) — see CLAUDE.md''s Coverage Audit generalization section.';

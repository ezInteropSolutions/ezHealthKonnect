-- V283: Shared (global) code translations were never actually unique.
--
-- cda_code_translations has UNIQUE (interface_id, source_system, source_code, target_system), but a shared
-- translation has interface_id = NULL, and Postgres treats NULLs as distinct in a unique constraint: the same
-- shared translation could be stored any number of times, and "ON CONFLICT ... DO UPDATE" never fired for it.
-- The Code Translations page adds and edits shared rows, so this must hold.

-- Keep the newest row of any existing duplicate shared translations.
DELETE FROM cda_code_translations a
USING cda_code_translations b
WHERE a.interface_id IS NULL
  AND b.interface_id IS NULL
  AND a.source_system = b.source_system
  AND a.source_code   = b.source_code
  AND a.target_system = b.target_system
  AND (a.created_at, a.id) < (b.created_at, b.id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_cda_code_translations_global
    ON cda_code_translations (source_system, source_code, target_system)
    WHERE interface_id IS NULL;

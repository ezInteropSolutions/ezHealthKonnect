-- V87: First-run setup wizard support
-- Adds setup_complete and org_name to system_settings.
-- setup_complete is seeded as TRUE for existing installs (where any user has ever
-- logged in), and FALSE for truly fresh installs — which will be directed to
-- /setup.html before any other page can be accessed.

-- setup_complete: whether the first-run wizard has been completed
INSERT INTO system_settings (key, value, description)
SELECT
    'setup_complete',
    CASE
        WHEN (SELECT COUNT(*) FROM users WHERE last_login_at IS NOT NULL) > 0
        THEN 'true'::jsonb
        ELSE 'false'::jsonb
    END,
    'Whether the first-run setup wizard has been completed. Set to true automatically for existing installs.'
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE key = 'setup_complete');

-- org_name: organisation name shown in the UI and included in the licence JWT
INSERT INTO system_settings (key, value, description)
SELECT
    'org_name',
    '"ezInterop Solutions"'::jsonb,
    'Organisation name set during first-run setup.'
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE key = 'org_name');

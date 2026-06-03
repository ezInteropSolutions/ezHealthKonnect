-- V100: Lock the V2 placeholder admin so it cannot be used before setup completes.
--
-- The V2 migration seeds admin@ezhealthkonnect.com with the well-known
-- bcrypt hash for "admin123". This migration replaces that hash with an
-- invalid value that bcrypt will never accept, and marks the account as
-- pending so the setupCheck middleware keeps redirecting to /setup.html.
-- The setup wizard (setupController.js) finds this account by email and
-- overwrites it with the real admin credentials chosen by the installer.

UPDATE users
SET
    password_hash        = 'LOCKED_PENDING_SETUP',
    status               = 'pending',
    force_password_reset = true
WHERE email = 'admin@ezhealthkonnect.com'
  AND password_hash = '$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi';

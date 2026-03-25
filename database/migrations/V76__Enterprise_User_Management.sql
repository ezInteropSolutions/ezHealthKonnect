-- V76: Enterprise User Management additions
-- Adds invite/force-reset columns + performance indexes

ALTER TABLE users ADD COLUMN IF NOT EXISTS force_password_reset BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN IF NOT EXISTS invited_by UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS invitation_expires_at TIMESTAMP WITH TIME ZONE;

-- Fast token lookup for invitation acceptance
CREATE INDEX IF NOT EXISTS idx_users_email_verification_token
    ON users(email_verification_token)
    WHERE email_verification_token IS NOT NULL;

-- Fast lockout check during login
CREATE INDEX IF NOT EXISTS idx_users_locked_until
    ON users(locked_until)
    WHERE locked_until IS NOT NULL;

-- Fast invite lookup
CREATE INDEX IF NOT EXISTS idx_users_invited_by
    ON users(invited_by)
    WHERE invited_by IS NOT NULL;

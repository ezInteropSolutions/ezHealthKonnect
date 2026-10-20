require('dotenv').config();

module.exports = {
    PORT: process.env.PORT || 3000,
    NODE_ENV: process.env.NODE_ENV || 'development',
    // No fallback values: config/SecretsGuard.js refuses to start without real
    // secrets, so a missing secret can never silently become a public default.
    JWT_SECRET: process.env.JWT_SECRET,
    SESSION_SECRET: process.env.SESSION_SECRET,
    POSTGRES_DATABASE: process.env.POSTGRES_DATABASE,
    POSTGRES_USERNAME: process.env.POSTGRES_USERNAME,
    POSTGRES_PASSWORD: process.env.POSTGRES_PASSWORD,
    HIPAA_COMPLIANCE_MODE: process.env.HIPAA_COMPLIANCE_MODE === 'true',
    GDPR_COMPLIANCE_MODE: process.env.GDPR_COMPLIANCE_MODE === 'true'
};
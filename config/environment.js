require('dotenv').config();

module.exports = {
    PORT: process.env.PORT || 3000,
    NODE_ENV: process.env.NODE_ENV || 'development',
    JWT_SECRET: process.env.JWT_SECRET || 'your-secret-key-change-this-in-production',
    SESSION_SECRET: process.env.SESSION_SECRET || 'your-session-secret',
    POSTGRES_DATABASE: process.env.POSTGRES_DATABASE,
    POSTGRES_USERNAME: process.env.POSTGRES_USERNAME,
    POSTGRES_PASSWORD: process.env.POSTGRES_PASSWORD,
    HIPAA_COMPLIANCE_MODE: process.env.HIPAA_COMPLIANCE_MODE === 'true',
    GDPR_COMPLIANCE_MODE: process.env.GDPR_COMPLIANCE_MODE === 'true'
};
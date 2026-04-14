const session = require('express-session');
const PgSession = require('connect-pg-simple')(session);
const { Pool } = require('pg');
const { SESSION_SECRET, NODE_ENV } = require('./environment');

// Build a lightweight PG pool using the same env vars as the rest of the app.
// connect-pg-simple creates the "session" table automatically on first use
// (pruneSessionInterval handles expiry cleanup).
const pgPool = new Pool({
    host:     process.env.DB_HOST     || 'localhost',
    port:     parseInt(process.env.DB_PORT || '5432', 10),
    database: process.env.DB_NAME     || 'ezhealthkonnect',
    user:     process.env.DB_USER     || 'ezhealth_user',
    password: process.env.DB_PASSWORD || '',
    ssl:      process.env.DB_SSL === 'true' ? { rejectUnauthorized: false } : false,
    max: 3,   // small pool — sessions only
});

module.exports = session({
    store: new PgSession({
        pool: pgPool,
        tableName: 'user_sessions',
        pruneSessionInterval: 60 * 15, // clean expired sessions every 15 min
        createTableIfMissing: true,
    }),
    secret: SESSION_SECRET,
    resave: false,
    saveUninitialized: false,
    name: 'ezhealth.sid',
    cookie: {
        secure: NODE_ENV === 'production' && process.env.SESSION_COOKIE_SECURE !== 'false',
        httpOnly: true,
        maxAge: 24 * 60 * 60 * 1000, // 24 hours
        sameSite: 'strict'
    }
});
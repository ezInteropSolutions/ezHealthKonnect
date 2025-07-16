const session = require('express-session');
const { SESSION_SECRET, NODE_ENV } = require('./environment');

module.exports = session({
    secret: SESSION_SECRET,
    resave: false,
    saveUninitialized: false,
    name: 'ezhealth.sid',
    cookie: { 
        secure: NODE_ENV === 'production',
        httpOnly: true,
        maxAge: 24 * 60 * 60 * 1000, // 24 hours
        sameSite: 'strict'
    }
});
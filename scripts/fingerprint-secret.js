#!/usr/bin/env node
// scripts/fingerprint-secret.js
//
// Adds a leaked or publicly-known secret value to config/compromised_secrets.json,
// so production refuses to start with it (Node: config/SecretsGuard.js,
// Go: services/secretguard). Only the SHA-256 fingerprint is stored.
//
// The value is read from stdin, never from argv, so it stays out of shell
// history and process listings:
//
//   node scripts/fingerprint-secret.js "dev compose JWT_SECRET (leaked 2026-10)" < value.txt
//   printf '%s' "$VALUE" | node scripts/fingerprint-secret.js "note"
//
// Values are trimmed. For a placeholder-style value, store its lowercase form so
// the case-insensitive match covers every casing.
'use strict';

const fs = require('fs');
const path = require('path');
const { sha256 } = require('../config/SecretsGuard');

const DEFAULT_FILE = path.join(__dirname, '..', 'config', 'compromised_secrets.json');

class CompromisedSecretRegistry {
    constructor(file = DEFAULT_FILE) {
        this.file = file;
        this.data = JSON.parse(fs.readFileSync(file, 'utf8'));
        this.data.fingerprints = this.data.fingerprints || [];
    }

    /** Adds a fingerprint. Returns false when the value is already listed. */
    add(value, note) {
        const v = (value || '').trim();
        if (!v) throw new Error('empty value - nothing to fingerprint');
        if (!note || !note.trim()) throw new Error('a note describing where the value leaked is required');
        const hash = sha256(v);
        if (this.data.fingerprints.some(f => f.sha256 === hash)) return false;
        this.data.fingerprints.push({ sha256: hash, note: note.trim() });
        return true;
    }

    save() {
        fs.writeFileSync(this.file, JSON.stringify(this.data, null, 2) + '\n');
    }
}

function readStdin() {
    return fs.readFileSync(0, 'utf8');
}

if (require.main === module) {
    const note = process.argv[2];
    try {
        if (process.stdin.isTTY) throw new Error('pipe the secret value on stdin (it is never accepted as an argument)');
        const registry = new CompromisedSecretRegistry();
        if (registry.add(readStdin(), note)) {
            registry.save();
            console.log(`Fingerprint added to ${path.relative(process.cwd(), registry.file)}. Commit it and rotate the secret.`);
        } else {
            console.log('Already listed - nothing changed.');
        }
    } catch (err) {
        console.error(`fingerprint-secret: ${err.message}`);
        console.error('usage: node scripts/fingerprint-secret.js "<where it leaked>" < file-with-value');
        process.exit(1);
    }
}

module.exports = { CompromisedSecretRegistry };

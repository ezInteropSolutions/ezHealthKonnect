#!/usr/bin/env node
// CI gate: fail on any known vulnerability of moderate severity or worse in the
// production dependencies, except the advisories listed in
// npm-audit-allowlist.json. Each exception names the advisory, why it is
// accepted, and an expiry date: after that date the gate fails again so the
// exception is looked at, not forgotten. Use only when no fixed version exists.
'use strict';
const { spawnSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const RANK = { info: 0, low: 1, moderate: 2, high: 3, critical: 4 };
const THRESHOLD = 'moderate';

// advisories collects the distinct advisories of an `npm audit --json` report.
function advisories(report) {
    const found = new Map();
    for (const v of Object.values((report && report.vulnerabilities) || {})) {
        for (const via of v.via || []) {
            if (typeof via !== 'object' || !via.url) continue; // a package name: reported through its own entry
            const id = String(via.url).split('/').pop();
            if (!found.has(id)) found.set(id, { id, package: via.name, severity: via.severity, title: via.title, url: via.url });
        }
    }
    return [...found.values()];
}

// evaluate returns what fails the gate and what is excepted.
function evaluate(report, allowlist, today) {
    const failures = [], excepted = [], expired = [];
    for (const a of advisories(report)) {
        if ((RANK[a.severity] ?? 0) < RANK[THRESHOLD]) continue;
        const rule = allowlist.find(r => r.id === a.id);
        if (!rule) { failures.push(a); continue; }
        if (rule.expires && today > rule.expires) { expired.push({ ...a, expires: rule.expires }); continue; }
        excepted.push({ ...a, reason: rule.reason, expires: rule.expires });
    }
    return { failures, excepted, expired };
}

function main() {
    const allowlist = JSON.parse(fs.readFileSync(path.join(__dirname, 'npm-audit-allowlist.json'), 'utf8'));
    const run = spawnSync('npm', ['audit', '--omit=dev', '--json'], { encoding: 'utf8', shell: process.platform === 'win32', maxBuffer: 64 << 20 });
    let report;
    try { report = JSON.parse(run.stdout); } catch (e) {
        console.error('npm audit gave no readable report:', (run.stderr || run.stdout || '').slice(0, 500));
        process.exit(1);
    }
    if (report.error) { console.error('npm audit failed:', report.error.summary || JSON.stringify(report.error)); process.exit(1); }
    const today = new Date().toISOString().slice(0, 10);
    const { failures, excepted, expired } = evaluate(report, allowlist, today);
    for (const a of excepted) console.log(`accepted until ${a.expires}: ${a.id} (${a.package}, ${a.severity}) - ${a.reason}`);
    for (const a of expired) console.error(`EXCEPTION EXPIRED ${a.expires}: ${a.id} (${a.package}, ${a.severity}) ${a.url} - check for a fixed version, or renew with a reason`);
    for (const a of failures) console.error(`VULNERABLE: ${a.id} (${a.package}, ${a.severity}) ${a.title} ${a.url}`);
    if (failures.length || expired.length) process.exit(1);
    console.log('npm audit gate passed');
}

if (require.main === module) main();
module.exports = { advisories, evaluate };

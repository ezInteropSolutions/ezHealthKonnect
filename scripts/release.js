#!/usr/bin/env node
'use strict';

/**
 * Release script: computes vYY.MM (or vYY.MM.N patch) from today's date,
 * shows commits since last tag, then tags and pushes to trigger the CI pipeline.
 *
 * Usage:
 *   npm run release            — tag and push
 *   npm run release -- --dry-run  — preview only, no tag created
 */

const { execSync } = require('child_process');

const DRY_RUN = process.argv.includes('--dry-run');

function run(cmd) {
  return execSync(cmd, { encoding: 'utf8' }).trim();
}

function tryRun(cmd) {
  try {
    return run(cmd);
  } catch {
    return '';
  }
}

function fail(msg) {
  console.error(`\nError: ${msg}\n`);
  process.exit(1);
}

// ── Guards ────────────────────────────────────────────────────────────────────

const branch = run('git branch --show-current');
if (branch !== 'main') {
  fail(`must be on main branch (you are on "${branch}")\nSwitch: git checkout main`);
}

const dirty = tryRun('git status --porcelain');
if (dirty) {
  fail(`uncommitted changes present — commit or stash first:\n${dirty}`);
}

// ── Version computation ───────────────────────────────────────────────────────

const now = new Date();
const yy = String(now.getFullYear()).slice(2);
const mm = String(now.getMonth() + 1).padStart(2, '0');
const base = `v${yy}.${mm}`;

const existingTags = new Set(tryRun('git tag').split('\n').filter(Boolean));

let version = base;
if (existingTags.has(base)) {
  let patch = 1;
  while (existingTags.has(`${base}.${patch}`)) patch++;
  version = `${base}.${patch}`;
}

// ── Changelog preview ─────────────────────────────────────────────────────────

const lastTag = tryRun('git describe --tags --abbrev=0');

console.log('\n── ezHealthKonnect Release ──────────────────────────────');
console.log(`  Version : ${version}`);
console.log(`  Branch  : ${branch}`);
if (lastTag) console.log(`  Since   : ${lastTag}`);
if (DRY_RUN) console.log('  Mode    : DRY RUN');
console.log('─────────────────────────────────────────────────────────');

if (lastTag) {
  const commits = tryRun(`git log ${lastTag}..HEAD --oneline --no-decorate`);
  if (commits) {
    console.log('\nCommits included in this release:');
    commits.split('\n').forEach(l => console.log(`  ${l}`));
  } else {
    console.log('\nNo commits since last tag.');
  }
} else {
  console.log('\n(first release — all commits included)');
}

console.log('');

// ── Dry run exit ──────────────────────────────────────────────────────────────

if (DRY_RUN) {
  console.log('Dry run complete — no tag created. Drop --dry-run to ship.');
  process.exit(0);
}

// ── Tag and push ──────────────────────────────────────────────────────────────

try {
  run(`git tag ${version}`);
} catch (e) {
  fail(`could not create tag ${version}: ${e.message}`);
}

try {
  run(`git push origin ${version}`);
} catch (e) {
  run(`git tag -d ${version}`); // roll back local tag
  fail(`push failed — tag removed locally.\n${e.message}`);
}

console.log(`Tag ${version} pushed.\n`);
console.log('GitHub Actions is building the release now.');
console.log('Check the Actions tab on GitHub to follow progress.');
console.log('');

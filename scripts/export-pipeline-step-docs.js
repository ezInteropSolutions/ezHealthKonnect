#!/usr/bin/env node
'use strict';

/**
 * Exports the pipeline builder's StepDocumentationRegistry content (the real,
 * continuously-maintained source of truth for "how does step X work") to a
 * JSON file the Go AI knowledge-ingestion service can read — see
 * services/ai/pipeline_step_docs_ingestion.go.
 *
 * The registry only exists as browser JS (public/js/pipeline/documentation/*.js,
 * loaded via <script> tags, self-registering into a class-level static map).
 * This script loads those same files into a Node vm context in the same order
 * pipeline-builder.html does, then serializes the resulting registry to JSON.
 *
 * Usage: npm run docs:export-step-docs
 */

const fs = require('fs');
const path = require('path');
const vm = require('vm');

const ROOT = path.join(__dirname, '..');
const PIPELINE_BUILDER_HTML = path.join(ROOT, 'public', 'pipeline-builder.html');
const DOCS_DIR = path.join(ROOT, 'public', 'js', 'pipeline', 'documentation');
const OUTPUT_PATH = path.join(ROOT, 'architecture', 'generated', 'pipeline_step_docs.json');

function fail(msg) {
  console.error(`\nError: ${msg}\n`);
  process.exit(1);
}

// Derive the file load order from pipeline-builder.html itself, rather than
// hardcoding it here — a newly-added category file is picked up automatically
// once it's wired into the HTML, same as it already needs to be for the
// browser to load it at all.
function findDocFileList() {
  const html = fs.readFileSync(PIPELINE_BUILDER_HTML, 'utf8');
  const re = /<script src="\/js\/pipeline\/documentation\/([A-Za-z0-9_]+\.js)(?:\?[^"]*)?"><\/script>/g;
  const files = [];
  let m;
  while ((m = re.exec(html)) !== null) {
    files.push(m[1]);
  }
  if (files.length === 0) {
    fail(`no /js/pipeline/documentation/*.js <script> tags found in ${PIPELINE_BUILDER_HTML}`);
  }
  if (files[0] !== 'StepDocumentationRegistry.js') {
    fail(`expected StepDocumentationRegistry.js to load first, found "${files[0]}" — the class definition must load before any category file registers into it`);
  }
  return files;
}

function loadRegistry(files) {
  const ctx = vm.createContext({});
  for (const file of files) {
    const filePath = path.join(DOCS_DIR, file);
    const source = fs.readFileSync(filePath, 'utf8');
    vm.runInContext(source, ctx, { filename: filePath });
  }
  // StepDocumentationRegistry is a top-level `class` declaration — a lexical
  // binding, not a property of the context object — so it must be bridged out
  // explicitly rather than read directly off ctx.
  vm.runInContext(
    'globalThis.__export = {' +
      'registry: StepDocumentationRegistry._registry, ' +
      'aliases: StepDocumentationRegistry._aliases' +
    '};',
    ctx
  );
  return ctx.__export;
}

function main() {
  const files = findDocFileList();
  console.log(`Loading ${files.length} documentation files in HTML order:\n  ${files.join('\n  ')}`);

  const { registry, aliases } = loadRegistry(files);
  const stepTypes = Object.keys(registry);
  const aliasNames = Object.keys(aliases);

  if (stepTypes.length === 0) {
    fail('StepDocumentationRegistry._registry is empty after loading all files — something is wrong with the vm bridge');
  }

  const output = {
    _generated_by: 'scripts/export-pipeline-step-docs.js',
    _generated_at: new Date().toISOString(),
    _source_files: files,
    registry,
    aliases,
  };

  fs.mkdirSync(path.dirname(OUTPUT_PATH), { recursive: true });
  fs.writeFileSync(OUTPUT_PATH, JSON.stringify(output, null, 2) + '\n', 'utf8');

  console.log(`\nWrote ${stepTypes.length} step types + ${aliasNames.length} aliases to:`);
  console.log(`  ${path.relative(ROOT, OUTPUT_PATH)}`);
}

main();

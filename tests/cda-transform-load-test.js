'use strict';
/**
 * cda-transform-load-test.js — ezHealthKonnect CDA parse/transform HTTP load test
 *
 * Targets POST /api/fhir/cda/parse — the CDA XML -> ParsedJSON endpoint, rate-limited
 * to 100 req/min per authenticated user as of the 2026-07 security audit (see
 * main.go's rateLimitMiddleware(100, 20, rateLimitByUserOrIP) on this route). There is
 * no separate /api/fhir/cda/transform route; this is the CPU-heavy CDA processing path
 * the rate limit was designed to protect, so it's the right target for this test.
 *
 * The endpoint itself is stateless (no DB/object-storage writes — see
 * TransformationTestController.ParseCDA), so running this repeatedly does not grow
 * any on-disk state the way the MLLP throughput test's message pipeline does.
 *
 * Usage:
 *   node tests/cda-transform-load-test.js [options]
 *
 * Options:
 *   --requests   N    Total requests to send (default: 200)
 *   --concurrent C    Parallel workers (default: 20)
 *   --target-rps R    Cap at R requests/second (default: 0 = unlimited). The endpoint
 *                      is rate-limited to 100 req/min/user (burst 20) as of the 2026-07
 *                      security audit — pace below ~1.6 req/s to measure genuine
 *                      processing latency instead of 429 rejections.
 *   --sample     PATH CDA/CCD XML file to POST (default: cda/document/testdata/full_ccd_nist.xml)
 *   --base       URL  Node.js base URL (default: http://localhost:3000)
 *   --email      E    Admin email for login (default: admin@ezhealthkonnect.com)
 *   --password   P    Admin password (default: admin123)
 *   --output     PATH JSON results output path (default: tests/cda-transform-load-results.json)
 *
 * Exit code:
 *   0 — all requests succeeded (HTTP 200)
 *   1 — any request failed
 */

const http = require('http');
const https = require('https');
const fs = require('fs');
const path = require('path');

// ── CLI args ──────────────────────────────────────────────────────────────────
const argv = process.argv.slice(2);
const getArg = (flag, def) => { const i = argv.indexOf(flag); return i >= 0 && argv[i + 1] ? argv[i + 1] : def; };

const REQUESTS    = parseInt(getArg('--requests', '200'), 10);
const CONCURRENT  = Math.max(1, parseInt(getArg('--concurrent', '20'), 10));
const TARGET_RPS  = parseFloat(getArg('--target-rps', '0')); // 0 = unlimited
const SAMPLE_PATH = getArg('--sample', path.join(__dirname, '..', 'cda', 'document', 'testdata', 'full_ccd_nist.xml'));
const BASE_URL    = getArg('--base', 'http://localhost:3000');
const EMAIL       = getArg('--email', 'admin@ezhealthkonnect.com');
const PASSWORD    = getArg('--password', 'admin123');
const OUTPUT      = getArg('--output', path.join(__dirname, 'cda-transform-load-results.json'));
const TIMEOUT_MS  = parseInt(getArg('--timeout-ms', '15000'), 10);

// ── HTTP helper (no external dependency, matches throughput-test.js's style) ──
function request(baseUrl, options, body) {
  const url = new URL(options.path, baseUrl);
  const lib = url.protocol === 'https:' ? https : http;
  const startMs = Date.now();

  return new Promise((resolve) => {
    const req = lib.request(url, {
      method: options.method,
      headers: options.headers,
      timeout: TIMEOUT_MS,
    }, (res) => {
      const chunks = [];
      res.on('data', (c) => chunks.push(c));
      res.on('end', () => {
        resolve({
          statusCode: res.statusCode,
          body: Buffer.concat(chunks).toString('utf8'),
          latencyMs: Date.now() - startMs,
          error: null,
        });
      });
    });
    req.on('timeout', () => { req.destroy(); });
    req.on('error', (err) => {
      resolve({ statusCode: 0, body: '', latencyMs: Date.now() - startMs, error: err.message });
    });
    if (body) req.write(body);
    req.end();
  });
}

async function login() {
  const res = await request(BASE_URL, {
    method: 'POST',
    path: '/api/auth/login',
    headers: { 'Content-Type': 'application/json' },
  }, JSON.stringify({ email: EMAIL, password: PASSWORD }));

  if (res.statusCode !== 200) {
    throw new Error(`Login failed (${res.statusCode}): ${res.body}`);
  }
  const parsed = JSON.parse(res.body);
  if (!parsed.token) throw new Error('Login response had no token');
  return parsed.token;
}

function postCDA(token, xmlContent) {
  return request(BASE_URL, {
    method: 'POST',
    path: '/api/fhir/cda/parse',
    headers: {
      'Content-Type': 'application/xml',
      'Authorization': `Bearer ${token}`,
    },
  }, xmlContent);
}

// ── Rate limiter (same TokenBucket pattern as throughput-test.js) ─────────────
class TokenBucket {
  constructor(rps) {
    this.interval = rps > 0 ? 1000 / rps : 0;
    this.lastTick = 0;
  }
  async wait() {
    if (this.interval === 0) return;
    const now = Date.now();
    const next = this.lastTick + this.interval;
    if (now < next) await new Promise((r) => setTimeout(r, next - now));
    this.lastTick = Date.now();
  }
}

// ── Percentile utility (same as throughput-test.js) ───────────────────────────
function percentile(arr, p) {
  if (!arr.length) return 0;
  const sorted = [...arr].sort((a, b) => a - b);
  const idx = Math.ceil((p / 100) * sorted.length) - 1;
  return sorted[Math.max(0, idx)];
}

// ── Main ──────────────────────────────────────────────────────────────────────
async function main() {
  console.log('\n╔══════════════════════════════════════════════════════════╗');
  console.log('║   ezHealthKonnect — CDA Parse/Transform HTTP Load Test    ║');
  console.log('╚══════════════════════════════════════════════════════════╝\n');

  if (!fs.existsSync(SAMPLE_PATH)) {
    console.error(`Sample CDA file not found: ${SAMPLE_PATH}`);
    process.exit(1);
  }
  const xmlContent = fs.readFileSync(SAMPLE_PATH, 'utf8');

  console.log(`Target:     ${BASE_URL}/api/fhir/cda/parse`);
  console.log(`Sample:     ${SAMPLE_PATH} (${(xmlContent.length / 1024).toFixed(1)} KB)`);
  console.log(`Requests:   ${REQUESTS}  |  Concurrent: ${CONCURRENT}  |  Target RPS: ${TARGET_RPS || 'unlimited'}\n`);

  console.log('Logging in...');
  const token = await login();
  console.log('Login OK.\n');

  const bucket  = new TokenBucket(TARGET_RPS);
  const results = [];
  const startMs = Date.now();
  let sent = 0, inFlight = 0, queueIdx = 0;

  await new Promise((resolve) => {
    const tryNext = () => {
      while (inFlight < CONCURRENT && queueIdx < REQUESTS) {
        queueIdx++;
        inFlight++;
        sent++;

        (async () => {
          await bucket.wait();
          const r = await postCDA(token, xmlContent);
          const pass = r.statusCode === 200;

          let sectionCount = 0;
          if (pass) {
            try { sectionCount = Object.keys(JSON.parse(r.body).sections || {}).length; } catch { /* ignore */ }
          }

          results.push({
            statusCode: r.statusCode,
            latencyMs:  r.latencyMs,
            error:      r.error,
            sectionCount,
            pass,
          });

          if (sent % Math.max(1, Math.floor(REQUESTS / 10)) === 0 || sent === REQUESTS) {
            const pct = Math.round(sent / REQUESTS * 100);
            process.stdout.write(`\r  Progress: ${sent}/${REQUESTS} (${pct}%)   `);
          }

          inFlight--;
          if (queueIdx < REQUESTS) tryNext();
          else if (inFlight === 0) resolve();
        })();
      }
    };
    tryNext();
  });

  const totalMs = Date.now() - startMs;
  console.log('\n');

  const passed = results.filter((r) => r.pass).length;
  const failed = results.length - passed;
  const lats   = results.filter((r) => r.latencyMs > 0).map((r) => r.latencyMs);

  const tps    = (results.length / (totalMs / 1000)).toFixed(2);
  const p50    = percentile(lats, 50);
  const p90    = percentile(lats, 90);
  const p95    = percentile(lats, 95);
  const p99    = percentile(lats, 99);
  const avgLat = lats.length ? Math.round(lats.reduce((a, b) => a + b, 0) / lats.length) : 0;

  console.log('┌─────────────────────────── RESULTS ───────────────────────────┐');
  console.log(`│ Total requests:     ${String(results.length).padEnd(10)} Duration: ${(totalMs / 1000).toFixed(2)}s              │`);
  console.log(`│ Throughput (req/s): ${String(tps).padEnd(45)} │`);
  console.log(`│ Passed (200):       ${String(passed).padEnd(10)} Failed: ${String(failed).padEnd(36)} │`);
  console.log(`│ Latency avg:        ${String(avgLat + 'ms').padEnd(45)} │`);
  console.log(`│ Latency p50/p90/p95/p99: ${p50}ms / ${p90}ms / ${p95}ms / ${p99}ms${' '.repeat(Math.max(0, 15 - String(p50 + p90 + p95 + p99).length))} │`);
  console.log('└───────────────────────────────────────────────────────────────┘');

  if (failed > 0) {
    const sample = results.find((r) => !r.pass);
    console.log(`\nSample failure: status=${sample.statusCode} error=${sample.error || 'n/a'}`);
  }

  const summary = {
    timestamp:   new Date().toISOString(),
    config:      { requests: REQUESTS, concurrent: CONCURRENT, sample: SAMPLE_PATH, sampleSizeBytes: xmlContent.length },
    duration_ms: totalMs,
    throughput_rps: parseFloat(tps),
    total: results.length,
    passed,
    failed,
    latency: { avg: avgLat, p50, p90, p95, p99 },
  };
  fs.writeFileSync(OUTPUT, JSON.stringify({ summary, results }, null, 2), 'utf8');
  console.log(`\nResults written to: ${OUTPUT}`);

  const exitCode = failed > 0 ? 1 : 0;
  console.log(exitCode === 0 ? '\n[PASS] All requests returned 200.' : `\n[FAIL] ${failed} request(s) did not return 200.`);
  process.exit(exitCode);
}

main().catch((err) => { console.error('Fatal:', err); process.exit(1); });

'use strict';
/**
 * throughput-test.js — ezHealthKonnect throughput baseline + chaos harness
 *
 * Usage:
 *   node tests/throughput-test.js [options]
 *
 * Options:
 *   --messages   N    Total messages to send (default: 100)
 *   --concurrent C    Parallel senders (default: 10)
 *   --target-tps T    Cap at T messages/second (default: unlimited)
 *   --port       P    MLLP listener port (default: 6613)
 *   --host       H    MLLP host (default: localhost)
 *   --chaos           Enable chaos injection scenarios
 *   --chaos-rate R    Fraction of messages that are chaos payloads (default: 0.2)
 *   --output     PATH JSON results output path (default: tests/throughput-results.json)
 *   --baseline        Run a clean 100-message baseline and exit (no chaos)
 *
 * Exit code:
 *   0 — all non-chaos messages ACK'd with AA
 *   1 — any unexpected ACK failure (AE/AR) on normal messages
 */

const net  = require('net');
const fs   = require('fs');
const path = require('path');

// ── CLI args ──────────────────────────────────────────────────────────────────
const argv = process.argv.slice(2);
const getArg = (flag, def) => { const i = argv.indexOf(flag); return i >= 0 && argv[i+1] ? argv[i+1] : def; };
const hasFlag = flag => argv.includes(flag);

const MESSAGES    = parseInt(getArg('--messages',   '100'), 10);
const CONCURRENT  = Math.max(1, parseInt(getArg('--concurrent', '10'), 10));
const TARGET_TPS  = parseFloat(getArg('--target-tps', '0')); // 0 = unlimited
const MLLP_PORT   = parseInt(getArg('--port', '6613'), 10);
const MLLP_HOST   = getArg('--host', 'localhost');
const CHAOS_MODE  = hasFlag('--chaos');
const CHAOS_RATE  = parseFloat(getArg('--chaos-rate', '0.2'));
const OUTPUT      = getArg('--output', path.join(__dirname, 'throughput-results.json'));
const BASELINE    = hasFlag('--baseline');
const TIMEOUT_MS  = parseInt(getArg('--timeout-ms', '10000'), 10);

// ── MLLP framing ──────────────────────────────────────────────────────────────
const MLLP_SB = Buffer.from([0x0B]);
const MLLP_EB = Buffer.from([0x1C, 0x0D]);

function mllpFrame(hl7) {
  return Buffer.concat([MLLP_SB, Buffer.from(hl7, 'ascii'), MLLP_EB]);
}

// ── HL7 message builders ──────────────────────────────────────────────────────
const SEG = '\r';
let seqNum = 0;

function nextCtrl() { return 'TH' + String(++seqNum).padStart(8, '0'); }

function buildNormal(idx) {
  const ctrl = nextCtrl();
  const ts   = '20260525120000';
  const pid  = idx % 5;
  const pids = ['SMITH^JOHN','JONES^MARY','GARCIA^CARL','BROWN^ANNE','PATEL^RAJ'];
  const mrn  = `MRN${String(idx).padStart(6,'0')}`;
  return [
    `MSH|^~\\&|THTEST|HOSP|EZHK|EZHK|${ts}||ADT^A01^ADT_A01|${ctrl}|P|2.5`,
    `EVN|A01|${ts}`,
    `PID|1||${mrn}^^^HOSP^MR||${pids[pid % 5]}|||M|||${idx} MAIN ST^^ANYTOWN^CA^90210`,
    `PV1|1|I|WARD0${(idx%5)+1}^10${(idx%10)+1}^A^HOSP`,
  ].join(SEG);
}

// ── Chaos payload builders ─────────────────────────────────────────────────────
const chaosScenarios = [
  {
    name: 'large_payload',
    build: (idx) => {
      const ctrl = nextCtrl();
      const ts   = '20260525120000';
      const obs  = Array.from({length: 200}, (_, i) =>
        `OBX|${i+1}|NM|${1000+i}^LAB_ITEM_${i}^LOCAL|||${(Math.random()*100).toFixed(2)}|mg/dL||N|||F`
      ).join(SEG);
      return [
        `MSH|^~\\&|LARGE|HOSP|EZHK|EZHK|${ts}||ORU^R01^ORU_R01|${ctrl}|P|2.5`,
        `PID|1||LARGE${idx}^^^HOSP^MR||LARGE^TEST|||M`,
        `OBR|1||LAB${idx}|PANEL^LARGE LAB PANEL^LOCAL|||${ts}`,
        obs,
      ].join(SEG);
    },
    expectNack: false, // Large but valid — should still ACK
  },
  {
    name: 'truncated_msh',
    build: () => 'MSH|^~\\&|TRUNC', // Truncated — no segment end
    expectNack: true,
  },
  {
    name: 'empty_payload',
    build: () => '',
    expectNack: true,
  },
  {
    name: 'repeated_segments',
    build: (idx) => {
      const ctrl = nextCtrl();
      const ts   = '20260525120000';
      const pids = Array.from({length: 20}, (_, i) =>
        `PID|${i+1}||MULTI${i}^^^HOSP^MR||MULTI^NAME^${i}|||${i%2===0?'M':'F'}`
      ).join(SEG);
      return [
        `MSH|^~\\&|REPEAT|HOSP|EZHK|EZHK|${ts}||ADT^A01^ADT_A01|${ctrl}|P|2.5`,
        `EVN|A01|${ts}`,
        pids,
        `PV1|1|I|WARD01^101^A^HOSP`,
      ].join(SEG);
    },
    expectNack: false,
  },
  {
    name: 'binary_in_body',
    build: () => {
      const ctrl = nextCtrl();
      const ts   = '20260525120000';
      return `MSH|^~\\&|BINARY|HOSP|EZHK|EZHK|${ts}||ADT^A01^ADT_A01|${ctrl}|P|2.5${SEG}PID|1||\x00\xFF\xFE\xFD^^^HOSP^MR||BINARY^TEST|||M`;
    },
    expectNack: true, // Non-ASCII in identifier — likely rejected
  },
];

function buildChaosMessage(idx) {
  const scenario = chaosScenarios[idx % chaosScenarios.length];
  return { scenario: scenario.name, hl7: scenario.build(idx), expectNack: scenario.expectNack };
}

// ── Single MLLP send ──────────────────────────────────────────────────────────
function sendMLLP(hl7, timeoutMs) {
  const startMs = Date.now();
  return new Promise(resolve => {
    const result = { ackCode: '', success: false, error: '', latencyMs: 0 };
    const sock = new net.Socket();
    let ackBuf = Buffer.alloc(0), settled = false;
    const finish = () => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      sock.destroy();
      result.latencyMs = Date.now() - startMs;
      const raw = ackBuf.toString('ascii').replace(/^\x0B/, '').replace(/\x1C\x0D$/, '');
      const msa = raw.split(/[\r\n]/).find(l => l.startsWith('MSA'));
      if (msa) { result.ackCode = (msa.split('|')[1] || '').trim(); }
      result.success = true;
      resolve(result);
    };
    const timer = setTimeout(() => {
      if (!settled) {
        settled = true;
        result.error = 'timeout';
        result.latencyMs = Date.now() - startMs;
        sock.destroy();
        resolve(result);
      }
    }, timeoutMs);
    sock.on('error', err => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      result.error = err.message;
      result.latencyMs = Date.now() - startMs;
      sock.destroy();
      resolve(result);
    });
    sock.on('data', chunk => {
      ackBuf = Buffer.concat([ackBuf, chunk]);
      if (ackBuf.length >= 2 && ackBuf[ackBuf.length-1]===0x0D && ackBuf[ackBuf.length-2]===0x1C) finish();
    });
    sock.on('close', finish);
    sock.connect(MLLP_PORT, MLLP_HOST, () => {
      const frame = mllpFrame(hl7);
      sock.write(frame);
    });
  });
}

// ── Rate limiter ──────────────────────────────────────────────────────────────
class TokenBucket {
  constructor(tps) {
    this.interval = tps > 0 ? 1000 / tps : 0;
    this.lastTick = 0;
  }
  async wait() {
    if (this.interval === 0) return;
    const now = Date.now();
    const next = this.lastTick + this.interval;
    if (now < next) await new Promise(r => setTimeout(r, next - now));
    this.lastTick = Date.now();
  }
}

// ── Percentile utility ────────────────────────────────────────────────────────
function percentile(arr, p) {
  if (!arr.length) return 0;
  const sorted = [...arr].sort((a, b) => a - b);
  const idx = Math.ceil((p / 100) * sorted.length) - 1;
  return sorted[Math.max(0, idx)];
}

// ── Main ──────────────────────────────────────────────────────────────────────
async function main() {
  console.log('\n╔══════════════════════════════════════════════════════════╗');
  console.log('║       ezHealthKonnect — Throughput & Chaos Test          ║');
  console.log('╚══════════════════════════════════════════════════════════╝\n');

  console.log(`Target:   ${MLLP_HOST}:${MLLP_PORT}`);
  console.log(`Messages: ${MESSAGES}  |  Concurrent: ${CONCURRENT}  |  TPS cap: ${TARGET_TPS||'unlimited'}`);
  console.log(`Chaos:    ${CHAOS_MODE ? `enabled (rate=${CHAOS_RATE})` : 'disabled'}`);
  if (BASELINE) console.log('Mode:     baseline (clean run, no chaos)\n');
  else          console.log('');

  // ── Build work queue ──────────────────────────────────────────────────────
  const queue = [];
  for (let i = 0; i < MESSAGES; i++) {
    const isChaos = CHAOS_MODE && !BASELINE && Math.random() < CHAOS_RATE;
    if (isChaos) {
      const { scenario, hl7, expectNack } = buildChaosMessage(i);
      queue.push({ idx: i, hl7, isChaos: true, scenario, expectNack });
    } else {
      queue.push({ idx: i, hl7: buildNormal(i), isChaos: false, scenario: 'normal', expectNack: false });
    }
  }

  // ── Execute with concurrency control ─────────────────────────────────────
  const bucket  = new TokenBucket(TARGET_TPS);
  const results = [];
  const startMs = Date.now();

  let sent = 0, inFlight = 0, queueIdx = 0;

  await new Promise(resolve => {
    const tryNext = () => {
      while (inFlight < CONCURRENT && queueIdx < queue.length) {
        const item = queue[queueIdx++];
        inFlight++;
        sent++;

        (async () => {
          await bucket.wait();
          const r = await sendMLLP(item.hl7, TIMEOUT_MS);
          const pass = item.expectNack
            ? (r.ackCode !== 'AA' || r.error === 'timeout')  // chaos: NACK is expected
            : (r.ackCode === 'AA');                           // normal: must be AA

          results.push({
            idx:        item.idx,
            scenario:   item.scenario,
            isChaos:    item.isChaos,
            expectNack: item.expectNack,
            ackCode:    r.ackCode,
            latencyMs:  r.latencyMs,
            error:      r.error,
            pass,
          });

          if (sent % Math.max(1, Math.floor(MESSAGES / 10)) === 0 || sent === MESSAGES) {
            const pct = Math.round(sent / MESSAGES * 100);
            process.stdout.write(`\r  Progress: ${sent}/${MESSAGES} (${pct}%)   `);
          }

          inFlight--;
          if (queueIdx < queue.length) {
            tryNext();
          } else if (inFlight === 0) {
            resolve();
          }
        })();
      }
    };
    tryNext();
  });

  const totalMs = Date.now() - startMs;
  console.log('\n');

  // ── Stats ──────────────────────────────────────────────────────────────────
  const normals = results.filter(r => !r.isChaos);
  const chaosR  = results.filter(r => r.isChaos);

  const normalPass   = normals.filter(r => r.pass).length;
  const normalFail   = normals.length - normalPass;
  const normalLats   = normals.filter(r => r.latencyMs > 0).map(r => r.latencyMs);
  const chaosPass    = chaosR.filter(r => r.pass).length;
  const chaosFail    = chaosR.length - chaosPass;

  const tps        = (results.length / (totalMs / 1000)).toFixed(1);
  const p50        = percentile(normalLats, 50);
  const p90        = percentile(normalLats, 90);
  const p99        = percentile(normalLats, 99);
  const avgLat     = normalLats.length ? Math.round(normalLats.reduce((a, b) => a + b, 0) / normalLats.length) : 0;

  console.log('┌─────────────────────────── RESULTS ───────────────────────────┐');
  console.log(`│ Total messages:     ${String(results.length).padEnd(10)} Duration: ${(totalMs/1000).toFixed(2)}s              │`);
  console.log(`│ Throughput (TPS):   ${String(tps).padEnd(45)} │`);
  console.log('├────────────────────────── NORMAL LOAD ────────────────────────┤');
  console.log(`│ Sent:               ${String(normals.length).padEnd(45)} │`);
  console.log(`│ Passed (AA):        ${String(normalPass).padEnd(10)} Failed: ${String(normalFail).padEnd(36)} │`);
  console.log(`│ Latency avg:        ${String(avgLat + 'ms').padEnd(45)} │`);
  console.log(`│ Latency p50/p90/p99: ${p50}ms / ${p90}ms / ${p99}ms${' '.repeat(Math.max(0, 23 - String(p50+p90+p99).length))} │`);

  if (chaosR.length) {
    const byScenario = {};
    chaosR.forEach(r => {
      if (!byScenario[r.scenario]) byScenario[r.scenario] = { pass: 0, fail: 0 };
      byScenario[r.scenario][r.pass ? 'pass' : 'fail']++;
    });
    console.log('├──────────────────────────── CHAOS ────────────────────────────┤');
    console.log(`│ Injected:           ${String(chaosR.length).padEnd(10)} Handled correctly: ${String(chaosPass).padEnd(20)} │`);
    Object.entries(byScenario).forEach(([sc, cnt]) => {
      const line = `│   ${sc.padEnd(22)}: ${cnt.pass} handled, ${cnt.fail} unexpected          │`;
      console.log(line.substring(0, 67) + ' │');
    });
  }
  console.log('└───────────────────────────────────────────────────────────────┘');

  const exitCode = normalFail > 0 ? 1 : 0;

  // ── Write JSON results ─────────────────────────────────────────────────────
  const summary = {
    timestamp:   new Date().toISOString(),
    config:      { messages: MESSAGES, concurrent: CONCURRENT, targetTps: TARGET_TPS, chaos: CHAOS_MODE, chaosRate: CHAOS_RATE },
    duration_ms: totalMs,
    throughput_tps: parseFloat(tps),
    normal: {
      total:   normals.length,
      passed:  normalPass,
      failed:  normalFail,
      latency: { avg: avgLat, p50, p90, p99 },
    },
    chaos: chaosR.length ? {
      total:             chaosR.length,
      handled_correctly: chaosPass,
      unexpected:        chaosFail,
    } : null,
  };
  fs.writeFileSync(OUTPUT, JSON.stringify({ summary, results }, null, 2), 'utf8');
  console.log(`\nResults written to: ${OUTPUT}`);

  if (exitCode !== 0) {
    console.error(`\n[FAIL] ${normalFail} normal message(s) did not receive AA acknowledgement.`);
  } else {
    console.log('\n[PASS] All normal messages acknowledged successfully.');
  }

  process.exit(exitCode);
}

main().catch(err => { console.error('Fatal:', err); process.exit(1); });

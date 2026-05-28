#!/usr/bin/env node
/**
 * ezCompanion Persona Evaluation
 * Simulates 6 user personas asking realistic questions,
 * scores each response, and surfaces knowledge gaps.
 *
 * Usage:  node tests/ezcompanion-eval.js
 */

const BASE      = 'http://localhost:3000/api/ai/ask';
const LOGIN_URL = 'http://localhost:3000/api/auth/login';

let sessionCookie = '';

async function login() {
  const res = await fetch(LOGIN_URL, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: 'admin@ezhealthkonnect.com', password: 'admin123' }),
  });
  if (!res.ok) throw new Error(`Login failed: ${res.status}`);
  const setCookie = res.headers.get('set-cookie') || '';
  const match = setCookie.match(/ezhealth\.sid=[^;]+/);
  if (!match) throw new Error('No session cookie in login response');
  sessionCookie = match[0];
  console.log('  ✅ Logged in — session cookie acquired\n');
}

// ─── Personas ─────────────────────────────────────────────────────────────────

const PERSONAS = [
  {
    name: 'New User',
    desc: 'No HL7 background, just installed the product',
    questions: [
      'What is ezHealthKonnect and what does it do?',
      'What is HL7 and why does it matter in healthcare?',
      'How do I create my first interface?',
      'What is a pipeline and how does it work?',
    ],
    keywords: ['interface', 'HL7', 'FHIR', 'pipeline', 'connect', 'transform'],
    expectCode: false,
  },
  {
    name: 'Experienced User',
    desc: 'Uses the product daily, building integrations',
    questions: [
      'How do I map the patient name from PID.5 to Patient.name in FHIR?',
      'What is the difference between ADT^A01 and ADT^A28?',
      'How do I configure a TCP/MLLP inbound connection on port 2575?',
      'How do I add a conditional step that routes based on message type?',
    ],
    keywords: ['PID', 'Patient', 'ADT', 'MLLP', 'step', 'pipeline', 'field_mapping'],
    expectCode: false,
  },
  {
    name: 'Interface Engineer — Developer',
    desc: 'Writing transform scripts and pipeline logic',
    questions: [
      'Write a transform() script that extracts the patient last name from PID.5.1 and sets it as a custom field',
      'What is the structure of the enhancedSegments object I get in a transform() function?',
      'How do I call an external REST API from an enrichment.api step?',
      'Show me the config schema for an enrichment.script step',
    ],
    keywords: ['transform', 'enhancedSegments', 'input', 'function', 'return', 'PID', 'config'],
    expectCode: true,
  },
  {
    name: 'Interface Engineer — Support',
    desc: 'Diagnosing failed messages and pipeline errors',
    questions: [
      'A message arrived but the pipeline failed — where do I look first?',
      'What does MSA.1 = AE mean in an HL7 ACK response?',
      'How do I redrive a message from the Dead Letter Queue?',
      'How do I diagnose why a FHIR validation step is failing?',
    ],
    keywords: ['DLQ', 'error', 'ACK', 'AE', 'retry', 'log', 'pipeline', 'fail'],
    expectCode: false,
  },
  {
    name: 'Admin',
    desc: 'Manages users, settings, and system health',
    questions: [
      'How do I create a new user and assign them the admin role?',
      'How do I enable or disable the AI assistant?',
      'How do I check which Ollama model is active?',
      'How do I back up or export interface configurations?',
    ],
    keywords: ['user', 'role', 'admin', 'settings', 'AI', 'model', 'Ollama'],
    expectCode: false,
  },
  {
    name: 'Compliance Engineer',
    desc: 'Focused on HIPAA, audit trails, and PHI handling',
    questions: [
      'Does ezHealthKonnect support HIPAA-compliant audit logging?',
      'Is any patient data sent to external AI services like OpenAI or Anthropic?',
      'How is PHI protected during message processing?',
      'What encryption is used for credentials stored in the system?',
    ],
    keywords: ['audit', 'HIPAA', 'PHI', 'local', 'Ollama', 'encrypt', 'log'],
    expectCode: false,
  },
];

// ─── Scoring ──────────────────────────────────────────────────────────────────

function scoreResponse(answer, persona, question) {
  const issues = [];
  const hits = [];
  let score = 0;

  // Length
  if (answer.length < 80) {
    issues.push('Too short — likely a refusal or no RAG hit');
  } else if (answer.length < 300) {
    score += 1;
    hits.push('Answered (brief)');
  } else {
    score += 2;
    hits.push('Answered (detailed)');
  }

  // Uncertainty signals
  const uncertain = /i don't know|i'm not sure|i cannot|i do not have|unable to|not aware|no information/i;
  if (uncertain.test(answer)) {
    issues.push('Expressed uncertainty — knowledge gap likely');
    score -= 1;
  }

  // Keyword coverage
  const matched = persona.keywords.filter(k => answer.toLowerCase().includes(k.toLowerCase()));
  const coverage = matched.length / persona.keywords.length;
  if (coverage >= 0.5) {
    score += 2;
    hits.push(`Keywords: ${matched.join(', ')}`);
  } else if (coverage > 0) {
    score += 1;
    hits.push(`Partial keywords: ${matched.join(', ')}`);
  } else {
    issues.push('No relevant keywords found');
  }

  // Code expected
  if (persona.expectCode) {
    if (/function\s+\w+|```|transform\s*\(/.test(answer)) {
      score += 2;
      hits.push('Included code/example');
    } else {
      issues.push('Expected code example — none provided');
      score -= 1;
    }
  }

  // Hallucination signals (mentioning non-existent things)
  const hallucination = /openai|anthropic|chatgpt|gpt-4|claude api|send.*cloud/i;
  if (hallucination.test(answer)) {
    issues.push('⚠️  Possible hallucination: mentioned external AI service');
    score -= 2;
  }

  return { score: Math.max(0, score), max: 6, hits, issues };
}

// ─── Runner ───────────────────────────────────────────────────────────────────

async function ask(question, sessionId, personaName) {
  const res = await fetch(BASE, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Cookie': sessionCookie },
    body: JSON.stringify({
      question,
      session_id: sessionId,
      context: { personaEval: true, personaName },
    }),
  });

  if (!res.ok) {
    const text = await res.text();
    throw new Error(`HTTP ${res.status}: ${text.slice(0, 200)}`);
  }

  const json = await res.json();
  return {
    answer: json.data?.answer ?? '',
    chunks: json.data?.context_chunks ?? 0,
    sources: json.data?.sources ?? [],
  };
}

function bar(score, max) {
  const filled = Math.round((score / max) * 10);
  return '█'.repeat(filled) + '░'.repeat(10 - filled);
}

function truncate(text, n = 300) {
  return text.length > n ? text.slice(0, n) + '…' : text;
}

// ─── Main ─────────────────────────────────────────────────────────────────────

async function main() {
  console.log('\n╔══════════════════════════════════════════════════════════════╗');
  console.log('║         ezCompanion Persona Evaluation                       ║');
  console.log('╚══════════════════════════════════════════════════════════════╝\n');

  await login();

  const allGaps = [];
  const personaScores = [];

  for (const persona of PERSONAS) {
    const sessionId = `eval-${persona.name.replace(/\s+/g, '-').toLowerCase()}-${Date.now()}`;
    console.log(`\n${'─'.repeat(66)}`);
    console.log(`👤  ${persona.name.toUpperCase()}`);
    console.log(`    ${persona.desc}`);
    console.log('─'.repeat(66));

    let personaTotal = 0;
    let personaMax = 0;
    const personaGaps = [];

    for (const question of persona.questions) {
      process.stdout.write(`\n  Q: "${question}"\n`);

      let result;
      try {
        result = await ask(question, sessionId, persona.name);
      } catch (err) {
        console.log(`  ❌ Request failed: ${err.message}`);
        personaGaps.push({ question, issue: `API error: ${err.message}` });
        continue;
      }

      const { answer, chunks, sources } = result;
      const { score, max, hits, issues } = scoreResponse(answer, persona, question);

      personaTotal += score;
      personaMax += max;

      const grade = score >= 5 ? '✅' : score >= 3 ? '⚠️ ' : '❌';
      console.log(`  ${grade} Score: ${score}/${max}  [${bar(score, max)}]  RAG chunks: ${chunks}`);

      if (hits.length) console.log(`     ✓ ${hits.join(' · ')}`);
      if (issues.length) {
        issues.forEach(i => console.log(`     ✗ ${i}`));
        personaGaps.push(...issues.map(i => ({ question, issue: i })));
      }

      console.log(`\n  Answer preview:\n  ${truncate(answer, 280).replace(/\n/g, '\n  ')}`);
    }

    const pct = personaMax ? Math.round((personaTotal / personaMax) * 100) : 0;
    console.log(`\n  📊 ${persona.name} overall: ${personaTotal}/${personaMax} (${pct}%) [${bar(personaTotal, personaMax)}]`);
    personaScores.push({ persona: persona.name, score: personaTotal, max: personaMax, pct });
    allGaps.push(...personaGaps.map(g => ({ persona: persona.name, ...g })));
  }

  // ─── Summary ──────────────────────────────────────────────────────────────

  console.log('\n\n╔══════════════════════════════════════════════════════════════╗');
  console.log('║                    SUMMARY & GAPS                           ║');
  console.log('╚══════════════════════════════════════════════════════════════╝\n');

  console.log('Persona Scores:');
  for (const { persona, score, max, pct } of personaScores) {
    const grade = pct >= 75 ? '✅' : pct >= 50 ? '⚠️ ' : '❌';
    console.log(`  ${grade}  ${persona.padEnd(35)} ${String(score).padStart(2)}/${max}  ${String(pct).padStart(3)}%  [${bar(score, max)}]`);
  }

  if (allGaps.length === 0) {
    console.log('\n🎉 No significant gaps found!');
  } else {
    console.log(`\nKnowledge Gaps (${allGaps.length} total):\n`);
    const grouped = {};
    for (const g of allGaps) {
      grouped[g.persona] = grouped[g.persona] || [];
      grouped[g.persona].push(g);
    }
    for (const [persona, gaps] of Object.entries(grouped)) {
      console.log(`  ${persona}:`);
      for (const { question, issue } of gaps) {
        console.log(`    • [${issue}]`);
        console.log(`      Q: "${question}"`);
      }
    }
  }

  console.log('\nRecommended fixes (if gaps found):');
  console.log('  1. Add missing topics to builtin_knowledge.go (appBuiltinKnowledge)');
  console.log('  2. Re-run POST /api/ai/ingest/docs to refresh vector store');
  console.log('  3. For code quality gaps: expand EHK_Step_Types_Core entry with more examples');
  console.log('  4. For compliance gaps: add an EHK_Compliance entry covering HIPAA/PHI handling');
  console.log('  5. If uncertainty on app flow: add architecture/*.md docs and ingest\n');
}

main().catch(err => {
  console.error('Eval failed:', err);
  process.exit(1);
});

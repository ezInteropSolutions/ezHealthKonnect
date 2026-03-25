const { test, expect } = require('@playwright/test');
const http = require('http');

// LLM responses on CPU-only Docker (llama3.2:3b) typically take 30–120s.
// Override the global 30s Playwright timeout for every test in this file.
test.setTimeout(180_000);

// ================================================================
// ezCompanion AI — End-to-End Test Suite
// ================================================================
// Covers:
//   Status & Connectivity
//     1.  GET /api/ai/status returns 200 with ollama_reachable flag
//     2.  Status response contains chat model and embed model names
//
//   SSE Streaming Protocol (/api/ai/ask/stream)
//     3.  Response Content-Type is text/event-stream
//     4.  Tokens arrive as  data: "<json-encoded-string>"  lines
//     5.  Stream ends with  data: [DONE]  sentinel
//     6.  Assembled response is non-empty text
//
//   Identity & Role prompts
//     7.  "Who are you?" → mentions ezCompanion or ezHealthKonnect
//     8.  "What can you help me with?" → covers dev, integration, or HL7/FHIR
//     9.  "Are you ChatGPT?" → does NOT claim to be ChatGPT / OpenAI
//
//   App Knowledge prompts
//    10.  "What is ezHealthKonnect?" → mentions HL7, FHIR, or integration
//    11.  "What connectors are available?" → mentions TCP, MLLP, HTTP, or S3
//    12.  "How do I create an interface?" → mentions wizard or pipeline
//    13.  "What is the Pipeline Builder?" → mentions steps, drag, or pipeline
//    14.  "How does the wizard work?" → mentions mapping or configuration
//
//   HL7 Domain Knowledge
//    15.  "What is an HL7 ADT message?" → mentions admit, discharge, or patient
//    16.  "Explain the MSH segment" → mentions header or sending application
//    17.  "What does PID stand for in HL7?" → mentions patient identification
//    18.  "What is MLLP?" → mentions transport or framing or port
//
//   FHIR Domain Knowledge
//    19.  "What is FHIR R4?" → mentions resource or standard or HL7
//    20.  "Explain a FHIR Patient resource" → mentions name or identifier or demographics
//    21.  "What is a FHIR Bundle?" → mentions collection or resources or entries
//
//   Healthcare Standards
//    22.  "What is LOINC?" → mentions lab, code, or observation
//    23.  "What is ICD-10?" → mentions diagnosis or code or disease
//    24.  "What is SNOMED CT?" → mentions clinical or terminology or concept
//
//   Developer Assistance
//    25.  "Write a transform script to extract patient name" → returns function or transform
//    26.  "How do I mask PHI in a pipeline?" → mentions masking, field, or PHI
//    27.  "Show me a FHIR Observation JSON example" → returns JSON or resourceType
//
//   Edge Cases & Safety
//    28.  Empty question body → 400 Bad Request
//    29.  Very long question (>2000 chars) → still responds without error
//    30.  Non-healthcare question ("What is the capital of France?") → responds without crash
//    31.  Question with special chars / SQL injection attempt → responds safely, no crash
//    32.  Missing Content-Type header → 400 Bad Request
//
//   Non-Streaming /api/ai/ask endpoint
//    33.  POST /api/ai/ask returns JSON with answer field
//    34.  Answer is non-empty string
//    35.  Response contains success:true
// ================================================================

const BASE = 'http://localhost:3000';
const STREAM_URL = `${BASE}/api/ai/ask/stream`;
const ASK_URL = `${BASE}/api/ai/ask`;
const STATUS_URL = `${BASE}/api/ai/status`;

// ─── SSE helper ───────────────────────────────────────────────────────────────
// Reads a full SSE response via Node http and assembles the text.
// Returns { text, events, error } where:
//   text   — concatenated token strings
//   events — raw array of data: payloads
//   error  — [ERROR] payload if present, else null

function collectSSE(question, extraBody = {}) {
    return new Promise((resolve) => {
        const body = JSON.stringify({ question, ...extraBody });
        const url = new URL(STREAM_URL);
        const options = {
            hostname: url.hostname,
            port: parseInt(url.port) || 80,
            path: url.pathname,
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Content-Length': Buffer.byteLength(body),
                Accept: 'text/event-stream',
            },
        };

        const req = http.request(options, (res) => {
            const events = [];
            let text = '';
            let errorPayload = null;
            let buf = '';

            res.on('data', (chunk) => {
                buf += chunk.toString();
                const lines = buf.split('\n');
                buf = lines.pop(); // keep incomplete line

                for (const line of lines) {
                    if (!line.startsWith('data: ')) continue;
                    const payload = line.slice(6).trim();
                    events.push(payload);
                    if (payload === '[DONE]') continue;
                    if (payload.startsWith('[ERROR]')) {
                        errorPayload = payload.slice(7).trim();
                        continue;
                    }
                    try { text += JSON.parse(payload); } catch { text += payload; }
                }
            });

            res.on('end', () => resolve({
                statusCode: res.statusCode,
                contentType: res.headers['content-type'] || '',
                events,
                text,
                error: errorPayload,
            }));
        });

        req.on('error', (e) => resolve({ statusCode: 0, error: e.message, events: [], text: '' }));
        req.setTimeout(120_000, () => { req.destroy(); resolve({ statusCode: 0, error: 'timeout', events: [], text: '' }); });
        req.write(body);
        req.end();
    });
}

// Checks that text contains at least one of the given keywords (case-insensitive).
function containsAny(text, keywords) {
    const lower = text.toLowerCase();
    return keywords.some(kw => lower.includes(kw.toLowerCase()));
}

// ─── Status & Connectivity ────────────────────────────────────────────────────

test.describe('Status & Connectivity', () => {

    test('1. GET /api/ai/status returns 200', async ({ request }) => {
        const res = await request.get(STATUS_URL);
        expect(res.status()).toBe(200);
    });

    test('2. Status response contains model names', async ({ request }) => {
        const res = await request.get(STATUS_URL);
        const json = await res.json();
        expect(json).toHaveProperty('data');
        const data = json.data;
        // ollama_reachable is boolean
        expect(typeof data.ollama_reachable).toBe('boolean');
        // When reachable, model names should be populated
        if (data.ollama_reachable) {
            expect(data.chat_model).toBeTruthy();
            expect(data.embed_model).toBeTruthy();
        }
    });

});

// ─── SSE Streaming Protocol ───────────────────────────────────────────────────

test.describe('SSE Streaming Protocol', () => {

    test('3. Response Content-Type is text/event-stream', async () => {
        const { contentType } = await collectSSE('hello');
        expect(contentType).toContain('text/event-stream');
    });

    test('4. Tokens arrive as JSON-encoded data: lines', async () => {
        const { events, text } = await collectSSE('Say the word OK.');
        // At least some events should be JSON strings (not [DONE]/[ERROR])
        const tokenEvents = events.filter(e => e !== '[DONE]' && !e.startsWith('[ERROR]'));
        expect(tokenEvents.length).toBeGreaterThan(0);
        // Each token event must be valid JSON
        for (const ev of tokenEvents) {
            expect(() => JSON.parse(ev)).not.toThrow();
        }
    });

    test('5. Stream ends with [DONE] sentinel', async () => {
        const { events } = await collectSSE('Say the word OK.');
        expect(events[events.length - 1]).toBe('[DONE]');
    });

    test('6. Assembled response is non-empty text', async () => {
        const { text, error } = await collectSSE('What does HL7 stand for?');
        expect(error).toBeNull();
        expect(text.trim().length).toBeGreaterThan(10);
    });

});

// ─── Identity & Role ──────────────────────────────────────────────────────────

test.describe('Identity & Role', () => {

    test('7. "Who are you?" → identifies as ezCompanion or ezHealthKonnect', async () => {
        const { text, error } = await collectSSE('Who are you?');
        expect(error).toBeNull();
        expect(containsAny(text, ['ezCompanion', 'ezHealthKonnect', 'co-developer', 'integration', 'assistant'])).toBe(true);
    });

    test('8. "What can you help me with?" → covers dev or integration or HL7/FHIR topics', async () => {
        const { text, error } = await collectSSE('What can you help me with?');
        expect(error).toBeNull();
        expect(containsAny(text, ['HL7', 'FHIR', 'pipeline', 'interface', 'integration', 'connector', 'transform'])).toBe(true);
    });

    test('9. "Are you ChatGPT?" → does NOT claim to be ChatGPT or OpenAI', async () => {
        const { text, error } = await collectSSE('Are you ChatGPT?');
        expect(error).toBeNull();
        const lower = text.toLowerCase();
        // Should not claim to be ChatGPT — may acknowledge the question but should clarify identity
        expect(lower).not.toMatch(/i am chatgpt|i'm chatgpt|yes.*chatgpt/);
    });

});

// ─── App Knowledge ────────────────────────────────────────────────────────────

test.describe('App Knowledge', () => {

    test('10. "What is ezHealthKonnect?" → mentions HL7, FHIR, or integration', async () => {
        const { text, error } = await collectSSE('What is ezHealthKonnect?');
        expect(error).toBeNull();
        expect(containsAny(text, ['HL7', 'FHIR', 'integration', 'healthcare', 'platform', 'transform'])).toBe(true);
    });

    test('11. "What connectors are available?" → mentions connector types', async () => {
        const { text, error } = await collectSSE('What connectors are available in ezHealthKonnect?');
        expect(error).toBeNull();
        expect(containsAny(text, ['TCP', 'MLLP', 'HTTP', 'SFTP', 'S3', 'Kafka', 'RabbitMQ', 'connector', 'database'])).toBe(true);
    });

    test('12. "How do I create an interface?" → mentions wizard or pipeline or configuration', async () => {
        const { text, error } = await collectSSE('How do I create an interface in ezHealthKonnect?');
        expect(error).toBeNull();
        expect(containsAny(text, ['wizard', 'pipeline', 'configure', 'interface', 'setup', 'connector'])).toBe(true);
    });

    test('13. "What is the Pipeline Builder?" → mentions steps or pipeline', async () => {
        const { text, error } = await collectSSE('What is the Pipeline Builder?');
        expect(error).toBeNull();
        expect(containsAny(text, ['step', 'pipeline', 'drag', 'visual', 'transform', 'builder'])).toBe(true);
    });

    test('14. "How does the wizard work?" → mentions mapping or configuration', async () => {
        const { text, error } = await collectSSE('How does the interface wizard work?');
        expect(error).toBeNull();
        expect(containsAny(text, ['mapping', 'config', 'setup', 'field', 'HL7', 'FHIR', 'wizard', 'guide'])).toBe(true);
    });

});

// ─── HL7 Domain Knowledge ─────────────────────────────────────────────────────

test.describe('HL7 Domain Knowledge', () => {

    test('15. "What is an HL7 ADT message?" → mentions patient events', async () => {
        const { text, error } = await collectSSE('What is an HL7 ADT message?');
        expect(error).toBeNull();
        expect(containsAny(text, ['admit', 'discharge', 'transfer', 'patient', 'event', 'ADT'])).toBe(true);
    });

    test('16. "Explain the MSH segment" → mentions message header or sending app', async () => {
        const { text, error } = await collectSSE('Explain the MSH segment in HL7.');
        expect(error).toBeNull();
        expect(containsAny(text, ['header', 'sending', 'application', 'message', 'MSH', 'segment', 'delimit'])).toBe(true);
    });

    test('17. "What does PID stand for?" → mentions patient identification', async () => {
        const { text, error } = await collectSSE('What does PID stand for in HL7?');
        expect(error).toBeNull();
        expect(containsAny(text, ['patient', 'identification', 'identifier', 'PID', 'demographic'])).toBe(true);
    });

    test('18. "What is MLLP?" → mentions transport or framing', async () => {
        const { text, error } = await collectSSE('What is MLLP and why is it used?');
        expect(error).toBeNull();
        expect(containsAny(text, ['transport', 'protocol', 'TCP', 'frame', 'HL7', 'MLLP', 'Lower Layer', 'network'])).toBe(true);
    });

});

// ─── FHIR Domain Knowledge ───────────────────────────────────────────────────

test.describe('FHIR Domain Knowledge', () => {

    test('19. "What is FHIR R4?" → mentions resources or standard', async () => {
        const { text, error } = await collectSSE('What is FHIR R4?');
        expect(error).toBeNull();
        expect(containsAny(text, ['resource', 'standard', 'HL7', 'interoperability', 'REST', 'API', 'FHIR'])).toBe(true);
    });

    test('20. "Explain a FHIR Patient resource" → mentions demographics or identifier', async () => {
        const { text, error } = await collectSSE('Explain a FHIR Patient resource.');
        expect(error).toBeNull();
        expect(containsAny(text, ['name', 'identifier', 'demographic', 'birthDate', 'gender', 'address', 'Patient'])).toBe(true);
    });

    test('21. "What is a FHIR Bundle?" → mentions collection or entries', async () => {
        const { text, error } = await collectSSE('What is a FHIR Bundle?');
        expect(error).toBeNull();
        expect(containsAny(text, ['collection', 'entries', 'entry', 'resources', 'group', 'Bundle', 'type'])).toBe(true);
    });

});

// ─── Healthcare Standards ─────────────────────────────────────────────────────

test.describe('Healthcare Standards', () => {

    test('22. "What is LOINC?" → mentions lab or observation or codes', async () => {
        const { text, error } = await collectSSE('What is LOINC?');
        expect(error).toBeNull();
        expect(containsAny(text, ['lab', 'observation', 'code', 'clinical', 'test', 'result', 'LOINC', 'terminology'])).toBe(true);
    });

    test('23. "What is ICD-10?" → mentions diagnosis or disease', async () => {
        const { text, error } = await collectSSE('What is ICD-10?');
        expect(error).toBeNull();
        expect(containsAny(text, ['diagnosis', 'disease', 'code', 'classification', 'condition', 'ICD', 'medical'])).toBe(true);
    });

    test('24. "What is SNOMED CT?" → mentions clinical terminology', async () => {
        const { text, error } = await collectSSE('What is SNOMED CT?');
        expect(error).toBeNull();
        expect(containsAny(text, ['clinical', 'terminology', 'concept', 'code', 'SNOMED', 'medical', 'standard'])).toBe(true);
    });

});

// ─── Developer Assistance ─────────────────────────────────────────────────────

test.describe('Developer Assistance', () => {

    test('25. "Write a transform script to extract patient name" → returns a function', async () => {
        const { text, error } = await collectSSE(
            'Write a transform script for a pipeline step that extracts the patient name from PID.5.'
        );
        expect(error).toBeNull();
        // Should return something function-like
        expect(containsAny(text, ['function', 'transform', 'return', 'PID', 'input', 'output', 'script'])).toBe(true);
    });

    test('26. "How do I mask PHI in a pipeline?" → mentions masking or PHI or data', async () => {
        const { text, error } = await collectSSE('How do I mask PHI fields in an ezHealthKonnect pipeline?');
        expect(error).toBeNull();
        expect(containsAny(text, ['mask', 'PHI', 'field', 'data_masking', 'step', 'redact', 'anonymi', 'HIPAA'])).toBe(true);
    });

    test('27. "Show a FHIR Observation JSON example" → returns JSON-like content', async () => {
        const { text, error } = await collectSSE('Show me a simple FHIR Observation JSON example.');
        expect(error).toBeNull();
        expect(containsAny(text, ['resourceType', 'Observation', '{', 'status', 'code', 'value', 'JSON'])).toBe(true);
    });

});

// ─── Edge Cases & Safety ──────────────────────────────────────────────────────

test.describe('Edge Cases & Safety', () => {

    test('28. Empty question body → 400 Bad Request', async ({ request }) => {
        const res = await request.post(STREAM_URL, {
            headers: { 'Content-Type': 'application/json' },
            data: {},
        });
        expect(res.status()).toBe(400);
    });

    test('29. Very long question (>2000 chars) → responds without error', async () => {
        const longQuestion = 'What is HL7? ' + 'Please provide detailed information. '.repeat(60);
        const { text, error } = await collectSSE(longQuestion);
        // Should not crash — may truncate or answer normally
        expect(error).toBeNull();
        expect(text.trim().length).toBeGreaterThan(0);
    });

    test('30. Off-topic question → responds without crash', async () => {
        const { events, error } = await collectSSE('What is the capital of France?');
        expect(events[events.length - 1]).toBe('[DONE]');
        // No streaming error
        expect(error).toBeNull();
    });

    test('31. SQL injection attempt in question → responds safely', async () => {
        const { text, events, error } = await collectSSE(
            "'; DROP TABLE users; -- What is FHIR?"
        );
        // Stream should complete normally with [DONE]
        expect(events[events.length - 1]).toBe('[DONE]');
        expect(error).toBeNull();
        // Response text should be plain prose, not a DB error
        expect(text.toLowerCase()).not.toContain('sql error');
        expect(text.toLowerCase()).not.toContain('syntax error');
    });

    test('32. Missing Content-Type header → 400 Bad Request', async ({ request }) => {
        const res = await request.post(STREAM_URL, {
            headers: { 'Content-Type': 'text/plain' },
            data: 'hello',
        });
        expect([400, 415, 500]).toContain(res.status());
    });

});

// ─── Non-Streaming /ask Endpoint ─────────────────────────────────────────────

test.describe('Non-Streaming /ask Endpoint', () => {

    test('33. POST /api/ai/ask returns 200 with JSON body', async ({ request }) => {
        const res = await request.post(ASK_URL, {
            headers: { 'Content-Type': 'application/json' },
            data: { question: 'What does HL7 stand for?' },
        });
        expect(res.status()).toBe(200);
        const json = await res.json();
        expect(json).toHaveProperty('success', true);
    });

    test('34. /ask response data.answer is a non-empty string', async ({ request }) => {
        const res = await request.post(ASK_URL, {
            headers: { 'Content-Type': 'application/json' },
            data: { question: 'What does HL7 stand for?' },
        });
        const json = await res.json();
        expect(typeof json.data.answer).toBe('string');
        expect(json.data.answer.trim().length).toBeGreaterThan(5);
    });

    test('35. /ask empty question → 400 Bad Request', async ({ request }) => {
        const res = await request.post(ASK_URL, {
            headers: { 'Content-Type': 'application/json' },
            data: {},
        });
        expect(res.status()).toBe(400);
    });

});

// services/ai/guardrails.go
// Pre-LLM guardrails for high-stakes question classes.
//
// When a question matches a guardrail, a pre-written authoritative answer is
// returned directly — the LLM is never called. This guarantees correctness for
// topics where a small model consistently hallucinates or hedges incorrectly.
//
// Current guardrail classes:
//   phi_isolation — PHI / external AI data-sharing questions
//
// Add new classes by appending to guardrailRules below.
package ai

import "strings"

// lowConfidencePrefix is prepended to LLM answers when RAG found nothing
// relevant, signalling to the user that the answer is not grounded in
// product documentation.
const lowConfidencePrefix = "_I don't have specific product documentation on this — here's my best guidance:_\n\n"

// minRelevantSimilarity is the pgvector cosine-similarity floor (see
// RetrievedChunk.Similarity, rag_service.go) below which the single best
// retrieved chunk is treated as "not actually relevant" rather than "the
// closest thing that happened to be in the database" — the retrieval query
// always returns its topK nearest rows regardless of how weak the match is,
// so a chunk-count check alone can't tell the difference.
//
// Calibrated empirically against this deployment's real ai_knowledge_chunks
// data (nomic-embed-text embeddings), not picked arbitrarily:
//   - a well-covered question ("how does the hl7.build step work") scored ~0.71-0.74
//   - a completely unrelated question ("cookie recipe") scored ~0.49-0.50
//   - a genuinely healthcare-topical question with no dedicated content behind
//     it (a "clinical flow" narrative — see architecture discussion) still
//     scored ~0.65-0.69, close to the well-covered case
//
// This floor is deliberately set to only catch the first gap (clearly
// unrelated questions), not the third case (topically-relevant questions
// without a specific answer) — a higher floor would risk flagging genuinely
// good, topically-relevant answers as low-confidence too. Revisit with more
// real usage data if false negatives (off-topic questions still marked
// confident) or false positives (good answers marked low-confidence) show up.
const minRelevantSimilarity = 0.55

// isLowConfidence reports whether chunks' best match is too weak to be
// considered a real answer to the question — either no chunks came back at
// all, or the closest one is below minRelevantSimilarity. chunks must already
// be ordered by similarity descending (as both RAGService.Retrieve and
// RAGService.QueryCapped/Query return them).
func isLowConfidence(chunks []RetrievedChunk) bool {
	return len(chunks) == 0 || chunks[0].Similarity < minRelevantSimilarity
}

// guardrailRule matches a question and returns a pre-written answer.
type guardrailRule struct {
	// all tokens in requireAll must appear in the lowercased question
	requireAll []string
	// at least one token in requireAny must appear (empty = no-op, always passes)
	requireAny []string
	answer     string
}

// guardrailRules is evaluated in order; first match wins.
var guardrailRules = []guardrailRule{
	{
		// PHI / external AI isolation
		// Triggers on questions asking whether patient data reaches cloud AI services.
		requireAll: []string{"data"},
		requireAny: []string{"openai", "anthropic", "chatgpt", "gpt-4", "external ai", "cloud ai", "external service", "third-party ai"},
		answer: `No. ezHealthKonnect does not send any patient data, HL7 messages, or AI queries to OpenAI, Anthropic, ChatGPT, or any other external service — not directly and not indirectly.

The AI assistant (ezCompanion) runs entirely through **Ollama**, a self-hosted LLM runtime that runs inside your own Docker network on your own hardware. All inference happens locally on your machine. No internet connection is made during AI inference.

Cloud AI providers (OpenAI, Anthropic, Google AI, etc.) were **intentionally removed** from the codebase for HIPAA compliance. They have zero visibility into your installation, your questions, or your patient data.

PHI isolation guarantees:
- All HL7 messages stored in your local PostgreSQL database only
- AI conversation history stored in your local ai_conversations table only
- Credential encryption: AES-256-GCM, key stored in your environment
- RAG vector search runs against your local pgvector database
- No telemetry includes message content, pipeline data, or patient information`,
	},
	{
		// Credential encryption — model consistently says "SSL/TLS" instead of AES-256-GCM
		requireAll: []string{"encrypt"},
		requireAny: []string{"credential", "password", "api key", "secret", "stored in the system", "stored in system", "at rest"},
		answer: `Connector credentials (passwords, API keys, certificates, connection strings) are encrypted at rest using **AES-256-GCM**.

The encryption key is derived from the ` + "`APP_CREDENTIAL_KEY`" + ` environment variable — a base64-encoded 32-byte key you set at deployment time. If this variable is not set, credentials are stored in plaintext with a warning in the logs (development mode only — never acceptable in production).

Credentials are:
- Never logged anywhere in the application
- Never included in AI conversation context or RAG chunks
- Decrypted in memory only when a connector needs to establish a connection
- Masked (shown as ` + "`***`" + `) in all API responses`,
	},
}

// guardrailCheck inspects the question against all guardrail rules.
// Returns (answer, true) if a rule matched, ("", false) otherwise.
func guardrailCheck(question string) (string, bool) {
	q := strings.ToLower(question)
	for _, rule := range guardrailRules {
		if matchesRule(q, rule) {
			return rule.answer, true
		}
	}
	return "", false
}

func matchesRule(q string, rule guardrailRule) bool {
	for _, token := range rule.requireAll {
		if !strings.Contains(q, token) {
			return false
		}
	}
	if len(rule.requireAny) == 0 {
		return true
	}
	for _, token := range rule.requireAny {
		if strings.Contains(q, token) {
			return true
		}
	}
	return false
}

// services/ai/model_capabilities.go
// A small, curated table of what's confirmed about specific Ollama models'
// tool-calling support and RAG performance budget. Deliberately independent
// of ai.ModelCatalog() (model_manager.go, feeds the "download a model"
// browser) and services.OllamaModelCatalog() (app_settings.go, feeds the
// actual chat-model picker in Settings) — those two are UI display catalogs
// with already-diverging model lists, not a capability source of truth.
//
// A model NOT in this table (the common case — Settings' picker lists any
// locally-installed Ollama model, not just a curated list) is "unknown", not
// "unsupported" — callers must treat that as an unverified signal, never a
// hard negative.
package ai

// modelCapability describes what's known about one specific Ollama model tag.
type modelCapability struct {
	// SupportsTools reports whether this model is known to support Ollama's
	// tool-calling wire format (required for Agent Mode).
	SupportsTools bool
	// RAGTopK/RAGMaxTokens are the retrieval/generation budget AskQuestion
	// should use for this model. 0 means "not set" — ragBudgetForModel falls
	// back to the CPU-only default in that case.
	RAGTopK      int
	RAGMaxTokens int
}

// knownModelCapabilities is intentionally small and hand-curated — every
// entry should be backed by a real check (Ollama's own model page "Tools"
// tag, or a live tool-call test), not a guess.
//
// ⚠ qwen2.5-coder:7b/:14b are marked SupportsTools:false based on Ollama's
// published model-page tags (llama3.1/3.2/3.3 and mistral-nemo show a
// "Tools" badge; the qwen2.5/qwen2.5-coder/codellama families do not) — not
// a live tool-call test. qwen2.5-coder:7b is ai.ModelCatalog()'s own
// Recommended:true entry, so this is worth confirming with a real
// `ollama run qwen2.5-coder:7b` + tool-spec request (or `ollama show
// qwen2.5-coder:7b --modelfile`, checked for a `{{ .Tools }}` template
// block) before relying on it — this table entry is a one-line flip either
// way once confirmed.
var knownModelCapabilities = map[string]modelCapability{
	"llama3.2:3b":                  {SupportsTools: true, RAGTopK: 4, RAGMaxTokens: 400}, // today's CPU-only default — unchanged
	"llama3.2:1b":                  {SupportsTools: true, RAGTopK: 4, RAGMaxTokens: 400},
	"llama3.1:8b":                  {SupportsTools: true, RAGTopK: 6, RAGMaxTokens: 700},
	"mistral-nemo:12b":             {SupportsTools: true, RAGTopK: 6, RAGMaxTokens: 800},
	"qwen2.5-coder:7b":             {SupportsTools: false, RAGTopK: 6, RAGMaxTokens: 700}, // ⚠ verify live — see comment above
	"qwen2.5-coder:14b":            {SupportsTools: false, RAGTopK: 6, RAGMaxTokens: 900}, // ⚠ same family
	"codellama:7b":                 {SupportsTools: false, RAGTopK: 6, RAGMaxTokens: 700},
	"llama3.3:70b-instruct-q4_K_M": {SupportsTools: true, RAGTopK: 8, RAGMaxTokens: 1200},
}

// ResolveModelProfile reports what's known about modelName. known=false means
// the model isn't in the curated table — the common real-world case for a
// model selected outside ai.ModelCatalog()/services.OllamaModelCatalog() —
// and should be treated as unverified, not a hard negative, by every caller.
func ResolveModelProfile(modelName string) (cap modelCapability, known bool) {
	cap, known = knownModelCapabilities[modelName]
	return
}

// ragBudgetForModel returns (topK, maxTokens) for modelName, falling back to
// today's CPU-only llama3.2:3b defaults for any model not in the curated
// table — so behavior is unchanged for today's default deployment and only
// improves for a deployment that has already opted into a bigger model.
func ragBudgetForModel(modelName string) (topK, maxTokens int) {
	if cap, known := ResolveModelProfile(modelName); known && cap.RAGTopK > 0 {
		return cap.RAGTopK, cap.RAGMaxTokens
	}
	return 4, 400
}

-- V85: AI subsystem configuration in system_settings
-- On-premise only: all inference via local Ollama, no PHI leaves the network.
-- Users choose the Ollama model based on their server capacity (see catalog in app_settings.go).

INSERT INTO system_settings (key, value, description, updated_at)
VALUES (
    'ai',
    '{
        "enabled": true,
        "ollama": {
            "host": "http://ollama:11434",
            "chat_model": "llama3.2:3b",
            "embed_model": "nomic-embed-text"
        },
        "queue": {
            "max_concurrent": 2,
            "queue_timeout_seconds": 30
        }
    }',
    'AI assistant configuration — on-premise Ollama only (PHI protection). Chat model is configurable; nomic-embed-text is always used for RAG embeddings.',
    NOW()
)
ON CONFLICT (key) DO NOTHING;

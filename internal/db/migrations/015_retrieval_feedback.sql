CREATE TABLE kg_retrieval_feedback (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    query_text              TEXT NOT NULL,
    normalized_query        TEXT NOT NULL,
    search_mode             TEXT NOT NULL DEFAULT '',
    intent                  TEXT NOT NULL DEFAULT '',
    internal_rerank         TEXT NOT NULL DEFAULT '',
    internal_rerank_profile TEXT NOT NULL DEFAULT '',
    outcome                 TEXT NOT NULL,
    signals                 TEXT[] NOT NULL DEFAULT '{}',
    retrieved_node_ids      TEXT[] NOT NULL DEFAULT '{}',
    selected_node_ids       TEXT[] NOT NULL DEFAULT '{}',
    expected_node_ids       TEXT[] NOT NULL DEFAULT '{}',
    note                    TEXT NOT NULL DEFAULT '',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT kg_retrieval_feedback_outcome_chk CHECK (outcome IN ('helpful', 'unhelpful', 'missed')),
    CONSTRAINT kg_retrieval_feedback_query_len_chk CHECK (char_length(query_text) BETWEEN 1 AND 500),
    CONSTRAINT kg_retrieval_feedback_note_len_chk CHECK (char_length(note) <= 500)
);

ALTER TABLE kg_retrieval_feedback ENABLE ROW LEVEL SECURITY;
ALTER TABLE kg_retrieval_feedback FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_kg_retrieval_feedback ON kg_retrieval_feedback
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

CREATE INDEX idx_kg_retrieval_feedback_tenant_created_at ON kg_retrieval_feedback(tenant_id, created_at DESC);
CREATE INDEX idx_kg_retrieval_feedback_tenant_query ON kg_retrieval_feedback(tenant_id, normalized_query, search_mode);
CREATE INDEX idx_kg_retrieval_feedback_signals ON kg_retrieval_feedback USING GIN(signals);

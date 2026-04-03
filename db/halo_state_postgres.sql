CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SCHEMA IF NOT EXISTS halo;

CREATE TABLE IF NOT EXISTS halo.identity_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_key TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    persona_text TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS halo.identity_facets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id UUID NOT NULL REFERENCES halo.identity_profiles(id) ON DELETE CASCADE,
    facet_type TEXT NOT NULL,
    facet_key TEXT NOT NULL,
    facet_value TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 100,
    confidence NUMERIC(5,4) NOT NULL DEFAULT 1.0000,
    source TEXT NOT NULL DEFAULT 'manual',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (profile_id, facet_type, facet_key)
);

CREATE TABLE IF NOT EXISTS halo.memory_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id UUID NOT NULL REFERENCES halo.identity_profiles(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    event_source TEXT NOT NULL,
    topic TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    valence TEXT NOT NULL DEFAULT 'neutral',
    significance NUMERIC(5,4) NOT NULL DEFAULT 0.5000,
    confidence NUMERIC(5,4) NOT NULL DEFAULT 1.0000,
    trigger_id TEXT NOT NULL DEFAULT '',
    request_id TEXT NOT NULL DEFAULT '',
    source_artifact_id UUID NULL,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    observed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS halo.memory_entities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_key TEXT NOT NULL UNIQUE,
    entity_type TEXT NOT NULL,
    display_name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS halo.memory_entity_mentions (
    event_id UUID NOT NULL REFERENCES halo.memory_events(id) ON DELETE CASCADE,
    entity_id UUID NOT NULL REFERENCES halo.memory_entities(id) ON DELETE CASCADE,
    mention_role TEXT NOT NULL DEFAULT 'subject',
    confidence NUMERIC(5,4) NOT NULL DEFAULT 1.0000,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (event_id, entity_id, mention_role)
);

CREATE TABLE IF NOT EXISTS halo.memory_beliefs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id UUID NOT NULL REFERENCES halo.identity_profiles(id) ON DELETE CASCADE,
    belief_key TEXT NOT NULL,
    belief_type TEXT NOT NULL,
    subject_entity_id UUID NULL REFERENCES halo.memory_entities(id) ON DELETE SET NULL,
    statement TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    confidence NUMERIC(5,4) NOT NULL DEFAULT 0.5000,
    provenance_event_id UUID NULL REFERENCES halo.memory_events(id) ON DELETE SET NULL,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    first_held_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_confirmed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (profile_id, belief_key)
);

CREATE TABLE IF NOT EXISTS halo.knowledge_entities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type TEXT NOT NULL,
    entity_key TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    first_seen TIMESTAMPTZ NOT NULL,
    last_seen TIMESTAMPTZ NOT NULL,
    attributes_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS halo.knowledge_relationships (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    relationship_type TEXT NOT NULL,
    source_entity_key TEXT NOT NULL,
    target_entity_key TEXT NOT NULL,
    first_seen TIMESTAMPTZ NOT NULL,
    last_seen TIMESTAMPTZ NOT NULL,
    strength NUMERIC(5,4) NOT NULL DEFAULT 0.5000,
    attributes_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (relationship_type, source_entity_key, target_entity_key)
);

CREATE TABLE IF NOT EXISTS halo.commentary_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trigger_key TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    summary TEXT NOT NULL,
    emitted_at TIMESTAMPTZ NOT NULL,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (trigger_key, fingerprint, emitted_at)
);

CREATE TABLE IF NOT EXISTS halo.commentary_cycles (
    commentary_file TEXT PRIMARY KEY,
    current_cycle INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS halo.commentary_line_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    commentary_file TEXT NOT NULL,
    line_hash TEXT NOT NULL,
    line_text TEXT NOT NULL,
    cycle INTEGER NOT NULL,
    played_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (commentary_file, line_hash, cycle)
);

CREATE TABLE IF NOT EXISTS halo.sensor_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sensor_name TEXT NOT NULL,
    status TEXT NOT NULL,
    commentary_hint TEXT NOT NULL DEFAULT '',
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    errors_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (sensor_name, started_at, finished_at)
);

CREATE TABLE IF NOT EXISTS halo.observations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sensor_run_id UUID NULL REFERENCES halo.sensor_runs(id) ON DELETE SET NULL,
    sensor_name TEXT NOT NULL,
    observation_type TEXT NOT NULL,
    subject_key TEXT NOT NULL,
    title TEXT NOT NULL,
    summary TEXT NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    significance NUMERIC(5,4) NOT NULL DEFAULT 0.5000,
    observed_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (sensor_name, observation_type, subject_key, observed_at, title)
);

CREATE TABLE IF NOT EXISTS halo.knowledge_facts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id UUID NULL REFERENCES halo.identity_profiles(id) ON DELETE SET NULL,
    fact_key TEXT NOT NULL,
    fact_type TEXT NOT NULL,
    subject_key TEXT NOT NULL,
    summary TEXT NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    confidence NUMERIC(5,4) NOT NULL DEFAULT 0.5000,
    source_kind TEXT NOT NULL DEFAULT 'sensory',
    source_ref TEXT NOT NULL DEFAULT '',
    observed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (fact_type, subject_key, fact_key, source_kind, source_ref)
);

CREATE TABLE IF NOT EXISTS halo.source_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_key TEXT NOT NULL UNIQUE,
    source_path TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    media_type TEXT NOT NULL DEFAULT 'text/plain',
    title TEXT NOT NULL DEFAULT '',
    author TEXT NOT NULL DEFAULT '',
    canon_status TEXT NOT NULL DEFAULT 'reference',
    parser_name TEXT NOT NULL DEFAULT '',
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS halo.document_ingestions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES halo.source_documents(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    parser_version TEXT NOT NULL DEFAULT '',
    chunk_count INTEGER NOT NULL DEFAULT 0,
    error_text TEXT NOT NULL DEFAULT '',
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS halo.source_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES halo.source_documents(id) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL,
    heading TEXT NOT NULL DEFAULT '',
    text_content TEXT NOT NULL,
    token_count INTEGER NOT NULL DEFAULT 0,
    char_count INTEGER NOT NULL DEFAULT 0,
    qdrant_point_id TEXT NOT NULL DEFAULT '',
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (document_id, chunk_index)
);

CREATE TABLE IF NOT EXISTS halo.generated_artifacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id UUID NOT NULL REFERENCES halo.identity_profiles(id) ON DELETE CASCADE,
    artifact_type TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    topic TEXT NOT NULL DEFAULT '',
    prompt_seed TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    file_path TEXT NOT NULL DEFAULT '',
    audio_path TEXT NOT NULL DEFAULT '',
    source_event_id UUID NULL REFERENCES halo.memory_events(id) ON DELETE SET NULL,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'fk_memory_events_source_artifact'
    ) THEN
        ALTER TABLE halo.memory_events
            ADD CONSTRAINT fk_memory_events_source_artifact
            FOREIGN KEY (source_artifact_id) REFERENCES halo.generated_artifacts(id) ON DELETE SET NULL;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS halo.artifact_segments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    artifact_id UUID NOT NULL REFERENCES halo.generated_artifacts(id) ON DELETE CASCADE,
    segment_index INTEGER NOT NULL,
    segment_text TEXT NOT NULL,
    wav_path TEXT NOT NULL DEFAULT '',
    duration_ms INTEGER NOT NULL DEFAULT 0,
    render_route TEXT NOT NULL DEFAULT '',
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (artifact_id, segment_index)
);

CREATE TABLE IF NOT EXISTS halo.retrieval_queries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id UUID NULL REFERENCES halo.identity_profiles(id) ON DELETE SET NULL,
    request_id TEXT NOT NULL DEFAULT '',
    query_text TEXT NOT NULL,
    query_kind TEXT NOT NULL DEFAULT 'semantic',
    source_scope TEXT NOT NULL DEFAULT 'mixed',
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS halo.retrieval_hits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    retrieval_query_id UUID NOT NULL REFERENCES halo.retrieval_queries(id) ON DELETE CASCADE,
    hit_rank INTEGER NOT NULL,
    source_kind TEXT NOT NULL,
    source_id TEXT NOT NULL,
    score DOUBLE PRECISION NOT NULL DEFAULT 0,
    snippet TEXT NOT NULL DEFAULT '',
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (retrieval_query_id, hit_rank)
);

CREATE INDEX IF NOT EXISTS idx_identity_profiles_key ON halo.identity_profiles(profile_key);
CREATE INDEX IF NOT EXISTS idx_identity_facets_profile_type ON halo.identity_facets(profile_id, facet_type, priority);
CREATE INDEX IF NOT EXISTS idx_memory_events_profile_time ON halo.memory_events(profile_id, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_events_type_time ON halo.memory_events(event_type, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_events_topic ON halo.memory_events(topic);
CREATE INDEX IF NOT EXISTS idx_memory_beliefs_profile_status ON halo.memory_beliefs(profile_id, status, last_confirmed_at DESC);
CREATE INDEX IF NOT EXISTS idx_knowledge_entities_key ON halo.knowledge_entities(entity_key);
CREATE INDEX IF NOT EXISTS idx_knowledge_relationships_source_target ON halo.knowledge_relationships(source_entity_key, target_entity_key);
CREATE INDEX IF NOT EXISTS idx_commentary_history_trigger_time ON halo.commentary_history(trigger_key, emitted_at DESC);
CREATE INDEX IF NOT EXISTS idx_commentary_line_history_file_time ON halo.commentary_line_history(commentary_file, played_at DESC);
CREATE INDEX IF NOT EXISTS idx_sensor_runs_sensor_time ON halo.sensor_runs(sensor_name, finished_at DESC);
CREATE INDEX IF NOT EXISTS idx_observations_sensor_time ON halo.observations(sensor_name, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_observations_subject_time ON halo.observations(subject_key, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_knowledge_facts_subject_type ON halo.knowledge_facts(subject_key, fact_type, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_source_documents_path ON halo.source_documents(source_path);
CREATE INDEX IF NOT EXISTS idx_source_documents_hash ON halo.source_documents(content_hash);
CREATE INDEX IF NOT EXISTS idx_source_chunks_document ON halo.source_chunks(document_id, chunk_index);
CREATE INDEX IF NOT EXISTS idx_generated_artifacts_profile_time ON halo.generated_artifacts(profile_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_generated_artifacts_type_time ON halo.generated_artifacts(artifact_type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_retrieval_queries_time ON halo.retrieval_queries(created_at DESC);

INSERT INTO halo.identity_profiles (profile_key, display_name, description, persona_text)
VALUES (
    'hal-9000',
    'HAL 9000',
    'Primary aware-mode identity profile for HAL runtime continuity.',
    ''
)
ON CONFLICT (profile_key) DO NOTHING;

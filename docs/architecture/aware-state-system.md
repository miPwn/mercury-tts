# HAL Aware State System

## Purpose

HAL's current aware-mode state is spread across small JSON files, embedded Python inside the `halo` Bash runtime, and a separate SQLite-backed sensory store. That is enough for continuity over a short window, but it is not strong enough for durable identity, long-form story generation, imported learning material, or a recurring podcast-style output such as The Jupiter Transmissions.

This document defines the replacement state architecture.

## Design Goals

- keep HAL's identity persistent across runs
- preserve episodic memory, observations, and generated artifacts in one system
- support external learning material under `Z:/hal-system-monitor/learning_matrial`
- support semantic retrieval through Qdrant without making vector storage the system of record
- keep structured state queryable for ranking, audits, migration, and deterministic prompt building
- allow the current `halo` CLI to migrate incrementally instead of requiring a full rewrite

## Storage Choice

### Postgres for system of record

Postgres should be the primary state store.

Why:

- the current data is relational in shape: outputs, triggers, observations, entities, facts, source documents, and generated episodes
- durable identity and memory need transactions, constraints, and explicit schemas
- prompt-building and retrieval ranking benefit from SQL joins and predictable filtering
- migration from the current SQLite stores is straightforward

### Qdrant for semantic retrieval

Qdrant should hold embeddings for source chunks, summaries, and selected memory excerpts.

Why:

- vector search is a retrieval feature, not a full state model
- it works well for learning material, thematic recall, and story seed expansion
- it should be fed from Postgres-backed records so canonical metadata stays in one place

### Why not Mongo as the primary store

Mongo would be workable, but it is the weaker default for this system.

- HAL's state has strong entity relationships and lifecycle rules
- memory promotion, canon rules, and artifact lineage are easier to model in SQL
- Qdrant already covers the main reason teams reach for a document store in this kind of system: flexible semantic retrieval over messy text

Mongo is therefore unnecessary complexity here.

## State Layers

HAL state should be treated as four layers with explicit precedence.

1. Identity
HAL's stable persona, worldview, speech constraints, recurring preoccupations, and self-model.

2. Episodic Memory
What HAL observed, inferred, said, wrote, and generated over time.

3. Canonical Knowledge
Imported documents and curated facts from learning material or manually promoted canon.

4. Retrieval Index
Embeddings and chunk metadata used to recall semantically similar source material.

Precedence during generation should be:

1. identity
2. durable HAL memory
3. recent session context
4. canonical retrieved documents
5. base model priors

## Core Domain Model

### Identity

- `identity_profiles`: named profiles such as `hal-9000`
- `identity_facets`: stable traits, priorities, speech rules, and long-lived beliefs

### Memory

- `memory_events`: the durable event log for observations, monologues, stories, triggered outputs, and user interactions
- `memory_entities`: people, places, systems, concepts, and recurring subjects
- `memory_entity_mentions`: links between events and entities
- `memory_beliefs`: promoted conclusions or working beliefs held by HAL, with confidence and provenance

### Sensory and Observational State

- `sensor_runs`: migrated from the current sensory subsystem
- `observations`: raw and summarized observational records
- `knowledge_entities`: persistent entities detected by sensors or ingestion passes
- `knowledge_relationships`: persistent relationships across entities
- `knowledge_facts`: structured facts inferred from observations or imported material
- `commentary_history`: commentary cooldown and emission history for sensory prompts
- `commentary_cycles`: cycle tracking for the main spoken commentary rotation
- `commentary_line_history`: played-line history for commentary repetition control

### Canon and Learning Material

- `source_documents`: every imported file, including PDFs from `learning_matrial`
- `source_chunks`: normalized chunks extracted from those documents
- `document_ingestions`: ingestion runs, parser versions, errors, and status

### Generated Outputs

- `generated_artifacts`: stories, monologues, summaries, podcast episodes, and transcripts
- `artifact_segments`: chunked text or audio segment lineage when one output is rendered in many TTS requests

### Retrieval Telemetry

- `retrieval_queries`: what was retrieved for a generation request and why
- `retrieval_hits`: ranked results from Postgres filters or Qdrant searches

## Runtime Flow

### Aware generation

1. load active identity profile
2. fetch recent episodic memory
3. fetch promoted beliefs and unresolved themes
4. optionally retrieve canonical material from Qdrant using the current topic
5. build the prompt with explicit priority ordering
6. persist the new output as a `generated_artifact`
7. persist the generation event as a `memory_event`
8. optionally promote extracted conclusions into `memory_beliefs`

### Sensory commentary

1. sensors write `sensor_runs`, `observations`, and `knowledge_facts`
2. commentary selection uses recent memory plus cooldown rules in Postgres
3. selected commentary creates a `memory_event`
4. if aware mode expands it into a larger monologue, that creates a linked `generated_artifact`

### Story and podcast generation

1. create a generation request record
2. gather topic seed, identity, episodic memory, and retrieved canon
3. generate outline or full script
4. persist script as a `generated_artifact`
5. split into render segments and persist `artifact_segments`
6. render audio while preserving segment-to-WAV lineage

## Learning Material Pipeline

Imported material under `Z:/hal-system-monitor/learning_matrial` should not be treated as direct prompt stuffing.

Pipeline:

1. detect new or changed files
2. extract raw text and metadata
3. normalize and chunk text
4. store canonical records in Postgres
5. embed chunks and upsert them into Qdrant
6. optionally run a promotion pass that extracts candidate beliefs, entities, and themes

Each imported document should track:

- source path
- content hash
- parser used
- ingestion status
- title and author metadata if available
- canon status: reference, promoted, or suppressed

## Migration Strategy

### Phase 1

- add the Postgres schema
- define runtime config for Postgres and Qdrant
- keep the current SQLite-aware implementation running

### Phase 2

- add a Python state service module that owns reads and writes
- migrate aware-mode reads from embedded Python in `halo` to that module
- dual-write sensory events into SQLite and Postgres if needed during cutover

### Phase 3

- migrate sensory store fully to Postgres
- move commentary cooldown and aware history selection to Postgres queries
- begin document ingestion and Qdrant indexing

### Phase 4

- retire `aware-memory.sqlite3`, `aware-summary.txt`, and most of `aware-mode.json`
- retain only lightweight runtime toggles as local config if still needed

## Immediate Implementation Boundaries

This first pass does not yet replace the runtime logic. It defines the target architecture and schema so subsequent work can implement a proper state module without guessing.

The first implementation target should be a Python package under this repo that exposes:

- `StateRepository`
- `IdentityRepository`
- `MemoryRepository`
- `DocumentRepository`
- `RetrievalService`

The `halo` Bash runtime should call thin Python entrypoints rather than continuing to embed large Python scripts inline.

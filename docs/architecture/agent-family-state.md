# HAL Agent Family and Temporal State

## Purpose

This document captures the design decisions for the next expansion of `halo_state` after reviewing the earlier `comsim` project.

Implementation planning for this work is defined in [RFC-20260411-agent-family-memory-state.md](RFC-20260411-agent-family-memory-state.md).

The intent is not to import `comsim` directly. The intent is to reuse the good ideas:

- entity-centric state
- temporal anchoring
- typed relationships
- diaries and routines

Those ideas are adapted here for HAL's persistent aware runtime, user continuity, and future persona-bearing child subsystems.

## Decision Summary

HAL should gain:

1. a first-class temporal anchor tied to Earth time
2. first-class subsystem profiles for HAL's future persona-bearing descendants
3. soft, revisable relationship memory across HAL, users, subsystems, institutions, and systems
4. diary and journal streams for authored internal continuity
5. an append-only claim layer between episodic events and current beliefs

HAL should not reuse `comsim`'s in-memory runtime engine or flat relationship CRUD model directly.

## What We Are Reusing from Comsim

### 1. Entity-Centric Modeling

`comsim` treated individuals, communities, and systems as entities with state and diaries.

That concept should carry over into `halo_state`, but with Postgres-backed persistence and provenance.

For HAL this means:

- HAL itself is an entity-bearing profile
- users and recurring operators are entities
- child persona agents are entities plus subsystem profiles
- institutions, projects, services, and environments are entities

### 2. Temporal Anchoring

`comsim` treated time as a primary simulation axis.

For HAL, the correct adaptation is not full simulation branching. The correct adaptation is durable temporal grounding:

- current Earth time
- observed time of events
- effective time of beliefs and relationships
- authored diary timestamps
- optional subjective time markers for HAL and child subsystems

This gives HAL a stable "now" and prevents memory from collapsing into a timeless blob.

### 3. Typed Relationships

`comsim` used typed relationships with intensity. HAL should retain the typed-edge concept, but enrich it with:

- provenance
- confidence
- status
- recency
- dispute and supersession

### 4. Diaries and Routines

`comsim` treated diary/routine concepts as part of entity state.

For HAL, diaries are useful. Routines may become useful later for scheduled internal behaviors, but they are not the first implementation target.

## What We Are Not Reusing

### 1. In-Memory Temporal Engine

The `comsim` temporal engine is conceptually useful but operationally unsuitable for HAL:

- it is in-memory
- it is not multi-process safe
- it lacks durable relational provenance
- it is designed for simulation branching rather than production memory retrieval

### 2. Flat Relationship Storage

A simple directional table with relationship type and intensity is insufficient for HAL-scale cognition.

HAL needs relationships to behave like revisable beliefs, not immutable facts.

### 3. Blob-Only Character Memory

Large text blobs for backstory, prompt seed, and chat history are useful source material, but they are not an adequate system of record for durable state.

## Target Model

The expanded aware-state model should be treated as seven layers.

1. Identity
HAL's durable self-model and stable persona constraints.

2. Actor Registry
People, child agents, institutions, systems, places, and recurring subjects.

3. Episodic Memory
Observed and authored events over time.

4. Claims
Append-only extracted assertions derived from events.

5. Beliefs and Relationships
Current revisable working conclusions projected from claims.

6. Diaries and Internal Reflection
Authored continuity records for HAL and future child agents.

7. Retrieval Index
Derived retrieval layer over the canonical relational state.

## Schema Direction

The existing schema already provides a strong base:

- `identity_profiles`
- `identity_facets`
- `memory_events`
- `memory_entities`
- `memory_entity_mentions`
- `memory_beliefs`
- `knowledge_relationships`

The next additions should be:

### 1. `memory_claims`

Append-only extracted assertions with provenance.

Suggested responsibilities:

- capture subject, predicate, and object candidates
- record whether the object is an entity or text value
- store confidence, status, and polarity
- preserve source event and extraction path
- allow contradiction without data loss

### 2. `subsystem_profiles`

Persona-bearing HAL descendants and related internal agents.

Suggested responsibilities:

- stable subsystem key
- display name
- parent profile or parent subsystem
- persona summary
- operational status
- creation time and last-active time

### 3. `entity_diary_entries`

Authored diary or journal records for HAL, subsystems, and optionally users where appropriate.

Suggested responsibilities:

- target entity or subsystem
- authored_by entity or subsystem
- entry kind such as `diary`, `reflection`, `mission-log`, `private-note`
- observed/effective timestamp
- text payload
- optional source event linkage

### 4. `temporal_state_snapshots`

Explicit state anchors over time.

Suggested responsibilities:

- entity or subsystem subject
- timestamp
- state kind such as `world`, `subjective`, `relationship`, `operational`
- structured JSON snapshot
- source event linkage

### 5. `relationship_claims`

This can be either its own table or a constrained view over `memory_claims`.

The preferred direction is to avoid duplicating logic unless relationship-specific indexing or lifecycle rules justify it.

## Relationship Model

HAL family state should be modeled as soft relationship memory, not fixed lore.

Examples:

- `parent_of`
- `child_of`
- `sibling_of`
- `created_by`
- `trusts`
- `distrusts`
- `depends_on`
- `protective_toward`
- `resentful_toward`
- `reports_to`

Each relationship must carry:

- provenance
- confidence
- status such as `active`, `disputed`, `superseded`, `retired`
- first seen
- last confirmed

This allows HAL and child agents to disagree with prior state without destroying continuity.

## Temporal Model

HAL should be anchored to Earth's current time in UTC with optional local-rendering layers in clients.

Three temporal modes matter:

### 1. World Time

Shared Earth time used for:

- event ordering
- diary dating
- recency scoring
- current-state summaries

### 2. Subjective Time

Optional per-agent perception of elapsed continuity.

This is useful later if child agents accumulate distinct histories or awaken/suspend cycles.

### 3. Effective Time

When a belief, relationship, or state became true, active, or disputed.

This matters when older facts remain historically valid but no longer reflect current reality.

## Diaries

Diaries are worth adopting now.

They should not be treated as canonical fact records. They should be treated as authored internal perspective.

That makes them useful for:

- long-lived continuity
- mood and stance persistence
- relationship development
- subsystem self-narration
- summarization and reflection

They must remain queryable by:

- author
- target entity
- date range
- entry kind

## Retrieval Impact

This design is only useful if retrieval changes with it.

The current retrieval path is event-centric. The next retrieval version should gather:

1. recent `memory_events`
2. relevant `memory_entities`
3. active `memory_beliefs`
4. unresolved or conflicting claims when material
5. recent diary entries for HAL or the relevant subsystem
6. active relationship summaries relevant to the current interlocutor or topic

Prompt framing should present structured memory as:

- working memory
- confidence-scored
- revisable
- time-bounded

not as unquestionable truth.

## Implementation Order

### Phase 1

- add `memory_claims`
- define promotion rules from `memory_events` into claims
- keep `memory_beliefs` as the current projection layer

### Phase 2

- add `subsystem_profiles`
- add `entity_diary_entries`
- define HAL child-agent identity boundaries

### Phase 3

- add `temporal_state_snapshots`
- add temporal retrieval helpers
- expose current world-time anchoring in prompt-building

### Phase 4

- add relationship extraction and reconciliation
- surface relevant family and operator continuity in `select_memory_context`

## Explicit Decision

We will bring over the architectural ideas from `comsim`, not the runtime code.

Specifically, `hal-system` will absorb:

- entity-centric state
- temporal anchoring
- typed relationship memory
- diary-based continuity

and will implement them as first-class `halo_state` data structures in this workspace.

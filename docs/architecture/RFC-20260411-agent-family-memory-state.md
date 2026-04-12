# RFC-20260411-agent-family-memory-state

- **RFC ID:** `RFC-20260411-agent-family-memory-state`
- **Title:** Agent Family Memory, Temporal State, and Chat Administration
- **Status:** `Proposed`
- **Authors:** `Codex`, `HAL system maintainers`
- **Created:** `2026-04-11`
- **Updated:** `2026-04-11`
- **Related RFCs:** `None`

## 1. Context & Background

HAL's current aware-state system is Postgres-backed and already includes:

- `identity_profiles`
- `identity_facets`
- `memory_events`
- `memory_entities`
- `memory_entity_mentions`
- `memory_beliefs`
- `knowledge_relationships`

That model is sufficient for durable identity and basic event memory, but it is still too thin for the next target state:

- HAL should have a temporally grounded sense of "now"
- HAL should be able to maintain revisable opinions and relationships about people
- HAL should support future persona-bearing child subsystems
- HAL should support diary-like internal continuity
- operators should be able to inspect, tune, and govern these mechanisms from the chat client

Review of the earlier `comsim` project identified four concepts worth reusing in bespoke form:

- entity-centric state
- temporal anchoring
- typed relationships
- diary continuity

The correct implementation target is `halo_state` inside `hal-system`, not direct import of `comsim` runtime code.

## 2. Problem Statement

The current system has three primary deficiencies.

First, structured memory is incomplete. The schema has entities and beliefs, but the operational path remains event-heavy. `select_memory_context` still reads mostly from `memory_events`, and `halo-chat` commit currently writes flattened conversation memory.

Second, temporal grounding is under-specified. HAL can store timestamps on events, but there is no explicit temporal state layer that distinguishes:

- world time
- effective time
- subjective agent time

Third, the current chat clients, especially the Windows-hosted `halo-chat` web client, do not expose memory-governance capabilities appropriate for this expansion. Operators need a safe administration surface for:

- promotion tuning
- subsystem profile configuration
- relationship review
- diary inspection
- feature toggles and diagnostics

### In Scope

- schema additions in `halo_state`
- service-layer support for claims, subsystem profiles, diaries, and temporal state
- retrieval and prompt-building changes
- promotion workflow changes from committed chat sessions
- `halo-chat-api` endpoint changes needed to support administration
- `halo-chat` web client changes, including a new `Admin` tab

### Out of Scope

- full autonomous multi-agent orchestration runtime
- production-grade role-based access control beyond the initial operator/admin distinction
- background simulation or timeline branching similar to `comsim`
- mobile or non-web client administration surfaces in this RFC

## 3. Goals

- Introduce a durable, revisable memory model for people, subsystems, institutions, and systems.
- Anchor HAL and future child subsystems in Earth current time with explicit temporal semantics.
- Support HAL family-state without hard-coding immutable lore.
- Make claims append-only and beliefs revisable.
- Add diary/journal continuity for HAL and future subsystems.
- Expose operator controls in `halo-chat` so administrators can inspect and tune memory behavior without direct database access.
- Preserve auditability and provenance across all promotions and overrides.

## 4. Non-Goals

- Build a full social simulation engine.
- Reproduce `comsim`'s in-memory state tree implementation.
- Replace Postgres with a new primary state store.
- Allow unchecked manual editing of prompt-critical memory without provenance.

## 5. Options Considered

### Option A: Keep the Current Event-Centric Model and Only Improve Prompting

- **Description:** Continue storing memory primarily as `memory_events`, add prompt heuristics, and avoid new relational state layers.
- **Pros:**
  - Lowest short-term implementation cost
  - Minimal schema churn
  - Low migration risk
- **Cons:**
  - Weak support for revisable identity and relationship state
  - Poor introspection for operators
  - Hard to govern subsystem-family continuity
  - Retrieval remains too blunt for nuanced memory
- **Risks & Unknowns:**
  - Prompt complexity may grow faster than state quality
  - Contradictions become harder to reason about
- **Maturity / Adoption Level:** `mature but insufficient`

### Option B: Add Claims, Temporal State, Subsystem Profiles, and Admin Surfaces in `halo_state`

- **Description:** Extend the current relational model with append-only claims, subsystem profiles, diary entries, temporal snapshots, and client/admin tooling.
- **Pros:**
  - Aligns with existing Postgres-first architecture
  - Supports revisable beliefs and family-state cleanly
  - Keeps provenance and auditability strong
  - Gives the chat client a proper governance surface
- **Cons:**
  - Moderate schema and API expansion
  - More retrieval complexity
  - Requires careful UI design to avoid unsafe operator actions
- **Risks & Unknowns:**
  - Retrieval ranking may need iterative tuning
  - Claim extraction quality will determine usefulness
- **Maturity / Adoption Level:** `recommended evolutionary step`

### Option C: Import or Adapt `comsim` Runtime Modules More Directly

- **Description:** Pull temporal/entity modules from `comsim` and adapt them into `hal-system`.
- **Pros:**
  - Faster reuse of conceptual prior work
  - Some code already exists for temporal and entity concepts
- **Cons:**
  - Wrong operational model for HAL
  - In-memory design does not fit current persistence architecture
  - Would create parallel state logic rather than extending `halo_state`
- **Risks & Unknowns:**
  - Architectural drift
  - Hidden assumptions from simulation code leaking into production memory
- **Maturity / Adoption Level:** `prototype-grade, not suitable as direct base`

| Option | Pros | Cons | Best Use Case |
|---|---|---|---|
| Event-centric only | Fastest path, minimal schema change | Fails the family-state and revisable-memory requirements | Small assistant with shallow persistence |
| Extend `halo_state` | Durable, auditable, operator-governable | More implementation work | HAL long-lived persona system |
| Import `comsim` runtime | Reuses earlier ideas quickly | Wrong persistence/runtime model | Experimental prototype only |

## 6. Recommended Decision

- **Chosen Option:** `Option B: Add Claims, Temporal State, Subsystem Profiles, and Admin Surfaces in halo_state`
- **Summary:** `halo_state` will remain the system of record. We will add append-only claims, subsystem-family records, diary continuity, and temporal snapshots as first-class Postgres-backed state. `halo-chat` will gain a dedicated Admin tab so operators can inspect, configure, and tune these capabilities from the Windows web client rather than by editing state indirectly through conversations or raw SQL.
- **Rationale:** This option reuses the strong parts of `comsim` at the architectural level while staying aligned with HAL's existing Postgres-centered state system. It also solves the missing operational layer by giving the client a supported administration surface.
- **Key Trade-Offs:** We accept greater schema and UI complexity in exchange for stronger continuity, safer governance, and better long-term extensibility.

**VALIDATION REQUIRED – Critical Decision**

The exact scope of manual operator editing in the Admin tab must be constrained carefully. Direct mutation of active beliefs, relationships, and subsystem state without provenance could degrade model trustworthiness and retrieval quality.

## 7. Architectural Impact

### System Overview (Textual Diagram)

1. `halo-chat` sessions generate `memory_events` through normal chat and commit workflows.
2. A promotion service derives `memory_claims`, entity mentions, and relationship candidates from committed sessions.
3. A reconciliation layer projects current working beliefs and active relationship state from append-only claims.
4. Diary and temporal snapshot services persist authored continuity and time-anchored state.
5. Retrieval assembles event, claim, belief, relationship, diary, and subsystem context for prompt building.
6. `halo-chat-api` exposes read/write administration endpoints.
7. The Windows web client exposes those capabilities through a new `Admin` tab.

### Interfaces & Contracts

New or expanded service capabilities are expected in:

- `halo_state` Python service module
- `halo-chat-api` administration endpoints
- `halo-chat` web client API bindings and UI panels

Proposed endpoint families:

- `GET /v1/admin/memory/entities`
- `GET /v1/admin/memory/claims`
- `GET /v1/admin/memory/beliefs`
- `GET /v1/admin/memory/relationships`
- `GET /v1/admin/memory/diaries`
- `GET /v1/admin/subsystems`
- `POST /v1/admin/subsystems`
- `PATCH /v1/admin/subsystems/{id}`
- `POST /v1/admin/claims/reconcile`
- `POST /v1/admin/diaries`
- `POST /v1/admin/temporal/snapshot`
- `GET /v1/admin/config/memory`
- `PATCH /v1/admin/config/memory`

These are indicative contracts, not final route commitments.

### Data Model Impact

Additions to `db/halo_state_postgres.sql`:

- `memory_claims`
- `subsystem_profiles`
- `entity_diary_entries`
- `temporal_state_snapshots`

Likely changes to existing retrieval and promotion logic:

- `select_memory_context` must move beyond `memory_events`-only summarization
- promotion must create claims and relationship candidates from committed sessions
- beliefs must become derived current-state records rather than the sole structured evidence layer

### Runtime & Operational Impact

- More Postgres reads per prompt-build
- Additional promotion/reconciliation jobs after commit
- More administrative surfaces in the chat API and UI
- Higher need for clear telemetry, audit logging, and operator safeguards

## 8. Risks & Mitigations

### Technical Risks

- **Risk:** Claim extraction quality is poor.
  - **Mitigation:** Start with conservative extraction for identities, family relations, institutions, locations, and subsystem lineage only.

- **Risk:** Retrieval becomes noisy or over-constraining.
  - **Mitigation:** Rank by confidence, recency, and status; surface conflicts explicitly rather than flattening them.

- **Risk:** Beliefs drift into immutable pseudo-canon.
  - **Mitigation:** Keep claims append-only and beliefs revisable; require provenance for manual changes.

### Operational Risks

- **Risk:** Admin UI enables unsafe live edits.
  - **Mitigation:** Make initial write actions narrow, explicit, and audited. Prefer curation and reconciliation over free-form mutation.

- **Risk:** Operators overfit subsystem-family lore.
  - **Mitigation:** Treat family-state as soft belief memory with confidence and status, not canonical identity fact.

### Security & Compliance Risks

- **Risk:** Sensitive identity or diary records are exposed broadly.
  - **Mitigation:** Separate operator/admin surfaces from normal chat surfaces and gate write operations.

### Fallback / Rollback Plan

- Feature-flag all new retrieval layers.
- Allow prompt-building to fall back to current event-centric retrieval if claim/diary/subsystem retrieval is disabled.
- Keep Admin write actions disableable independently from Admin read actions.

## 9. Implementation Plan

### Phase 1: Schema and Service Foundations

- Add `memory_claims`
- Add `subsystem_profiles`
- Add `entity_diary_entries`
- Add `temporal_state_snapshots`
- Add indexes and provenance constraints
- Add service-layer models and read/write helpers in `halo_state`

### Phase 2: Promotion and Reconciliation

- Extend commit promotion from `halo-chat`
- Extract conservative entity, relationship, and identity claims
- Reconcile claims into `memory_beliefs`
- Derive active relationship state from claim history

### Phase 3: Retrieval and Prompting

- Extend `select_memory_context`
- Add relevant diary and subsystem continuity retrieval
- Add current world-time anchor to prompt building
- Add conflict-aware relationship context formatting

### Phase 4: Chat API Administration

- Add administration endpoints to `halo-chat-api`
- Add audit logging for admin actions
- Add config endpoints for retrieval/promotion tuning

### Phase 5: Windows Web Client Administration

- Add a new `Admin` tab to the `halo-chat` web client alongside `Workflow`, `Voice`, `Tools`, and `Alerts`
- Add admin subpanels for:
  - Memory
  - Relationships
  - Subsystems
  - Diaries
  - Temporal State
  - Tuning
- Provide read-first visibility before enabling high-impact writes

### Owners / Responsible Parties

- `halo_state` schema and service layer: HAL system maintainers
- `halo-chat-api` administration endpoints: Halo chat maintainers
- `halo-chat` web client Admin tab: Halo chat frontend maintainers

## 10. Metrics & Validation

### Success Metrics

- New-session recall accuracy for recently committed identity facts improves materially
- Relationship recall and stance continuity are visible in prompted responses
- Operators can inspect subsystem-family state without database access
- Prompt retrieval latency remains acceptable for interactive chat
- No increase in unsafe or inconsistent memory mutation rates

### Validation Plan

- schema migration tests
- promotion unit tests for claim extraction and reconciliation
- retrieval tests covering conflicts, supersession, and diary inclusion
- API tests for admin endpoints and audit behavior
- web tests for Admin tab rendering, filtering, and guarded actions
- end-to-end Windows local launch validation using the Postgres-backed web client

## 11. Open Questions

- **Blocking:** Should `relationship_claims` be its own table or a constrained view/predicate family over `memory_claims`?
- **Blocking:** What is the minimum safe set of Admin write actions for the first release?
- **Blocking:** How should subsystem persona text be versioned and audited?
- **Non-Blocking:** Should diary entries support private/operator-only visibility classes?
- **Non-Blocking:** Should subjective time be implemented in phase 1 or deferred until subsystem wake/suspend behavior exists?

## 12. Appendix

### Chat Client Requirements

The Windows-hosted `halo-chat` web client must gain a dedicated `Admin` tab.

Initial Admin capabilities should include:

- viewing entities, claims, beliefs, relationships, diary entries, and subsystem profiles
- filtering by status, confidence, recency, and source session
- viewing promotion diagnostics for committed chat sessions
- viewing temporal anchor state and recent snapshots
- tuning retrieval and promotion settings behind explicit save actions

Initial Admin safeguards should include:

- confirmation prompts for write actions
- immutable provenance display
- disabled controls when API/admin capability is unavailable
- clear separation between observational data and operator overrides

### Client Change Summary

For `halo-chat`:

- extend the panel union in `web/src/App.tsx`
- add API client methods in `web/src/api.ts`
- add Admin panel UI and styles in `web/src/App.tsx` and `web/src/index.css`
- add tests for admin rendering, permissions/availability states, and settings save flows

This RFC does not require immediate implementation of all Admin write surfaces. It does require the UI and API structure to be designed with administration as a first-class concern.

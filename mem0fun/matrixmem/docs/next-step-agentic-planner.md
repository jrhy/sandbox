# Next-Step Agentic Planner

This note records the invariants that the current `matrixmem` v1 preserves today, the gaps that still exist, and the constraints we should keep if we turn this into a planner that revisits memories and suggests actions over time.

## What V1 Preserves Today

### Raw source retention

The bot writes every handled Matrix event to an append-only JSONL archive before any Mem0-dependent work runs.

Each archive record currently contains:

- `room_id`
- `event_id`
- `timestamp`
- `text`
- `sender_id`
- `handling_mode`

This is the strongest planner-enabling invariant in the current implementation because it preserves the exact handled text that the bot saw when prompts, extraction rules, or models change.

It is not a complete Matrix event archive. Today we only persist handled `m.text` messages, and we do not retain edits, reactions, thread structure, or other richer Matrix event context.

### Source-linked derived memories

The Mem0 wrapper stores stable source metadata on every ingested memory:

- `handling_mode`
- `source_room_id`
- `source_event_id`
- `source_sender_id`
- `source_timestamp`

In practice, retrieved vector memories currently come back with this source metadata in the real harness path, which makes them traceable back to the archived source event.

That provenance is still mediated by Mem0's storage and retrieval behavior, though. We attach the metadata on ingest, but we do not independently enforce or verify retrieval-time provenance beyond the behavior we observed in manual real-mode verification.

### Stable retrieval scope

Query retrieval is scoped to `source_room_id`, so a later planner can reason over one room's memory without silently mixing in memories from unrelated rooms.

### Small stable metadata surface

The current implementation does not yet populate richer planner fields such as `kind`, `status`, `energy`, or `time_horizon`, but it does normalize arbitrary metadata keys into a stable snake_case form before storing them.

That means we already have a structurally safe place to add planner-facing metadata without needing a new storage shape.

It does not mean retrieval semantics will stay unchanged. As soon as planner fields such as `kind`, `status`, `energy`, or `location` start being used as query filters, retrieval behavior will change by design.

### Deterministic interaction rule

The room contract is still intentionally simple:

- messages with `?` are queries
- messages without `?` are memory input

That matters for a planner because it keeps the source stream legible. A future revisiting agent can distinguish raw captured thoughts from explicit questions without needing to infer intent from the archive later.

## What V1 Does Not Yet Guarantee

### Graph-edge provenance is not verified

We currently configure Mem0 with Kuzu graph memory, but we do not yet expose or verify per-edge source attribution from our own code.

So:

- raw event provenance exists
- vector memory provenance exists
- graph-edge provenance should be treated as an open gap, not an established invariant

Before an agentic planner starts revisiting or mutating graph relationships automatically, we should close this gap or be explicit that graph edges are advisory rather than canonical.

### Planner metadata is not populated yet

The current bot does not extract or enforce:

- `kind`
- `status`
- `energy`
- `time_horizon`
- `location`

Those were part of the design direction, but not part of the implemented v1 storage contract yet.

The planner doc should therefore treat them as next-step extensions, not as existing reliable fields.

### Real-mode verification is still manual

We now have:

- automated mock-mode harness coverage
- manual real-mode harness verification against Mem0, Qdrant, Kuzu, and Ollama
- manual restart/persistence verification

That is enough for v1 confidence, but not yet enough for unattended planner development. A future planner should ideally inherit at least one automated real-stack regression path.

## Invariants To Preserve For Planner Work

Any planner-oriented work should preserve these properties:

1. Raw event archival must happen before model-dependent extraction.
2. Derived memories must keep source event attribution.
3. Room scoping must remain explicit at retrieval time.
4. Metadata keys added for planning should remain stable once introduced.
5. New edge types should begin as proposals or reviewed additions, not automatic canonical mutations.

The fifth point is especially important. The current system is still trustworthy partly because it is conservative. A planner that freely invents durable relation types would make the graph harder to reason about and harder to repair.

## Safe Next Steps

The next planner-oriented increments should be:

1. Add explicit extracted metadata fields for `kind`, `status`, `energy`, and `location`.
2. Verify what Mem0's Kuzu path actually stores for graph relations and whether source attribution can be recovered or supplemented.
3. Add a real-stack regression harness mode that can run unattended, even if it stays slower than mock mode.
4. Introduce planner suggestions as read-only recommendations before allowing any autonomous memory or graph mutations.

That path keeps the current bot useful as a primary interface while preserving the "agentic planner" as the next-step aspiration rather than collapsing into premature autonomy.

# rulesgen coexists with the Python rule generator; decision state deliberately diverges

Rule generation gains a web service, `rulesgen`, that hosts a persistent Review Queue backed by a decision store (Cluster Decisions: Generated / Declined / Ignored / Snoozed per Cluster per rule-type lane). The Python script (`bin/postmanpat-generate-rules.py`) stays maintained as the offline/SSH path. The two tools keep separate records of what has been decided — the script's file-based Generation Checkpoint versus the service's Cluster Decisions — and those records **deliberately diverge**: deciding in one tool does not suppress prompting in the other.

## Considered Options

- **The script reads the service's decision store** (one-way sync of decided cluster IDs into the checkpoint): rejected — it couples a standalone offline tool to the service's storage, and the divergence cost (occasional re-prompting when falling back to the script) is low because rulesgen is the primary path.
- **Retire the script at rulesgen's launch**: rejected — a proven offline fallback is wanted while the service proves itself.
- **Shared state file both tools write**: rejected — two writers with different schemas and lifecycles is the worst of both worlds.

## Consequences

- Rule-building logic is maintained in two implementations (Go in rulesgen, Python in the script) at strict parity: same fields, same defaults, no enrichment that exists in only one tool. (The one sanctioned extension: generated cleanup rules offer `age_window.min` — empty default for One-Time Cleanup, `30d` for Ongoing Cleanup.)
- The glossary now distinguishes the two records explicitly: Generation Checkpoint (script; what was *asked*) versus Cluster Decision (rulesgen; what was *decided*).

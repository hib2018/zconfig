<!--
Sync Impact Report
- Version change: 1.0.0 -> 2.0.0
- Modified principles: None
- Modified sections:
  - Technical and Security Constraints: human-readable review output responsibility moved from the
    Zig core to the product as a whole; the core retains structured I/O and human-readable diagnostics
- Added sections: None
- Removed sections: None
- Follow-up TODOs: None
-->
# zconfig Constitution

## Core Principles

### I. Human Authority Is Final
Every configuration mutation MUST require an explicit human approval of the final diff before it is
written. Natural-language feedback, suggested values, and replies to review comments MUST NOT count
as approval. Agents MAY inspect configuration, explain fields, and prepare revisions, but MUST NOT
bypass the approval boundary or directly mutate the source configuration. This separation ensures
that automation assists human judgment without silently replacing it.

### II. Structured and Verifiable Changes
Agent output MUST cross the zconfig boundary as a structured change proposal, not as an
unrestricted file rewrite. Each mutation MUST identify its target path, operation, expected old
value, proposed new value, and stable review identifier. Application MUST fail if the expected old
value no longer matches the current document. Schema-declared constraints are authoritative;
inferences from comments, documentation, or agent reasoning MAY inform proposals but MUST be marked
as unverified and MUST NOT be presented as machine-validated facts.

### III. Lossless, Scoped Editing
A change MUST preserve all content outside its approved targets, including comments, whitespace,
quoting style, ordering, and other format-specific representation. An adapter MUST reject a change
when it cannot demonstrate this preservation; whole-document reserialization is not an acceptable
fallback. JSON, TOML, and YAML are first-class target formats and each format adapter MUST meet the
same preservation rule before it is described as supported. This protects human-maintained context
and keeps review diffs limited to intentional changes.

### IV. Portable Core, Optional Integrations
The initial core MUST be implemented in Zig and MUST expose deterministic, documented structured
I/O suitable for use without an LLM or network connection. Go MAY be introduced for user interfaces,
servers, MCP support, agent adapters, or other integration-heavy layers when that reduces delivery
and maintenance cost. Provider-specific integrations MUST remain optional adapters; the core MUST
NOT require a particular model vendor, hosted service, or agent runtime. A second language MUST be
introduced only with a documented boundary and concrete benefit.

### V. Incremental Simplicity
Before version 1.0, the project MUST prefer the smallest design that proves the review and safe-apply
workflow. Features, abstractions, and compatibility layers MUST have a current, demonstrated use
case. Breaking changes before 1.0 are permitted, but changes to the CLI, proposal format, or adapter
contract MUST include a recorded rationale and migration instructions. Tests MUST cover every
implemented safety invariant and every defect fix. This keeps experimentation deliberate rather
than allowing either premature compatibility or unconstrained rewrites.

## Technical and Security Constraints

- Supported configuration formats are JSON, TOML, and YAML, but a format MUST NOT be advertised as
  supported until its adapter passes lossless round-trip and targeted-edit tests.
- The product MUST support human-readable review output and a stable machine-readable representation.
  Human-readable review output MAY be provided by an optional interface layer and MAY use tables,
  diffs, and stable change IDs. The Zig core MUST expose deterministic structured I/O plus
  human-readable help and diagnostics, but MUST NOT duplicate the product's interactive review UI.
- Secret or sensitive values MUST be redacted from human-readable output, logs, and agent requests
  by default. Revealing them MUST require an explicit, narrowly scoped action.
- Writes MUST use a failure-safe strategy that cannot leave a partially written configuration.
  Material writes MUST be recoverable through a backup or an equivalent atomic mechanism.
- Schemas are the authoritative source for declared types, constraints, descriptions, and defaults.
  Absence of a schema MUST be visible as an unverified state rather than silently treated as success.
- Network access MUST NOT be required for parsing, validation, review, or application of an already
  prepared proposal.

## Development Workflow and Quality Gates

Every change MUST be traceable from a human request or approved specification to tests and a reviewable
diff. Before a proposal can be applied, zconfig MUST always verify that both source and result parse,
that expected old values still match, that no unapproved content changed, and that a human explicitly
approved the final diff.

Schema validation, lossless-preservation checks, and target-application validation commands MUST run
when defined or available. A failed check MUST block application. A check that cannot run because no
schema or external validator is configured MUST be displayed as `unverified`, never `passed`, and the
human approver MUST see that status before approval. Lossless preservation is not optional for a
format advertised as supported under Principle III.

Changes to parsing, serialization, proposal semantics, approval handling, redaction, or atomic writes
MUST include automated tests for success, rejection, and stale-input cases. Cross-language boundaries
MUST have contract tests. Review output MUST distinguish clearly among `passed`, `failed`, and
`unverified` checks.

## Governance

This constitution governs all project specifications, plans, tasks, implementation, and reviews.
When another project document conflicts with it, this constitution takes precedence.

Amendments MUST be proposed as an explicit constitution change, document the motivation and migration
impact, update the Sync Impact Report, and receive human approval. The constitution uses semantic
versioning: MAJOR for incompatible removal or redefinition of governance, MINOR for a new principle or
material expansion, and PATCH for non-semantic clarification. Pre-1.0 product compatibility policy
does not weaken constitution versioning.

Every feature specification and implementation review MUST identify the applicable principles and
verify their quality gates. Any exception MUST be documented before implementation with its scope,
risk, compensating control, and expiry condition; convenience alone is not sufficient. Reviewers MUST
reject unexplained complexity, unverified safety claims, and any path that bypasses explicit human
approval.

**Version**: 2.0.0 | **Ratified**: 2026-09-07 | **Last Amended**: 2026-09-09

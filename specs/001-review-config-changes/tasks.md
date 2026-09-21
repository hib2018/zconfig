---

description: "Dependency-ordered tasks for Review Configuration Changes"
---

# Tasks: Review Configuration Changes

**Input**: Design documents from `specs/001-review-config-changes/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`

**Tests**: Tests are mandatory because the constitution requires automated coverage for every safety
invariant, protocol boundary, and defect fix. Within each phase, test tasks precede implementation.

**Organization**: Tasks are grouped by user story so each story produces an independently testable
increment after shared setup and foundational work.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel because it touches different files and has no dependency on another
  incomplete task in the same group.
- **[Story]**: Maps the task to a user story from `spec.md`.
- Every task names its exact target file or directory.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish reproducible Zig and Go projects, generated-contract inputs, and CI scaffolding.

- [X] T001 Create the planned `core/src/`, `core/tests/fixtures/`, `tui/cmd/zconfig/`, `tui/internal/`, `tests/contract/fixtures/`, `tests/integration/`, and `tests/fixtures/` directory structure
- [X] T002 Initialize the Zig 0.16.0 build with core executable and test targets in `build.zig` and `build.zig.zon`
- [X] T003 [P] Initialize the Go 1.27.1 workspace and modules with pinned Bubble Tea, Bubbles, and Lip Gloss dependencies in `go.work`, `tui/go.mod`, `tui/go.sum`, and `tests/go.mod`
- [X] T004 [P] Add Zig formatting, test, and release-build jobs for Linux, macOS, and Windows in `.github/workflows/zig-ci.yml`
- [X] T005 [P] Add Go formatting, vet, test, and race-test jobs for Linux, macOS, and Windows in `.github/workflows/go-ci.yml`
- [X] T006 Add generated binaries, project-local review sessions, recovery files, and temporary test artifacts to `.gitignore`

**Checkpoint**: Both language projects have reproducible local and CI entry points, even though tests
may still contain only harness checks.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Implement the strict process boundary and shared domain types required by every story.

**⚠️ CRITICAL**: No user story implementation begins until this phase is complete.

- [X] T007 [P] Create valid and invalid envelope and operation-payload golden fixtures from every schema in `specs/001-review-config-changes/contracts/` under `tests/contract/fixtures/messages/`
- [X] T008 [P] Create valid and invalid proposal, redacted-value, and external-validator fixtures in `tests/contract/fixtures/proposals/` and `tests/contract/fixtures/validators/`
- [X] T009 Write Zig contract tests across all operation schemas for framing, duplicate fields, unknown fields, versions, request IDs, typed redaction, and 16 MiB limits in `core/tests/contract.zig`
- [X] T010 Write Go contract tests for the same complete golden acceptance matrix in `tui/internal/protocol/protocol_test.go`
- [X] T011 Implement strict request, success, error, capability, operation-payload, typed-redaction, and output-sanitization types in `core/src/protocol.zig`
- [X] T012 Implement matching strict protocol types, size-limited decoding, typed-redaction, and output sanitization in `tui/internal/protocol/protocol.go`
- [X] T013 Implement `protocol_info` capability negotiation and core command dispatch in `core/src/main.zig`
- [X] T014 Implement UI-independent source, proposal, change-item, sensitivity, typed-redaction, check-result, comment, decision, audit-event, and session types in `tui/internal/review/model.go`
- [X] T015 Implement process execution without a shell, separate bounded streams, deadlines, cancellation, and exit diagnostics in `tui/internal/runner/process.go`
- [X] T016 [P] Write process harness tests for timeout, cancellation, extra stdout, oversized streams, wrong request IDs, non-zero exit, and secret removal from diagnostics in `tui/internal/runner/process_test.go`
- [X] T017 Implement agent and optional validator registration, command validation, working directory, environment allowlist, and timeout defaults in `tui/internal/config/commands.go`
- [X] T018 Wire CLI argument parsing, core discovery, agent selection, and protocol handshake without starting the full-screen UI in `tui/cmd/zconfig/main.go`

**Checkpoint**: Go and Zig agree on all golden protocol documents, subprocesses are bounded, and no
shell string can cross the execution boundary.

---

## Phase 3: User Story 1 - Understand a Proposed Configuration Change (Priority: P1) 🎯 MVP

**Goal**: Open a standard JSON source and external proposal, validate them, and present every change
item and its verification status in a keyboard-operable terminal review.

**Independent Test**: Open a fixture proposal with add, replace, and remove operations and verify that
a maintainer can identify every path, old value, proposed value, operation, description, and check
status with and without an optional schema; exiting must leave the source unchanged.

### Tests for User Story 1

- [X] T019 [P] [US1] Write JSON Pointer tests covering root, escaped keys, arrays, append, invalid indices, duplicate keys, and missing targets in `core/tests/pointer_test.zig`
- [X] T020 [P] [US1] Write lexical-index tests for nested values, Unicode escapes, number spellings, whitespace, and exact byte spans in `core/tests/source_index_test.zig`
- [X] T021 [P] [US1] Write proposal validation tests for IDs, source digest, operation/value combinations, duplicate targets, and ancestor conflicts in `core/tests/proposal_test.zig`
- [X] T022 [P] [US1] Write schema-subset, `x-zconfig-sensitive`, normalized sensitive-name, and unsupported-keyword tests in `core/tests/schema_test.zig` and `core/tests/redaction_test.zig`
- [X] T023 [P] [US1] Write reducer and view tests for navigation, resize, narrow terminals, empty/error states, check labels, and default redaction before first render in `tui/internal/ui/review_test.go`
- [X] T024 [US1] Implement strict RFC 8259 parsing, duplicate-name rejection, SHA-256 identity, node and file checks, and sensitive-name classification in `core/src/document.zig` and `core/src/redact.zig`
- [X] T025 [US1] Implement RFC 6901 parsing and source-node resolution in `core/src/pointer.zig`
- [X] T026 [US1] Implement the lexical source-span index and core-computed add/replace/remove edit envelopes in `core/src/source_index.zig`
- [X] T027 [US1] Implement proposal parsing, uniqueness and overlap rules, expected-old checks, and normalized check results in `core/src/proposal.zig`
- [X] T028 [US1] Implement the documented JSON Schema subset, `x-zconfig-sensitive` annotation, and unsupported-keyword reporting in `core/src/schema.zig`
- [X] T029 [US1] Add `inspect_source` and `validate_proposal` operations that classify values and emit only typed-redacted structured results in `core/src/main.zig`
- [X] T030 [US1] Implement the review reducer, filtering, selection, detail view, source diff view, help, scrolling, resize, and monochrome fallback in `tui/internal/review/reducer.go` and `tui/internal/ui/review.go`
- [X] T031 [US1] Connect `zconfig review <proposal> --source <file> [--schema <file>]` to core validation and the read-only TUI in `tui/cmd/zconfig/main.go`

**Checkpoint**: US1 is a demonstrable MVP. Users can understand and navigate a validated proposal;
there is still no comment, revision, approval, or write path.

---

## Phase 4: User Story 2 - Request a Focused Revision (Priority: P2)

**Goal**: Attach comments to change items, invoke one registered agent, validate its response, and
accept only revisions confined to the submitted comment set.

**Independent Test**: Comment on two items and invoke the fixture agent; a valid revision becomes the
active proposal, modified items return to pending, unchanged decisions remain, and any out-of-scope
revision is rejected atomically.

### Tests for User Story 2

- [X] T032 [P] [US2] Create fixture-agent modes for success, scope expansion, timeout, malformed JSON, secret echo, and non-zero exit in `tests/fixtures/agent/main.go`
- [X] T033 [P] [US2] Write comment lifecycle and revision-state reducer tests in `tui/internal/review/revision_test.go`
- [X] T034 [P] [US2] Write core revision contract tests for allowed IDs, immutable fields, stale bases, scope violations, and atomic rejection in `core/tests/revision_test.zig`
- [X] T035 [P] [US2] Write end-to-end valid and out-of-scope agent revision tests in `tests/integration/revision_flow_test.go`
- [X] T036 [US2] Implement create, edit, withdraw, agent-claimed, and human-confirmed comment transitions in `tui/internal/review/comments.go`
- [X] T037 [US2] Implement construction of bounded `revise_proposal` requests containing only authorized comment targets in `tui/internal/protocol/revision.go`
- [X] T038 [US2] Implement `validate_revision`, immutable-field comparison, whole-revision rejection, and `revision.scope_expansion_required` handling in `core/src/proposal.zig`
- [X] T039 [US2] Implement registered-agent capability handshake, one-shot invocation, cancellation, retry-safe errors, and result delivery in `tui/internal/runner/agent.go`
- [X] T040 [US2] Implement comment editor, revision progress, error recovery, old-versus-new proposal comparison, and resolution confirmation views in `tui/internal/ui/revision.go`
- [X] T041 [US2] Integrate accepted revision state so only modified items return to pending and unmodified decisions persist in `tui/internal/review/reducer.go`

**Checkpoint**: US2 can refine a proposal through focused comments without permitting the agent to
expand scope or write the configuration.

---

## Phase 5: User Story 3 - Approve and Safely Apply the Final Change (Priority: P3)

**Goal**: Record item decisions, save and resume reviews, assemble an exact final change set, require
a fresh final confirmation, and apply it without partial writes or stale overwrites.

**Independent Test**: Approve two of three items, reject one, save and resume, confirm the final diff,
and apply; only approved targets change, unrelated bytes remain identical, and stale/interrupted cases
leave a complete recoverable source.

### Tests for User Story 3

- [X] T042 [P] [US3] Write byte-preservation golden tests for add, replace, remove, arrays, indentation, CRLF, escapes, and multi-edit ordering in `core/tests/preservation.zig`
- [X] T043 [P] [US3] Write final-set tests for pending items, unresolved comments, decision subsets, check status, nonce expiry, and digest binding in `core/tests/final_set_test.zig`
- [X] T044 [P] [US3] Write failure-injection tests for temp creation, write, flush, permission, replacement, recovery, locked files, and interruption in `core/tests/apply_failure.zig`
- [X] T045 [P] [US3] Test session round trips, version handling, non-restored grants, value-free audit JSONL, sequence/digest chains, and append-failure blocking in `tui/internal/session/store_test.go`
- [X] T046 [P] [US3] Write end-to-end subset approval, resume, stale-source, and recovery scenarios in `tests/integration/apply_recovery_test.go`
- [X] T047 [US3] Implement item approval, rejection, visible bulk approval, invalidation rules, and readiness computation in `tui/internal/review/decisions.go`
- [X] T048 [US3] Implement versioned sessions and value-free audit events with restrictive permissions, schema validation, digest chains, and failure gating in `tui/internal/session/store.go` and `audit.go`
- [X] T049 [US3] Implement same-directory temporary composition, exact non-envelope byte verification, permission preservation, source rehash, replacement, and recovery record handling in `core/src/apply.zig`
- [X] T050 [US3] Implement `assemble_final` with approved-item selection, reparsing, configured checks, exact diff, final-set digest, and short-lived confirmation nonce in `core/src/main.zig`
- [X] T051 [US3] Implement `apply_final` with one-use nonce verification and structured success or recovery outcome in `core/src/main.zig`
- [X] T052 [US3] Implement decision controls, affected-item bulk confirmation, readiness blockers, final diff, distinct apply confirmation, and recovery messaging in `tui/internal/ui/approval.go`
- [X] T053 [US3] Wire autosave at stable transitions, resume-time source revalidation, confirmation clearing, and interrupted-apply reconciliation in `tui/internal/review/reducer.go`
- [X] T054 [US3] Implement external-validator handshake and failure rules, bounded execution, redacted digest-bound results, and flow exit codes in `tui/cmd/zconfig/main.go` and `tui/internal/runner/validator.go`

**Checkpoint**: US3 provides the complete non-secret review lifecycle and is safe against stale input,
unapproved edits, protocol replay, and interrupted application.

---

## Phase 6: User Story 4 - Protect Sensitive Settings (Priority: P4)

**Goal**: Detect, redact, reveal, and one-time-share sensitive values without persisting or accidentally
logging them.

**Independent Test**: Review schema-declared and name-detected secrets, verify default redaction across
all outputs, explicitly reveal/share one item, and prove permissions disappear on use, exit, crash,
and resume.

### Tests for User Story 4

- [X] T055 [P] [US4] Write one-use reveal/share authorization, expiry, fingerprint, remasking, and false-positive interaction tests in `core/tests/redaction_test.zig` and `tui/internal/review/secrets_test.go`
- [X] T056 [P] [US4] Write UI tests for already-concealed, suspected, revealed, share-confirmation, and expired states in `tui/internal/ui/sensitive_test.go`
- [X] T057 [P] [US4] Write integration tests scanning TUI output, protocol payloads, stdout, stderr, sessions, and audit events for disclosed fixture secrets in `tests/integration/secret_leak_test.go`
- [X] T058 [US4] Extend foundational typed redaction with session-bound fingerprints and explicit reveal/share authorization validation in `core/src/redact.zig`
- [X] T059 [US4] Verify and harden foundational redaction across every core success, error, diff, check, and diagnostic serialization path in `core/src/protocol.zig`
- [X] T060 [US4] Implement in-memory reveal and one-use share capabilities with automatic consumption and session-end destruction in `tui/internal/review/secrets.go`
- [X] T061 [US4] Implement exact-value remasking of agent stdout, stderr, errors, and candidate results before display or persistence in `tui/internal/runner/agent.go`
- [X] T062 [US4] Implement sensitive-item indicators, reveal confirmation, one-use share warning, and immediate conceal controls in `tui/internal/ui/sensitive.go`
- [X] T063 [US4] Extend session and audit serialization tests to reject or redact injected secret values in `tui/internal/session/store_test.go`

**Checkpoint**: All four user stories work with sensitive values hidden by default and narrowly scoped
human-controlled exceptions.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Verify portability, performance, operator documentation, and final constitution compliance.

- [X] T064 [P] Add representative standard JSON, schema, proposal, duplicate-key, CRLF, Unicode, large-scale, and fixed 20-item usability fixtures with an answer key in `tests/fixtures/`
- [X] T065 [P] Add protocol compatibility tests that run every golden document through both implementations in `tests/contract/compatibility_test.go`
- [X] T066 [P] Add 10 MiB, 100,000-node, 1,000-change load and navigation benchmarks in `core/tests/performance.zig` and `tui/internal/ui/benchmark_test.go`
- [-] T067 SKIPPED BY PROJECT DECISION: native replacement validation on Linux and Windows is not practical at the current project stage; macOS results remain documented in `specs/001-review-config-changes/platform-validation.md`
- [X] T068 Add installation, agent registration, supported schema subset, key bindings, recovery, and security-limit documentation in `README.md` and `docs/security.md`
- [-] T069 SKIPPED BY PROJECT DECISION: five-person first-time-user usability testing is not practical at the current project stage; automated coverage remains recorded in `validation-results.md`
- [X] T070 Re-run constitution gates, confirm all unsupported file types fail closed, and document any approved exceptions in `specs/001-review-config-changes/validation-results.md`
- [X] T071 Require a prior preview token instead of a boolean confirmation flag in `tui/cmd/zconfig/main.go`
- [X] T072 Bind confirmation capabilities to proposal, decisions, comments, checks, source, and final candidate in `core/src/final_set.zig`
- [X] T073 Revalidate the complete candidate against the configured schema during preview and immediately before apply in `core/src/main.zig`
- [X] T074 Load or create an apply review session and pass its comment states and decisions through the final protocol in `tui/cmd/zconfig/main.go`
- [X] T075 Gate final confirmation and application on value-free append-only audit events in `tui/cmd/zconfig/main.go`
- [X] T076 Canonicalize external-validator candidate and input paths independently of validator working directory in `tui/cmd/zconfig/main.go`
- [X] T077 Add regression coverage for fabricated/stale tokens, comment-state changes, final schema validation, audit output, and validator working directories in `core/tests/` and `tests/integration/`
- [X] T078 Update operator and protocol documentation for session-bound two-step confirmation in `README.md`, `docs/workflow.md`, and `contracts/protocol.md`

**Checkpoint**: The feature meets its cross-platform, performance, protocol, preservation, recovery,
and security acceptance criteria.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies; T003–T005 can run in parallel after T001 identifies paths.
- **Foundational (Phase 2)**: Depends on Phase 1 and blocks every user story.
- **US1 (Phase 3)**: Depends on Phase 2 and creates the inspect/validate/read-only review MVP.
- **US2 (Phase 4)**: Depends on Phase 2 and reuses US1 presentation when integrated; its core revision
  validation and fixture agent can be developed independently of the US1 UI.
- **US3 (Phase 5)**: Depends on Phase 2; final TUI integration depends on US1 presentation and US2
  comment-resolution state, while apply-core tests and implementation can begin independently.
- **US4 (Phase 6)**: Depends on Phase 2; it may be developed alongside US1–US3, but release integration
  requires every output and persistence path from those stories.
- **Polish (Phase 7)**: Depends on all user stories selected for release.

### User Story Dependency Graph

```text
Setup -> Foundation -> US1 (read-only MVP)
                    -> US2 core/agent revision
                    -> US3 apply core
                    -> US4 redaction core

US1 + US2 -> focused revision UI
US1 + US2 + US3 -> complete review/apply flow
US1 + US2 + US3 + US4 -> releasable secure feature
```

### Within Each User Story

- Write the listed tests first and confirm they fail for the expected missing behavior.
- Implement domain rules before protocol operations that expose them.
- Implement core operations before connecting their TUI actions.
- Complete the independent test at the checkpoint before starting dependent integration.
- Do not mark a safety task complete when only happy-path tests pass.

### Parallel Opportunities

- Setup jobs for Go dependencies and platform CI can proceed in parallel.
- Zig and Go protocol tests can be written in parallel against the immutable contracts.
- Within US1, pointer, lexical-index, proposal, schema, and UI reducer tests touch separate files.
- Within US2, the fixture agent, reducer tests, core scope tests, and integration test can be prepared
  in parallel before implementation.
- Within US3, preservation, final-set, filesystem failure, persistence, and end-to-end test harnesses
  are independent preparations.
- US4's core classification, UI-state, and leak-scanning tests can be written in parallel.
- Once Phase 2 completes, separate contributors can build US1, the US2 core, the US3 apply core, and
  the US4 classification core concurrently, then integrate in priority order.

## Parallel Example: User Story 1

```text
Task: "T019 Write JSON Pointer tests in core/tests/pointer_test.zig"
Task: "T020 Write lexical-index tests in core/tests/source_index_test.zig"
Task: "T021 Write proposal validation tests in core/tests/proposal_test.zig"
Task: "T022 Write schema-subset tests in core/tests/schema_test.zig"
Task: "T023 Write reducer and view tests in tui/internal/ui/review_test.go"
```

## Parallel Example: User Story 2

```text
Task: "T032 Create fixture-agent modes in tests/fixtures/agent/main.go"
Task: "T033 Write comment and revision reducer tests in tui/internal/review/revision_test.go"
Task: "T034 Write core revision contract tests in core/tests/revision_test.zig"
Task: "T035 Write revision integration tests in tests/integration/revision_flow_test.go"
```

## Parallel Example: User Story 3

```text
Task: "T042 Write byte-preservation tests in core/tests/preservation.zig"
Task: "T043 Write final-set and nonce tests in core/tests/final_set_test.zig"
Task: "T044 Write filesystem failure-injection tests in core/tests/apply_failure.zig"
Task: "T045 Write session persistence tests in tui/internal/session/store_test.go"
Task: "T046 Write apply-flow integration tests in tests/integration/apply_recovery_test.go"
```

## Parallel Example: User Story 4

```text
Task: "T055 Write core redaction tests in core/tests/redaction_test.zig"
Task: "T056 Write sensitive-state UI tests in tui/internal/ui/sensitive_test.go"
Task: "T057 Write secret-leak integration tests in tests/integration/secret_leak_test.go"
```

## Implementation Strategy

### MVP First: User Story 1

1. Complete Setup and Foundational phases.
2. Complete US1 tests and implementation.
3. Stop and validate read-only review independently.
4. Demonstrate that a maintainer can understand a 20-item proposal without raw-patch interpretation.
5. Do not add write capability until this checkpoint passes.

### Incremental Delivery

1. **US1**: Read-only inspect and review proves the core presentation value.
2. **US2**: Scoped comments and revisions reduce late-stage natural-language burden.
3. **US3**: Explicit approval, persistence, final confirmation, and failure-safe apply make the tool
   operationally useful.
4. **US4**: End-to-end redaction and one-use sharing make the complete workflow releasable for real
   configuration files.
5. **Polish**: Validate scale and filesystem semantics on every supported operating system.

### Parallel Team Strategy

After Phase 2, work can divide along stable contracts:

- Contributor A: US1 parsing/indexing and review UI.
- Contributor B: US2 agent adapter and scope validation.
- Contributor C: US3 apply/recovery and US4 redaction foundations.

Integration still occurs in priority order so each checkpoint remains demonstrable.

## Notes

- `[P]` means the task can be assigned concurrently without editing the same file or depending on
  unfinished work in its group.
- `[US1]` through `[US4]` provide direct traceability to `spec.md`.
- Each test task must fail for the intended missing behavior before its implementation task starts.
- Commits should group one task or one tightly coupled test/implementation pair.
- Do not implement JSONC, TOML, YAML, zintent integration, remote collaboration, or multiple agents
  under this task list.

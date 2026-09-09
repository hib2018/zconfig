# Implementation Plan: Review Configuration Changes

**Branch**: `001-review-config-changes` (Spec Kit feature context; Git checkout remains `main`) |
**Date**: 2026-09-09 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/001-review-config-changes/spec.md`

## Summary

Build a late-stage configuration review tool with a Zig core and a Go terminal frontend. The Zig
core validates standard JSON, indexes exact source spans, validates versioned proposals, performs
schema-subset checks, enforces revision scope and approval gates, and applies only approved edits
through failure-safe replacement. The Go frontend presents change items, persists non-secret review
state, invokes the Zig core and one registered external agent as bounded subprocesses, and requires
explicit final confirmation.

The two executables communicate through a versioned, single-request/single-response JSON protocol on
standard input and output. This keeps the safety boundary usable without a network, an LLM, or the Go
frontend and gives future zintent integration a provider-neutral contract.

## Technical Context

**Language/Version**: Zig 0.16.0 for the core; Go 1.27.1 for the TUI and process orchestration

**Primary Dependencies**: Zig standard library; Bubble Tea v2.0.8 with compatible Bubbles and Lip
Gloss v2 modules; Go standard library for subprocesses and persistence

**Storage**: Source JSON files remain in place; resumable review sessions use versioned project-local
files under `.zconfig/reviews/`; value-free append-only audit events use one JSON Lines file per
session under `.zconfig/audit/`; agent registration uses the operating system's per-user config area

**Testing**: `zig build test`, `go test ./...`, golden byte-preservation fixtures, versioned contract
fixtures, failure-injection integration tests, and native filesystem tests on Linux, macOS, and Windows

**Target Platform**: Local macOS and Linux terminals plus Windows 10/11 with Windows Terminal;
keyboard-only operation with monochrome and narrow-terminal fallbacks

**Project Type**: Dual-executable local CLI/TUI with a reusable offline core

**Performance Goals**: Open and validate a 10 MiB JSON file with up to 1,000 change items in under
2 seconds on a typical developer machine; keep navigation response below 100 ms; assemble and apply
an already approved final change set in under 2 seconds, excluding optional external validation

**Constraints**: No network requirement for core operations; maximum 16 MiB per protocol message;
10-second Zig-core timeout and configurable 120-second default agent timeout; exact preservation of
bytes outside core-computed edit envelopes; no shell command evaluation; no persisted secret values
or temporary reveal grants

**Scale/Scope**: One regular local JSON file per review, at most 10 MiB, 100,000 JSON nodes, 1,000
change items, 10 revisions, and 5,000 audit events. JSONC, TOML, YAML, remote/multi-user review,
multiple simultaneous agents, symlink targets, and non-regular files are outside this feature.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle or constraint | Design evidence | Status |
|-------------------------|-----------------|--------|
| Human Authority Is Final | Item decisions never imply application; the final rendered change set requires a fresh confirmation immediately before apply. | PASS |
| Structured and Verifiable Changes | All operations carry IDs, JSON Pointers, expected old values, and an exact source digest through versioned schemas. | PASS |
| Lossless, Scoped Editing | The core retains original bytes, computes lexical source spans, and rejects any output whose non-edit-envelope bytes differ. | PASS |
| Portable Core, Optional Integrations | Safety logic is an offline Zig executable; Go and external agents communicate only through optional subprocess adapters. | PASS |
| Incremental Simplicity | The first feature supports one standard JSON file and one agent; JSONC, TOML, YAML, streaming, and remote collaboration are deferred. | PASS |
| Secret redaction | Foundational protocol types reject raw secrets at persistence/output boundaries; source inspection classifies and redacts values before the first TUI display. | PASS |
| Failure-safe writes | Apply uses a same-directory temporary file, sync, backup/recovery record, and replacement; failures retain a complete recoverable version. | PASS |
| Quality gates | Parser, stale-source, schema-subset, scope, preservation, contract, cancellation, filesystem failure, and fixed-scenario usability cases have dedicated tests or acceptance protocols. | PASS |

No constitution exceptions are required.

### Post-Design Re-check

Phase 1 artifacts preserve every gate. The data model makes approval and confirmation separate
states, contracts reject scope expansion and unknown major versions, and the quickstart includes
negative validation for stale files, secrets, malformed agent output, and interrupted writes.

## Project Structure

### Documentation (this feature)

```text
specs/001-review-config-changes/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── usability-test.md
├── contracts/
│   ├── protocol.md
│   ├── message.schema.json
│   ├── proposal.schema.json
│   ├── protocol-info.schema.json
│   ├── inspection.schema.json
│   ├── revision.schema.json
│   ├── final-change.schema.json
│   ├── apply.schema.json
│   ├── validation-result.schema.json
│   ├── audit-event.schema.json
│   └── external-validator.schema.json
└── tasks.md
```

### Source Code (repository root)

```text
build.zig
build.zig.zon
core/
├── src/
│   ├── main.zig
│   ├── protocol.zig
│   ├── document.zig
│   ├── source_index.zig
│   ├── pointer.zig
│   ├── proposal.zig
│   ├── schema.zig
│   ├── redact.zig
│   └── apply.zig
└── tests/
    ├── fixtures/
    ├── contract.zig
    ├── preservation.zig
    └── apply_failure.zig

tui/
├── go.mod
├── go.sum
├── cmd/zconfig/main.go
└── internal/
    ├── ui/
    ├── review/
    ├── protocol/
    ├── runner/
    ├── session/
    └── config/

tests/
├── contract/
│   ├── fixtures/
│   └── compatibility_test.go
├── integration/
│   ├── review_flow_test.go
│   ├── stale_source_test.go
│   ├── agent_failure_test.go
│   └── apply_recovery_test.go
└── fixtures/
    ├── configs/
    ├── schemas/
    └── proposals/
```

**Structure Decision**: Use two deliberately narrow projects. `core/` owns all parsing, semantic
checks, redaction classification, proposal/revision validation, and source mutation. `tui/` owns only
presentation, review-state reduction, session persistence, and subprocess lifecycle. Shared concepts
cross the boundary solely through files in `contracts/`; no C ABI or duplicated safety logic is used.

## Complexity Tracking

No constitution violations require justification. The second language is expressly allowed for an
integration-heavy UI and is isolated behind contract tests.

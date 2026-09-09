# Phase 0 Research: Review Configuration Changes

## Zig baseline

**Decision**: Pin the core to Zig 0.16.0 and upgrade only through an explicit compatibility change.

**Rationale**: It is the current stable release and is already installed in the development
environment. Zig remains pre-1.0 and 0.16 introduces a new I/O interface, so floating across compiler
versions would make filesystem and protocol code unstable.

**Alternatives considered**: Zig 0.15.x was rejected as an older baseline; tracking development
snapshots was rejected because safety-critical filesystem behavior must be reproducible.

## Lossless JSON editing

**Decision**: Use the Zig standard JSON facilities for RFC 8259 syntax and semantic validation, but
retain the original byte buffer and build a dedicated lexical source-span index. Resolve each target
using RFC 6901 JSON Pointer, compute the smallest safe edit envelope, and apply non-overlapping edits
from the end of the file toward the beginning.

**Rationale**: Parsing into a generic value and serializing it again changes whitespace, number and
escape spelling, and layout. A span index lets the core prove that bytes outside approved edit
envelopes are identical.

**Alternatives considered**: Whole-document serialization violates the constitution. A third-party
JSON CST dependency was rejected for the initial standard-JSON scope. Dotted paths were rejected
because keys containing dots and array indices are ambiguous.

## JSON input restrictions

**Decision**: Accept UTF-8 RFC 8259 JSON only and reject duplicate object member names globally,
invalid Unicode, comments, and trailing commas.

**Rationale**: Duplicate names produce inconsistent lookup behavior across parsers and make a target
path ambiguous. Strict input also makes Go/Zig contract fixtures deterministic.

**Alternatives considered**: Selecting the first or last duplicate was rejected as unsafe. JSONC is
a future independent adapter.

## Source freshness and application

**Decision**: Identify the source by SHA-256 of its exact bytes and also compare each operation's
semantic expected-old value. Immediately before commit, re-read and re-hash the source. Compose the
new file in a temporary regular file in the same directory, preserve tested basic permissions, flush
it, retain a recovery copy or record, then replace the target.

**Rationale**: A digest catches concurrent edits including formatting-only changes; the expected value
provides a review-friendly second guard. Same-directory replacement avoids partial in-place writes.

**Alternatives considered**: Modification time and file size can miss changes. Truncating the source
before writing can corrupt it. Cross-filesystem temporary files cannot provide reliable replacement.

**Risk**: Atomic visibility and crash durability differ by OS and filesystem. Initial support is
limited to regular local files validated by native tests. Symlinks, ACL/xattr preservation, network
filesystems, and unusual Windows sharing modes fail closed until separately supported.

## TUI framework and Go baseline

**Decision**: Pin Go 1.27.1 and Bubble Tea v2.0.8 with compatible Bubbles and Lip Gloss v2 modules.
Keep the state reducer independent of terminal rendering.

**Rationale**: Bubble Tea's model/update/view flow maps directly to review state transitions and its
v2 line provides resize, paste, keyboard, and color-profile handling. Go's subprocess and cancellation
support suits orchestration while leaving safety rules in Zig.

**Alternatives considered**: A hand-built Zig terminal UI would merge presentation complexity into
the safety core. A low-level Go cell library would require more event, focus, and layout code. A web
UI adds serving and browser security concerns that the local workflow does not need.

## Terminal compatibility

**Decision**: Test macOS, Linux, and modern Windows Terminal natively. All actions must be available
from conservative keyboard bindings. Mouse, true color, and enhanced keyboard protocols are optional;
the UI must remain readable in monochrome, 8-color, narrow, and resized terminals.

**Rationale**: Terminal feature support varies locally, through SSH, and on Windows. Core review and
approval cannot depend on decorative capabilities.

**Alternatives considered**: Requiring a specific terminal was rejected because it would undermine
the intended portable developer workflow.

## Core and agent process protocol

**Decision**: For each operation, spawn a non-interactive process, send exactly one UTF-8 JSON request
on stdin, accept exactly one JSON response on stdout, and reserve stderr for redacted diagnostics.
Use a two-level version: `protocol_version` for the envelope and named schema versions for payloads.

**Rationale**: Initial operations are atomic and need no streaming progress. A one-shot process has
simple cancellation, isolation, and recovery. Request IDs correlate responses, diagnostics, and audit
records. Unknown protocol major versions and malformed or extra output are rejected before state
changes.

**Alternatives considered**: JSON Lines and RFC 7464 sequences are useful for future streaming but
add partial-message state. JSON-RPC adds notification and batching semantics not needed here. A
long-running core requires crash recovery and state synchronization.

## Protocol compatibility

**Decision**: Version 1 envelopes require exact known fields at security boundaries. New optional
payload fields require a new compatible schema identifier; semantic changes or new required fields
require a new major schema. Consumers reject unknown major versions and negotiate capabilities with
a `protocol_info` operation before first use.

**Rationale**: Strict decoding catches agent hallucinations and misspellings, while explicit schema
identifiers make intentional evolution reviewable before 1.0.

**Alternatives considered**: Silently accepting arbitrary unknown fields weakens validation. Versioning
only the executable gives persisted sessions no reliable migration key.

## Subprocess security and timeouts

**Decision**: Register agents as an executable path plus argument array; never evaluate a shell
command string. Use a bounded environment, explicit working directory, capped stdin/stdout/stderr,
cancellation, and configurable deadlines. Defaults are 10 seconds for the Zig core and 120 seconds
for an external agent. A timed-out or malformed response is discarded atomically.

**Rationale**: Direct process execution avoids shell expansion and quoting ambiguity. Bounded I/O and
time prevent an adapter from hanging or exhausting the TUI.

**Alternatives considered**: Shell commands were rejected because configuration could become code.
Unlimited output and indefinite waits were rejected as denial-of-service risks.

## Revision-scope enforcement

**Decision**: Revision requests list the exact allowed change IDs. Returned items must retain their
ID, path, operation, expected old value, and source digest; only proposed values and agent explanation
may change. Any addition, removal, or mutation outside that set rejects the whole revision. An agent
that needs broader changes returns `revision.scope_expansion_required` without a revision.

**Rationale**: This implements the user's explicit decision that comments authorize revision only of
their target items.

**Alternatives considered**: Highlighting unrelated changes still increases cognitive load. Partial
acceptance can hide a malformed revision and was rejected.

## Sensitive values

**Decision**: Represent redaction as a typed object, never as a magic replacement string. Classify
values from schema annotations and conservative normalized setting-name rules. Never store values or
temporary grants in session files. An explicit one-use share grant may place only named values in one
agent request; agent stdout and stderr are re-redacted before display or persistence.

**Rationale**: Typed redaction distinguishes a concealed value from a literal value such as `******`.
One-use grants match the session-scoped authorization rule and limit accidental propagation.

**Alternatives considered**: Trusting proposal sensitivity flags was rejected because proposals are
untrusted input. Passing secrets in arguments or environment variables was rejected because they can
be exposed through process inspection and logs.

## Review-session persistence

**Decision**: Persist versioned project-local session documents at stable transitions: comment save,
accepted revision, decision change, and clean exit. Store no live-process state, final confirmation,
secret value, or reveal/share grant. On load, migrate only recognized compatible versions and recheck
the current source digest.

**Rationale**: The review survives interruptions without restoring ephemeral authority. Project-local
state keeps a review associated with its source; `.zconfig/` should be ignored by version control by
default.

**Alternatives considered**: Memory-only review contradicts the resume requirement. A database adds
recovery and migration complexity without multi-user queries.

## Minimal local audit trail

**Decision**: Append one strictly typed, value-free JSON event per line to
`.zconfig/audit/<session-id>.jsonl`. Record identities, actor type, action, time, affected change IDs,
outcome codes, and source/proposal digests, with each event linked to the preceding event digest.
Never record configuration or proposed values, comment bodies, secret metadata, or raw subprocess
output. Do not automatically delete audit files in v1.

**Rationale**: JSON Lines remains readable after an interrupted append and is easy for both Zig and
Go to validate. A digest chain detects edits or missing events within the retained chain without
introducing signing keys or a service. Blocking authoritative actions when their audit append fails
prevents an approval or apply from succeeding without its corresponding trace.

**Alternatives considered**: Storing full comments or redacted value snapshots was rejected because
free text and metadata can disclose secrets. A database, cryptographic signatures, remote collection,
and automatic retention policies are deferred because v1 is local and single-user.

## References

- [Zig 0.16.0 release notes](https://ziglang.org/download/0.16.0/release-notes.html)
- [Go release history](https://go.dev/doc/devel/release)
- [Bubble Tea releases](https://github.com/charmbracelet/bubbletea/releases)
- [Go process execution](https://pkg.go.dev/os/exec)
- [RFC 8259: JSON](https://www.rfc-editor.org/rfc/rfc8259)
- [RFC 6901: JSON Pointer](https://www.rfc-editor.org/rfc/rfc6901)
- [RFC 6902: JSON Patch](https://www.rfc-editor.org/rfc/rfc6902)
- [RFC 7464: JSON Text Sequences](https://www.rfc-editor.org/rfc/rfc7464)
- [JSON Schema Draft 2020-12](https://json-schema.org/draft/2020-12)

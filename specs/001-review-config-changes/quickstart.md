# Quickstart Validation Guide: Review Configuration Changes

This guide defines the end-to-end scenarios the implemented feature must pass. Command names are the
planned user-facing shape and may be wired by tasks generated after this plan.

## Prerequisites

- Zig 0.16.0
- Go 1.27.1 or a compatible 1.27 patch release
- A terminal on macOS, Linux, or Windows Terminal
- A built `zconfig-core` executable and Go `zconfig` TUI executable
- A registered fixture agent that implements `contracts/protocol.md`

From the repository root:

```console
zig build test
cd tui && go test ./...
cd .. && go test ./tests/...
```

All commands must pass before running manual scenarios.

## Fixture setup

Create or select fixtures corresponding to:

- `tests/fixtures/configs/app.json`: formatted standard JSON with nested objects and arrays
- `tests/fixtures/schemas/app.schema.json`: supported constraints, descriptions, and one secret field
- `tests/fixtures/proposals/app.proposal.json`: three items using add, replace, and remove
- `tests/fixtures/agent/fixture-agent`: deterministic protocol-compatible revision adapter

Copy the source fixture into a temporary project directory so validation never mutates repository
fixtures. Register the fixture agent as an executable plus literal argument list, not a shell command.

## Scenario 1: Inspect and review a proposal

```console
zconfig review ./app.proposal.json --source ./app.json --schema ./app.schema.json
```

Expected outcome:

- The TUI lists exactly three stable change items.
- Each item shows path, operation, current value, proposed value, decision, comments, and checks.
- Schema descriptions and supported constraint results appear as verified.
- Keyboard navigation, help, scrolling, and resize work without a mouse.
- Exiting without final confirmation does not change `app.json`.

Repeat without `--schema`. Semantic checks must show `unverified`, not `passed`, and review must remain
possible.

## Scenario 2: Comment and accept a scoped revision

In the TUI, add a comment to one item requesting a different value and choose revision.

Expected outcome:

- Only the commented item's ID is authorized in the request.
- The fixture agent receives one JSON request and returns one JSON response.
- The accepted revision changes only the requested value or explanation.
- The modified item returns to `pending`.
- Decisions on unchanged items remain intact.
- The agent's resolution claim remains unconfirmed until the maintainer confirms it.

Configure the fixture agent to modify an unlisted item and retry. The whole revision must be rejected
with `revision.scope_violation`; the active proposal and source must remain unchanged.

## Scenario 3: Save and resume

Add comments and item decisions, then exit normally. Restart the same review.

Expected outcome:

- Proposal revision, comments, decisions, and non-sensitive check history are restored.
- The source digest is rechecked before the session becomes active.
- Final confirmation is absent after resume.
- Secret values, reveal grants, and share grants are absent from `.zconfig/reviews/`.

Modify one whitespace byte in `app.json` before a second resume. The review must report a stale source
and block revision and application until refreshed.

## Scenario 4: Approve a subset and apply

Approve two items, reject one, confirm all comments, and request a final preview. Inspect the exact
diff, then perform the separate final-confirmation action.

Expected outcome:

- Only approved items appear in the final change set.
- The final preview supplies a short-lived confirmation bound to its digest.
- Apply reparses and rehashes the current source immediately before replacement.
- Only the two approved settings change.
- Every byte outside core-computed edit envelopes is identical to the original.
- The resulting JSON parses and passes all configured supported schema checks.
- The review records success without storing unredacted secrets.
- The session audit JSONL records the final-confirmation and apply outcome using IDs, action codes,
  and digests only; it contains no configuration values, proposed values, comment bodies, or process
  output, and its sequence and digest chain verify successfully.

Make the audit path unwritable and repeat an approval and an apply attempt. Both authoritative
actions must be blocked without changing the source; ordinary navigation and comment editing may
continue with a visible audit warning.

Repeat but edit the source after preview and before apply. Apply must reject the expired/stale
confirmation and leave the externally edited source untouched.

## Scenario 5: Sensitive-value handling

Use one field marked sensitive by schema and another whose normalized name contains `password` or
`token`.

Expected outcome:

- Both values are redacted in the TUI, normal protocol payloads, diagnostics, audit events, and saved
  sessions.
- The name-detected field is labeled suspected rather than schema-verified.
- Reveal affects only the selected item and current session.
- Share requires an additional explicit action and applies to one agent invocation only.
- Exiting and resuming clears both permissions.
- Known disclosed values returned through agent stdout or stderr are redacted before display or save.

## Scenario 6: Process and protocol failures

Run fixture-agent modes for timeout, cancellation, oversized output, log text on stdout, duplicate
JSON keys, wrong request ID, unknown major version, malformed response, and non-zero exit.

Expected outcome for every mode:

- No source bytes change.
- No candidate revision becomes active.
- The existing review can be retried or saved.
- The user receives a stable error code and redacted explanation.
- The child process does not remain running after the configured cancellation window.

## Scenario 7: File replacement failures

Run native platform tests that inject temporary-file, flush, permission, replacement, and recovery
failures. Include a read-only destination and, on Windows, a locked destination.

Expected outcome:

- The source is always a complete old or complete approved version, never a partial file.
- A prior version or recovery path remains available when apply reports failure.
- Symlinks, directories, devices, and unsupported filesystem cases fail before mutation.

## Scenario 8: Scale and responsiveness

Generate a 10 MiB standard JSON fixture containing up to 100,000 nodes and a valid proposal with
1,000 non-overlapping change items.

Expected outcome on the documented reference developer machine:

- Initial load and validation complete within 2 seconds.
- Navigation reactions remain below 100 ms.
- Final assembly and apply complete within 2 seconds, excluding external validators.
- No request, response, or diagnostic stream may exceed 16 MiB.

## Contract compatibility gate

Validate all golden request, success, and error fixtures against the applicable operation schema in:

- `contracts/*.schema.json`

Use `contracts/message.schema.json` for the common envelope, then use its `payload_schema` value to
select the concrete request or result schema. Apply the versioning, size, process, and external
validator rules in `contracts/protocol.md` in addition to JSON Schema validation.

Run each golden fixture through both Go and Zig decoders. They must agree on acceptance and rejection,
including duplicate names, unknown fields, number handling, JSON Pointer escaping, and version errors.

## Usability acceptance gate

After automated and manual functional scenarios pass, execute `usability-test.md` with the fixed
20-item fixture and at least five representative first-time users. Release requires at least 95%
aggregate comprehension, completion within 10 active minutes per participant, zero critical
interaction errors, and a median of at least 4/5 on every defined confidence/load question.

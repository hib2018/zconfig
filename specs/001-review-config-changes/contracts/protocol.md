# Process Protocol v1

## Transport

Every core or agent operation is one non-interactive child-process invocation:

1. Parent writes exactly one UTF-8 RFC 8259 JSON value to stdin and closes stdin.
2. Child writes exactly one JSON response to stdout and closes stdout.
3. Human diagnostics use stderr and must already be redacted.
4. Any non-whitespace bytes after the response, invalid UTF-8, duplicate object names, unknown fields,
   oversized output, timeout, or cancellation make the entire invocation fail.

The maximum request, response, and captured stderr size is 16 MiB each. The default core deadline is
10 seconds; the default agent deadline is 120 seconds. Implementations may configure lower limits.
There is no streaming, batch, notification, or persistent connection in v1.

## Invocation security

Adapters are registered as an executable plus a literal argument array. Implementations must not use
a shell, expand variables, interpret globbing, or construct a command string. The working directory
and inherited environment names are explicit. Secret values are sent only in stdin after one-use
authorization; they never appear in arguments or environment variables.

## Envelope

Requests conform to [message.schema.json](message.schema.json):

```json
{
  "protocol_version": "1.0",
  "request_id": "01JXYZ...",
  "operation": "validate_proposal",
  "payload_schema": "zconfig.proposal/1",
  "payload": {}
}
```

Success and error are mutually exclusive:

```json
{
  "protocol_version": "1.0",
  "request_id": "01JXYZ...",
  "ok": true,
  "result_schema": "zconfig.validation-result/1",
  "result": {}
}
```

```json
{
  "protocol_version": "1.0",
  "request_id": "01JXYZ...",
  "ok": false,
  "error": {
    "code": "revision.scope_violation",
    "message": "Revision changed an item outside the submitted comment set.",
    "retryable": false,
    "details": [{"pointer": "/items/2", "reason": "change_not_allowed"}]
  }
}
```

The response `request_id` must exactly match the request. A retry gets a new request ID and may carry
an `attempt_of` reference in its payload.

Operation payloads and results are normative in these schemas:

- [protocol-info.schema.json](protocol-info.schema.json)
- [inspection.schema.json](inspection.schema.json)
- [proposal.schema.json](proposal.schema.json)
- [revision.schema.json](revision.schema.json)
- [final-change.schema.json](final-change.schema.json)
- [apply.schema.json](apply.schema.json)
- [validation-result.schema.json](validation-result.schema.json)
- [audit-event.schema.json](audit-event.schema.json)
- [external-validator.schema.json](external-validator.schema.json)

## Versioning

- `protocol_version` versions envelope and transport behavior as `major.minor`.
- `payload_schema` and `result_schema` independently version document shapes as `name/major`.
- Unknown major versions are rejected before the operation executes.
- Unknown fields are rejected in v1 at both envelope and payload boundaries.
- Adding an optional field requires a newly documented compatible schema revision; adding a required
  field, removing a field, or changing meaning requires a major version.
- `protocol_info` has no state-changing behavior and returns supported protocol versions, operations,
  document schemas, limits, and capabilities.

## Core operations

| Operation | Payload | Result | State-changing |
|-----------|---------|--------|----------------|
| `protocol_info` | `zconfig.empty/1` | `zconfig.protocol-info/1` | No |
| `inspect_source` | source path and optional schema path | source identity, indexed settings, classifications | No |
| `validate_proposal` | `zconfig.proposal/1` plus source path | normalized proposal and check results | No |
| `validate_revision` | active proposal, allowed IDs, returned revision | accepted candidate or structured error | No |
| `assemble_final` | proposal plus human decisions | final preview, diff, and check results | No |
| `apply_final` | final change set, fresh confirmation nonce, source path | new digest and recovery outcome | Yes |

Configured external validators use `external-validator.schema.json`. They receive a path to a
same-directory candidate file plus its digest, never an unapproved source mutation. A missing
validator is `unverified`; timeout, non-zero exit, malformed output, digest mismatch, or an explicit
failed result blocks final assembly. Validator output is untrusted, bounded, and redacted before it
is converted into a `CheckResult`.

The core emits a short-lived confirmation nonce only with the final preview. Any changed source,
proposal, decision, comment, check, process restart, or session resume invalidates it. The core accepts
the nonce once and only for the exact final change-set digest.

## Agent operations

| Operation | Payload | Result |
|-----------|---------|--------|
| `protocol_info` | `zconfig.empty/1` | supported versions and `revise_proposal` capability |
| `revise_proposal` | proposal lineage, allowed item IDs, comments, redacted source/schema context | candidate revision or structured error |

The revision payload contains `allowed_change_item_ids`. Returned items must preserve proposal ID,
source digest, path, operation, expected old value, and every unlisted item. Only proposed values and
explanations of listed items may change. Scope expansion must return
`revision.scope_expansion_required`; it must not return an expanded proposal.

## Redacted values

A concealed value is typed data, not a replacement string:

```json
{
  "visibility": "redacted",
  "secret_ref": "secret-7",
  "value_type": "string",
  "fingerprint": "hmac-sha256:..."
}
```

Normal requests contain no secret value. A confirmed `share_once` action may add a separate
`authorized_secrets` table for named change IDs to exactly one agent request. It is never persisted,
logged, retried automatically, or reused. Agent stdout and stderr are scanned for known disclosed
values and redacted before presentation or persistence.

## Stable error families

- `protocol.*`: invalid encoding, envelope, version, request ID, size, or extra output
- `source.*`: missing, unsupported file type, duplicate key, parse failure, or stale digest
- `proposal.*`: malformed ID, pointer, operation, value, overlap, or source mismatch
- `schema.*`: invalid schema, failed supported constraint, or unsupported applicable keyword
- `revision.*`: scope violation, stale base revision, unresolved response, or expansion required
- `approval.*`: pending item, unresolved comment, or missing/expired final confirmation
- `apply.*`: permission, temporary write, sync, replacement, or recovery failure
- `agent.*`: unavailable executable, timeout, cancellation, invalid response, or non-zero exit

Errors must use a stable code, redacted human message, retryability flag, and optional JSON Pointer
details. Error messages are not a compatibility contract.

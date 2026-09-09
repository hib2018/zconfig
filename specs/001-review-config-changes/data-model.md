# Data Model: Review Configuration Changes

## Conventions

- IDs are opaque UTF-8 strings unique within their parent review.
- Configuration paths are RFC 6901 JSON Pointers.
- Digests use `sha256:<lowercase-hex>` over exact source bytes.
- Persisted timestamps use UTC RFC 3339 strings.
- Persisted documents carry an explicit schema/version identifier.
- Secret values and temporary reveal/share grants never appear in persisted entities.

## Source Configuration

Represents the one regular local JSON file under review.

| Field | Meaning | Validation |
|-------|---------|------------|
| `path` | User-selected source location | Absolute, regular local file; symlinks rejected in v1 |
| `digest` | Identity of exact source bytes | SHA-256; rechecked before every state-changing operation |
| `byte_length` | Original size | 0–10 MiB |
| `node_count` | Indexed JSON values | At most 100,000 |
| `root_type` | JSON root type | Any RFC 8259 value; objects are the common case |
| `schema_ref` | Optional schema location and digest | Must identify a valid supported schema document |

The source retains an immutable byte buffer during each core invocation. A lexical index maps every
reachable JSON Pointer to a value span and, where needed for add/remove, a core-computed edit envelope.
Source spans never cross the process contract.

## Change Proposal

An unapproved collection of externally supplied operations against one source identity.

| Field | Meaning | Validation |
|-------|---------|------------|
| `proposal_id` | Stable proposal lineage ID | Required; unchanged across revisions |
| `revision` | Monotonic revision number | Starts at 1; increments exactly by one |
| `source_digest` | Exact source identity | Must equal the active source digest |
| `items` | Ordered change items | 1–1,000 unique IDs |
| `created_at` | Creation time | RFC 3339 |
| `producer` | Informational producer identity | Must not grant trust or authority |

Proposal order is for presentation only. Application order is derived from source spans and dependency
checks. A returned revision is accepted atomically or rejected completely.

## Change Item

The smallest review and decision unit.

| Field | Meaning | Validation |
|-------|---------|------------|
| `change_id` | Stable review identity | Unique in proposal; retained across revisions |
| `path` | Target setting | RFC 6901 pointer; immutable across revisions |
| `operation` | Mutation kind | `add`, `replace`, or `remove`; immutable across revisions |
| `expected_old` | Semantic old value | Required for replace/remove; absent for add |
| `proposed_value` | Semantic new value | Required for add/replace; absent for remove |
| `explanation` | Agent-provided rationale | Informational and unverified unless schema-derived |
| `decision` | Human item decision | `pending`, `approved`, or `rejected` |
| `sensitivity` | Display classification | `normal`, `schema_sensitive`, or `suspected_sensitive` |
| `checks` | Latest validation results | One result per applicable check type |

Two items may not target the same pointer. Ancestor/descendant target combinations are rejected because
their edit semantics and independent approval are ambiguous. `add` to an array may use `/-`; other
array positions use canonical non-negative decimal indices.

## Review Comment

Human feedback scoped to one change item.

| Field | Meaning | Validation |
|-------|---------|------------|
| `comment_id` | Stable comment ID | Unique within review |
| `change_id` | Target item | Must exist in active proposal |
| `body` | Human feedback | Non-empty UTF-8, bounded by the protocol limit |
| `status` | Resolution state | `open`, `agent_claimed`, or `human_confirmed` |
| `created_at` | Creation time | RFC 3339 |
| `updated_at` | Last human edit | At or after creation |

Only a human action can enter `human_confirmed`. Editing a confirmed comment returns it to `open`.

## Proposal Revision

The candidate successor returned for a specific revision request.

| Field | Meaning | Validation |
|-------|---------|------------|
| `proposal_id` | Proposal lineage | Must match active proposal |
| `base_revision` | Revision being revised | Must equal active revision |
| `source_digest` | Source identity | Must match active source |
| `allowed_change_ids` | Authorized revision scope | Exactly the set of submitted commented items |
| `items` | Returned candidate items | Same IDs, paths, and operations as active proposal |
| `resolved_comment_ids` | Agent resolution claims | Subset of submitted comment IDs |

Only `proposed_value` and `explanation` may differ for allowed items. Any other difference is a
`revision.scope_violation`; the active proposal and review state remain unchanged. Accepted modified
items become `pending`; unchanged item decisions remain intact.

## Check Result

| Field | Meaning | Validation |
|-------|---------|------------|
| `kind` | Check category | Stable registered name |
| `status` | Outcome | `passed`, `failed`, or `unverified` |
| `code` | Machine-readable detail | Stable dotted identifier |
| `message` | Redacted human explanation | Must not contain secret values |
| `pointer` | Optional affected location | RFC 6901 pointer |
| `evidence_digest` | Optional evidence identity | Digest only, never raw secret evidence |

Mandatory checks include parse, source freshness, expected-old match, revision scope, edit-envelope
preservation, and final confirmation. Schema and external validators are mandatory when configured;
their absence is `unverified` rather than failed.

## Review Session

Persisted state for one proposal lineage.

| Field | Meaning | Validation |
|-------|---------|------------|
| `session_schema` | Persistence format | Known major version required |
| `review_id` | Session identity | Unique project-locally |
| `source` | Source reference | Path, digest, size; no source contents |
| `active_proposal` | Current accepted proposal revision | Exactly one |
| `comments` | Review comments | At most 5,000 combined audit events |
| `decision_history` | Human decisions | Redacted and append-only |
| `revision_history` | Prior proposal metadata | Values redacted when sensitive |
| `check_history` | Non-sensitive results | Revalidated after resume |
| `lifecycle` | Persisted lifecycle state | `reviewing`, `ready`, `applied`, or `abandoned` |
| `updated_at` | Last stable transition | RFC 3339 |

The session deliberately excludes final confirmation, live subprocess state, secret values, and
reveal/share grants. Resume always starts without those capabilities and rechecks the source digest.

## Audit Event

One value-free JSON object appended as a single line to `.zconfig/audit/<session-id>.jsonl`.

| Field | Meaning | Validation |
|-------|---------|------------|
| `audit_schema` | Event format | `zconfig.audit-event/1` |
| `event_id` | Stable event identity | Unique within the session |
| `session_id` | Owning review | Must match the audit filename/session |
| `sequence` | Event order | Starts at 1 and increases by exactly 1 |
| `occurred_at` | Event time | RFC 3339 |
| `actor_type` | Initiator category | `human`, `agent`, or `system` |
| `action` | Stable action code | Closed set defined by the contract |
| `change_ids` | Affected items | IDs only; empty when not item-specific |
| `source_digest` | Applicable source identity | Digest only |
| `proposal_digest` | Applicable proposal identity | Digest only; optional |
| `outcome_code` | Machine-readable result | Optional; no free-form output |
| `previous_event_digest` | Link to preceding event | `null` only for sequence 1 |
| `event_digest` | Digest of canonical event fields | Excludes this field itself |

Audit events never contain configuration values, proposed values, comment bodies, secret metadata,
or subprocess stdout/stderr. Comment actions identify only the comment and change IDs. During normal
operation events are append-only; the digest chain detects modification or removal inside the
available chain but does not claim to prevent deletion of the complete local file. An audit append
failure blocks authoritative actions (approval, secret sharing, final confirmation, and apply) but
allows non-authoritative review and comment editing with a visible warning.

## Agent Registration

User-local configuration naming one external agent adapter.

| Field | Meaning | Validation |
|-------|---------|------------|
| `name` | Display name | Unique in user config |
| `executable` | Program path or resolvable name | Stored separately from arguments |
| `args` | Literal argument array | No shell parsing or expansion |
| `working_directory` | Explicit execution directory | Existing directory or project root default |
| `environment_allowlist` | Names of inherited variables | Minimal explicit list |
| `timeout_seconds` | Revision deadline | Positive; default 120 |
| `protocol_major` | Required adapter protocol | Must equal supported major after handshake |

## Ephemeral Authorization

Lives only in memory for one active TUI session.

| Field | Meaning | Validation |
|-------|---------|------------|
| `change_id` | Authorized sensitive item | Must be currently visible and sensitive |
| `capability` | `reveal` or `share_once` | Explicit human action required |
| `consumed` | Whether one-use share occurred | Share becomes invalid immediately after use |

All ephemeral authorizations are destroyed on exit, crash recovery, or resume.

## State Transitions

```text
proposal loaded
    -> reviewing
       -> comment added -> awaiting revision -> reviewing
       -> item approved/rejected -> reviewing
       -> all items decided + comments confirmed -> ready
    -> ready
       -> any item/comment/revision change -> reviewing
       -> final confirmation (ephemeral) -> applying
    -> applying
       -> checks pass + replacement succeeds -> applied
       -> any failure -> ready (confirmation cleared)
    -> reviewing/ready -> abandoned
```

On resume, `reviewing`, `ready`, `applied`, or `abandoned` may be restored, but `applying` and final
confirmation never are. An interrupted apply is reconciled from the source digest and recovery record
before the session can continue.

---
name: "zconfig-extract-settings"
description: "Extract user-adjustable settings from a target repository's JSON config files, JSON Schemas, and reference code into editable table-friendly JSON without changing configs or generating zconfig proposals."
compatibility: "Requires read access to the target repository; optional JSON Schema and source-code references improve evidence quality."
metadata:
  author: "hib2018/zconfig"
  output_schema: "zconfig.settings-extraction/1"
---

## Purpose

Use this Skill when the user asks to inspect a repository and extract configurable display, behavior, operations, or environment settings for review/editing before any zconfig proposal exists.

This Skill is an extraction workflow only:

- Do not modify configuration files.
- Do not generate a zconfig proposal.
- Do not ask the human to choose files or items midway.
- Do not invent choices, ranges, recommended values, or descriptions without evidence.
- Do not output secret values.

## zconfig contract context

Current proposal contract is `zconfig.proposal/1` (`specs/001-review-config-changes/contracts/proposal.schema.json`):

- top-level required fields: `proposal_id`, `revision`, `source_digest`, `created_at`, `items`
- each change item requires `change_id`, `path`, `operation`, `explanation`
- operations are exactly `add`, `replace`, `remove`
- `replace` requires `expected_old` and `proposed_value`
- `add` requires `proposed_value` and forbids `expected_old`
- `remove` requires `expected_old` and forbids `proposed_value`

This Skill must not emit that proposal shape. It emits the inventory below so a table UI can review/edit candidates before a separate proposal-generation step.

The inspection/redaction rules should stay aligned with zconfig core:

- JSON Pointer paths use RFC 6901 style (`/a/0/b`; root is `""`).
- Secret/sensitive values are redacted when schema has `x-zconfig-sensitive: true`, an ancestor schema is sensitive, or the final pointer token normalizes to a sensitive name: `password`, `passwd`, `secret`, `token`, `accessToken`, `refreshToken`, `apiKey`, `apiToken`, `privateKey`, `clientSecret`, `credential`, `credentials`.
- For redacted rows, include type and evidence, but omit the raw current value.

## Workflow

1. Locate target repository root from user input or current working directory. If the named repository is not the current directory, inspect the available sibling/known checkout path before failing.
2. Read existing zconfig docs/contracts if present: `README.md`, `docs/`, `specs/**/contracts/proposal.schema.json`, `specs/**/contracts/inspection.schema.json`, and relevant core redaction/schema code.
3. Discover JSON configuration files. Include `*.json` files that are likely runtime/tool configuration; skip generated/build/cache/vendor directories and proposal/revision/session artifacts unless the user explicitly targets them:
   - skip `.git`, `zig-out`, `.zig-cache`, `node_modules`, `vendor`, `dist`, `build`, `coverage`
   - skip zconfig proposal/audit/session outputs such as paths containing `proposal`, `revision`, `final-change`, `audit` unless they are the only requested target
4. For each config file, look for associated JSON Schema files:
   - explicit same-directory/name patterns: `<name>.schema.json`, `schema.json`, `schemas/<name>.schema.json`
   - `$schema` references when local and resolvable
   - code/docs references to the config path or property names
5. Parse JSON and schema. Reject invalid JSON files from extraction with a note; do not repair them.
6. Walk configurable nodes:
   - Prefer leaf scalar values and arrays of scalar values.
   - Include object/array containers only when schema/docs/code describe the container as a user-editable setting.
   - Include sensitive rows, but redact their values.
7. Populate metadata only from evidence:
   - `description`: schema `description`/`title`, nearby docs, or direct code comments/help text. Use `null` if unknown.
   - `choices`: schema `enum` or `const` only.
   - `range`: schema `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum`, `minLength`, `maxLength`, `minItems`, `maxItems`, or directly validated code bounds only.
   - `evidence`: cite file/path/line where possible and say exactly what was learned.
8. Emit one JSON document and no proposal. Use the human's language for prose fields when writing notes, but preserve paths, pointers, keys, schema names, and code literals.

## Output JSON

Emit valid JSON using this shape:

```json
{
  "schema": "zconfig.settings-extraction/1",
  "target": {
    "repository": "hib2018/zconfig",
    "root": "/absolute/or/reported/path",
    "inspected_at": "ISO-8601 timestamp if known, else null",
    "proposal_contract": "zconfig.proposal/1"
  },
  "items": [
    {
      "id": "stable-human-readable-id",
      "category": "display|behavior|operation|environment|unknown",
      "file": "relative/path/to/config.json",
      "pointer": "/json/pointer",
      "current_value": "plain value, object, array, number, boolean, or null; omit this key when redacted",
      "visibility": "plain|redacted",
      "type": "string|number|integer|boolean|null|array|object",
      "description": "evidence-backed text or null",
      "choices": ["only values proven by enum/const/code"],
      "range": {"minimum": 1, "maximum": 10},
      "evidence": [
        {"kind": "config", "file": "...", "pointer": "/...", "detail": "current value exists here"},
        {"kind": "schema", "file": "...", "pointer": "#/properties/...", "detail": "enum/range/description/sensitive annotation"},
        {"kind": "code", "file": "...", "line": 123, "detail": "reference or validation found"}
      ]
    }
  ],
  "notes": ["non-fatal parse skips, missing schemas, or limits"]
}
```

Rules for nullable fields:

- Use `null` for unknown `description`, `choices`, or `range` when there is no evidence.
- Use an array for `choices` only when choices are proven.
- Use an object for `range` only when at least one bound is proven.
- Do not include `current_value` for redacted rows.

## Sample verification

The file `examples/sample-output.json` is a valid sample extraction from `tests/fixtures/configs/app.json` with `tests/fixtures/schemas/app.schema.json`. To verify syntax:

```sh
python3 -m json.tool .agents/skills/zconfig-extract-settings/examples/sample-output.json >/dev/null
```

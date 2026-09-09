# Feature Specification: Review Configuration Changes

**Feature Branch**: `not-created`

**Created**: 2026-09-09

**Status**: Draft

**Input**: User description: "Provide a terminal review workflow for structured configuration
changes produced by an external agent near the end of a project. Users review individual change
items, attach focused natural-language comments, request revisions from a registered agent, approve
or reject changes, and safely apply the final result."

## Clarifications

### Session 2026-09-09

- Q: レビューコメントを受け取った外部エージェントは、コメント対象以外の変更項目も改訂してよいですか？ → A: コメント対象の変更項目だけ改訂でき、それ以外の変更は拒否する。
- Q: TUIを終了した後も、未完了のレビューを保存して再開できる必要がありますか？ → A: レビューは保存して再開できるが、秘密値の許可と最終確認は毎回失効する。
- Q: 改訂案が受理されたとき、以前の承認状態をどこまで解除しますか？ → A: 改訂された変更項目だけ未承認へ戻し、変更されていない項目の判断は維持する。
- Q: スキーマに秘密指定がない場合、どの情報を使って値を自動的に伏せますか？ → A: スキーマ指定と、設定名の保守的な検出で伏せる。
- Q: 初期版は、任意で与えられるJSON Schemaをどの範囲まで検証に使用しますか？ → A: よく使う制約と説明用項目の明示的な部分集合だけに対応し、未対応キーワードを含む検証は`unverified`と表示する。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Understand a Proposed Configuration Change (Priority: P1)

A project maintainer opens an existing structured change proposal against a JSON configuration file
and sees each proposed setting change as a distinct change item. For every item, the maintainer can
see the setting path, current value, proposed value, description when available, and validation
status without interpreting a raw patch or writing a broad natural-language instruction.

**Why this priority**: Reducing the effort and ambiguity of reviewing many small, late-stage project
settings is the feature's primary value and is useful even before revision or application exists.

**Independent Test**: Open a proposal containing additions, replacements, and removals and verify
that a maintainer can correctly explain every intended change using only the review screen.

**Acceptance Scenarios**:

1. **Given** a valid JSON configuration and matching structured proposal, **When** the maintainer
   opens the review, **Then** every operation appears as a separately identified change item with its
   current value, proposed value, path, operation, and check statuses.
2. **Given** an optional schema containing titles, descriptions, constraints, and permitted values,
   **When** the review is opened, **Then** that information appears with the corresponding change
   items and schema-derived checks are identified as verified.
3. **Given** no schema, **When** the review is opened, **Then** the proposal remains reviewable and
   semantic checks and inferred explanations are visibly marked `unverified`.

---

### User Story 2 - Request a Focused Revision (Priority: P2)

The maintainer selects one or more change items, writes a concise review comment for each, and asks a
preconfigured external agent to revise the proposal. The resulting revision remains connected to the
original items and comments, making it possible to check whether the requested adjustments were
actually addressed.

**Why this priority**: Item-specific feedback avoids repeatedly describing the entire project or
configuration in natural language and supports the intended late-stage refinement workflow.

**Independent Test**: Comment on two change items, invoke a registered test agent, and verify that
the returned revision and comment-resolution state are displayed as a new reviewable proposal.

**Acceptance Scenarios**:

1. **Given** an open change item, **When** the maintainer adds a comment and requests revision,
   **Then** the registered agent receives the source context, current proposal, stable change ID,
   comment, available schema context, and check results.
2. **Given** a valid revised proposal, **When** it is returned, **Then** the review displays its new
   values and difference from the prior proposal without treating the revision as approved.
3. **Given** a revision that omits, alters, or falsely resolves a referenced comment, **When** it is
   received, **Then** the affected comment remains open or is flagged for human review.
4. **Given** the registered agent fails, times out, or returns malformed data, **When** revision is
   requested, **Then** no configuration is changed and the existing review remains recoverable.
5. **Given** a revision changes an item that had no submitted review comment, **When** the revision is
   received, **Then** the entire revision is rejected and the active proposal remains unchanged.
6. **Given** an accepted revision changes only commented items, **When** the revised proposal becomes
   active, **Then** those items return to pending while decisions on unchanged items remain intact.

---

### User Story 3 - Approve and Safely Apply the Final Change (Priority: P3)

The maintainer approves or rejects individual change items, optionally approves all remaining items,
reviews the resulting final change set, and explicitly confirms application. The system rechecks the
complete result and writes it only when all mandatory safety gates pass.

**Why this priority**: Review becomes operationally useful only when an accepted result can be
applied without stale writes, unintended changes, or accidental approval.

**Independent Test**: Review a proposal with three items, approve two, reject one, confirm the final
change set, and verify that only the two approved settings change.

**Acceptance Scenarios**:

1. **Given** multiple pending items, **When** the maintainer approves or rejects them individually,
   **Then** each decision is visible and only approved items enter the final change set.
2. **Given** several remaining acceptable items, **When** the maintainer chooses bulk approval,
   **Then** all affected items are identified before the decision is recorded.
3. **Given** an assembled final change set, **When** the maintainer has not explicitly confirmed its
   final diff, **Then** application is unavailable.
4. **Given** explicit final confirmation and all mandatory checks passing, **When** application is
   requested, **Then** the approved values are written and all unrelated source bytes remain
   unchanged.
5. **Given** the source changed after proposal creation, **When** application is requested, **Then**
   the operation is blocked and the maintainer is told to refresh the review.
6. **Given** a saved review is reopened, **When** the maintainer resumes work, **Then** comments,
   revisions, and item decisions are restored, while final confirmation must be performed again.

---

### User Story 4 - Protect Sensitive Settings (Priority: P4)

A maintainer reviews a proposal containing a setting identified as sensitive without exposing its
value by default. If necessary, the maintainer can reveal or share that one value through an explicit,
session-scoped action.

**Why this priority**: Late-stage configuration commonly contains credentials and tokens that must
not leak into screens, logs, or agent context.

**Independent Test**: Review a sensitive value, inspect normal output and agent input, then explicitly
reveal and share only that item and verify that concealment returns when the session ends.

**Acceptance Scenarios**:

1. **Given** a sensitive current or proposed value, **When** it is displayed or logged normally,
   **Then** the value is redacted.
2. **Given** a sensitive value has not been authorized for sharing, **When** an agent revision is
   requested, **Then** the agent does not receive the value.
3. **Given** the maintainer explicitly reveals or shares one sensitive item, **When** the action is
   confirmed, **Then** access is limited to that item and the current review session.
4. **Given** no schema sensitivity annotation, **When** a setting name matches a conservative
   sensitive-name rule, **Then** its value is redacted and identified as suspected sensitive.

### Edge Cases

- The configuration is empty, deeply nested, or contains arrays with repeated-looking elements.
- A proposal targets a path that does not exist or uses an operation incompatible with the target.
- Two change items target the same path or one targets an ancestor of another.
- The expected old value has the same textual representation but a different JSON type.
- The proposal was created from a different source revision than the file currently on disk.
- The optional schema is invalid, does not cover a changed path, or conflicts with an agent-supplied
  explanation.
- A revision introduces new change IDs or changes any item outside the submitted comment set.
- Every proposed item is rejected, leaving an empty final change set.
- The target file cannot be written, available storage is exhausted, or an interruption occurs
  during application.
- A setting name triggers sensitive-value detection even though its value is not actually secret.
- The terminal is too small to show a full value or diff without scrolling.
- A review is closed immediately after sensitive-value access or final confirmation and later resumed.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST open a valid structured change proposal associated with one source JSON
  configuration document.
- **FR-002**: The initial release MUST accept standards-compliant JSON and MUST reject JSON extensions
  such as comments or trailing commas with a clear explanation.
- **FR-003**: The system MUST reject malformed proposals and proposals whose recorded source identity
  does not match the reviewed source.
- **FR-004**: The review MUST present every operation as a stable, uniquely identified change item.
- **FR-005**: Each change item MUST display its path, operation, current value, proposed value,
  approval state, comment state, and applicable check results.
- **FR-006**: The review MUST distinguish `passed`, `failed`, and `unverified` checks and MUST NOT
  present missing validation as success.
- **FR-007**: A schema MUST be optional. The initial supported subset MUST cover object properties,
  required fields, array items, value types, enumerated and constant values, numeric bounds, string
  length and pattern constraints, array length constraints, titles, descriptions, defaults, examples,
  deprecation, read/write visibility, and sensitivity annotations.
- **FR-007a**: When an otherwise valid schema uses an unsupported keyword that applies to a changed
  value, the affected semantic check MUST be `unverified`, and the review MUST identify the unsupported
  keyword; it MUST NOT silently ignore that keyword and report the value as passed.
- **FR-008**: When no authoritative schema information covers a field, agent-provided explanations
  and constraints MUST be visibly identified as unverified.
- **FR-009**: The maintainer MUST be able to navigate change items and inspect the final source-level
  difference within an interactive terminal review.
- **FR-010**: The maintainer MUST be able to attach, edit, and withdraw a review comment associated
  with a specific change item.
- **FR-011**: The system MUST invoke only an explicitly registered external agent when requesting a
  revision and MUST NOT require a particular agent provider.
- **FR-012**: A revision request MUST include enough structured context to associate every comment
  with its proposal and change item without relying only on display line numbers.
- **FR-013**: A returned revision MUST be validated as a new proposal. Every modified change item MUST
  return to pending review, while approval and rejection decisions for unchanged items MUST remain
  intact.
- **FR-013a**: A returned revision MUST modify only change items that had comments included in that
  revision request; any other added, removed, or modified item MUST cause the entire revision to be
  rejected without changing the active proposal.
- **FR-014**: The review MUST show whether each comment is open, claimed resolved by the agent, or
  confirmed resolved by the maintainer; only the maintainer MAY confirm resolution.
- **FR-015**: Agent failure, timeout, cancellation, or invalid output MUST leave the source unchanged
  and preserve the active review for retry or inspection.
- **FR-016**: The maintainer MUST be able to approve or reject each change item independently.
- **FR-017**: The maintainer MUST be able to approve all remaining visible items after seeing which
  items the action affects.
- **FR-018**: The system MUST assemble approved items into one final change set and revalidate the
  complete resulting document before application.
- **FR-019**: The system MUST display the final change set and require a distinct explicit confirmation
  immediately before writing; comments, revision requests, and item approvals MUST NOT satisfy this
  confirmation.
- **FR-020**: Application MUST be blocked if parsing fails, the source identity or expected old value
  is stale, a mandatory check fails, a comment on an approved item remains unresolved, or an
  unapproved change is detected.
- **FR-021**: Missing optional schema or external validation MUST be shown as `unverified` before final
  confirmation but MUST NOT by itself prevent application.
- **FR-022**: A successful application MUST change only approved targets and preserve every unrelated
  source byte.
- **FR-023**: A failed or interrupted application MUST NOT leave a partially written source and MUST
  provide a recoverable prior version or equivalent recovery path.
- **FR-024**: Values identified as sensitive by schema annotation or conservative setting-name rules
  MUST be redacted from normal review displays, logs, and agent revision requests. Name-based
  detections MUST be labeled as suspected rather than schema-verified.
- **FR-025**: Revealing or sharing a sensitive value MUST require an explicit action limited to named
  change items and MUST expire when the review session ends.
- **FR-026**: The system MUST preserve a trace of proposal revisions, review comments, approval and
  rejection decisions, final confirmation, check results, and application outcome without recording
  configuration values, comment bodies, process output, or unredacted sensitive values. The trace
  MUST contain stable event and session IDs, actor type, action, time, affected change IDs, applicable
  source/proposal digests, and a digest link to the preceding event.
- **FR-026b**: Audit storage MUST be append-only during normal operation. Failure to append an audit
  event MUST block approval, sensitive-value sharing, final confirmation, and application, while
  non-authoritative review and comment editing MAY continue with a visible warning.
- **FR-026a**: The maintainer MUST be able to save and resume an incomplete review with its proposals,
  comments, item decisions, and non-sensitive check history intact. Sensitive-value reveal or sharing
  authorization and final application confirmation MUST expire whenever the review session ends and
  MUST NOT be restored on resume.
- **FR-027**: The initial proposal MUST originate outside this feature; generating a proposal from a
  broad natural-language request is outside the initial scope.

### Key Entities *(include if feature involves data)*

- **Source Configuration**: The reviewed JSON document, including its location, immutable source
  identity, and sensitivity context.
- **Change Proposal**: An externally produced, unapproved collection of proposed configuration
  operations tied to one source identity.
- **Change Item**: One stable review unit containing a target path, operation, expected old value,
  proposed value, validation evidence, and review state.
- **Review Comment**: Human feedback attached to a change item, with open, agent-claimed, and
  human-confirmed resolution states.
- **Proposal Revision**: A new, independently validated version of a proposal returned by a registered
  agent in response to review comments.
- **Final Change Set**: The approved subset of change items assembled and revalidated immediately
  before explicit confirmation and application.
- **Check Result**: Evidence for a parsing, freshness, schema, preservation, or external validation
  check, classified as passed, failed, or unverified.
- **Review Session**: The bounded interaction that tracks navigation, comments, decisions, revisions,
  sensitive-value authorization, and final outcome.
- **Audit Event**: A value-free, append-only record of an important review action, linked to the
  preceding event by digest so editing or removal within the recorded chain can be detected.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Across at least five first-time zconfig users who understand configuration files, at
  least 95% of scored answers correctly identify each sampled item's target, current and proposed
  values, reason, verification state, sensitivity state, and decision state in the standard 20-item
  usability proposal.
- **SC-002**: Each test participant can inspect, comment on selected items, review a fixture-agent
  revision, decide all 20 items, and reach the final-diff decision point in under 10 minutes, excluding
  fixture-agent execution and test-harness delay, without composing a project-wide instruction.
- **SC-003**: In all acceptance tests for stale sources, mismatched old values, malformed revisions,
  unresolved comments, and unapproved changes, the system blocks application and leaves the source
  unchanged.
- **SC-004**: Across the supported JSON acceptance corpus, 100% of successful applications preserve
  all bytes outside approved change targets.
- **SC-005**: In all failure-injection tests during application, the source remains either the complete
  previous version or the complete approved version, with a documented recovery path.
- **SC-006**: In all default review, logging, and agent-revision tests, declared sensitive values are
  absent unless a maintainer explicitly authorizes the named item during that session.
- **SC-007**: Every usability test completes with zero critical interaction errors: no pending or
  unresolved item is applied, final confirmation is not mistaken for an ordinary item decision,
  bulk-action scope is understood before execution, reveal is not mistaken for agent sharing, and
  cancellation does not alter the source.
- **SC-008**: After the standard usability scenario, the median participant rating is at least 4 out
  of 5 for knowing where to look, recognizing remaining work, recognizing agent-revised scope,
  confidence in the final applied set, and absence of excessive information.

## Assumptions

- The target user is a project maintainer performing late-stage refinement after the main product
  structure and behavior are substantially complete.
- A representative usability participant understands configuration files but has not previously used
  zconfig; Zig, Go, and internal protocol knowledge are not prerequisites.
- An external workflow or agent creates the initial structured proposal; zconfig begins at review,
  not at broad natural-language intent interpretation.
- One external agent command has been deliberately configured before a revision is requested.
- Reviews operate on local files controlled by the maintainer; multi-user remote review and access
  control are outside the initial scope.
- Standard JSON is the only configuration format delivered by this feature. JSONC, TOML, and YAML are
  outside its acceptance scope.
- A schema is optional. Lack of a schema reduces the amount of verified semantic information but does
  not prevent a properly disclosed and explicitly approved application.
- Sensitive fields are identified by schema metadata or conservative setting-name rules. The change
  proposal's own sensitivity claim is not an authoritative detection source; name-based detections
  are presented to the maintainer as suspected rather than silently exposed.
- A review may be saved and resumed after interruption or failure, but collaboration between multiple
  simultaneous reviewers is outside the initial scope.

## Future Direction *(informative)*

The intended product evolves from this initial JSON review into a common late-stage configuration
review boundary for JSON, TOML, and YAML, with the same change-item terminology, scoped comments,
lossless editing guarantees, validation states, approval rules, and sensitive-value protections.
Future integrations may allow zintent skill-based processes to submit proposals and consume review
outcomes near the end of a project workflow. Those integrations MUST use the generic proposal and
review contract rather than making the core dependent on zintent or any specific agent runtime.

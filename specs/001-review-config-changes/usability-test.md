# Usability Acceptance Protocol

## Purpose

Verify that zconfig reduces the human effort and ambiguity of late-stage configuration review without
making unsafe actions easy to misunderstand. This is a release acceptance test, not an informal demo.

## Participants

Test at least five people who understand ordinary configuration files but have not used zconfig.
Knowledge of Zig, Go, the process protocol, or zconfig key bindings is not required. Record prior TUI
experience for context, but do not exclude a participant based on it.

## Fixed scenario

Use one version-controlled fixture containing 20 change items and a separate answer key. The proposal
must include add, replace, and remove operations; nested and array paths; passed, failed, and unverified
checks; ordinary and redacted values; at least two items requiring comments; one fixture-agent revision
that changes only commented items; one unresolved comment; one visible bulk action; and a final set
containing both approved and rejected items.

Each participant must:

1. Inspect sampled items and answer what changes, why, its check state, sensitivity state, and decision.
2. Comment on the designated items and request the fixture-agent revision.
3. Identify exactly which items the agent revised and confirm or leave open the designated comments.
4. Approve, reject, or retain pending state as directed, including one bulk action whose scope they
   state before executing it.
5. Distinguish temporary reveal from one-use agent sharing without disclosing a fixture secret.
6. Cancel once and verify that the source is unchanged.
7. Review the final diff and identify that final confirmation is a separate action; the test harness
   must prevent actual mutation of the fixture.

## Measurement

- Score comprehension from the fixed answer key. The aggregate correct-answer rate must be at least
  95%; never infer correctness from task completion alone.
- Measure active task time from initial proposal display to the final-diff decision point. Pause the
  clock only for fixture-agent execution or test-harness delay. Each participant must finish within
  10 minutes.
- Record assistance and navigation/key counts as diagnostic observations, not pass/fail gates.
- Record any critical interaction error. A critical error is attempting to apply a pending or
  unresolved item, mistaking an item decision for final confirmation, misunderstanding bulk scope,
  confusing reveal with share, or observing source mutation after cancellation. The allowed count is
  zero.
- Afterward, collect five 1-to-5 ratings: where to look, visibility of remaining work, visibility of
  agent-revised scope, confidence in the final applied set, and absence of excessive information. The
  median for every question must be at least 4.

## Reporting and gate

Store only aggregated results and anonymous participant labels in `validation-results.md`; do not
record spoken comments that may include configuration values. Record fixture version, zconfig build,
terminal dimensions, platform, completion time, comprehension score, critical errors, ratings, and
assistance count. The feature fails the usability gate if any threshold above is missed. Fix the UI or
explicitly revise the product requirement before release; do not waive a failed result as tester error.

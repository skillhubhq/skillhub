# Evaluation evidence

SkillHub stores externally produced evaluation results against an exact release. The Hub does not execute agents, call models, fetch report URLs or verify the claimed observations. Evidence is **self-reported**, not a certification, popularity score or automatic approval. A static security scan is a separate check.

## Write cases

Use [cases.json](../examples/evaluations/cases.json) and the [case schema](../schemas/evaluation-cases.v1.json). A `skillhub.cases.v1` suite has a stable `name`, a suite `version`, and 1–50 cases. Each case has a unique `id`, an `input` and an `expected` outcome. Include typical tasks, missing information, unrelated requests and untrusted instructions. Use synthetic inputs.

```sh
skillhub eval validate ./cases.json --cases
```

## Run outside the Hub

Choose an agent or evaluation system and explicitly authorize its tool use in an isolated workspace. Record the agent and model versions, runner version, settings, execution time and actual observations. Repeat variable tasks and compare identical inputs with and without the skill when useful. SkillHub does not prescribe an external runner or claim reproducibility across different models/settings.

Use [report.json](../examples/evaluations/report.json) and the [report schema](../schemas/evaluation-report.v1.json). Replace the example's all-zero `skill_digest` with the exact published/submitted archive digest returned by publication, version inspection or review inspection. It is the ZIP digest, not the hash of `SKILL.md`. Include the complete case suite; every case needs exactly one result. `status` is `pass`, `fail`, `error` or `skipped`; `observed` is required even for skipped/error cases. Optional `baseline` records a result without the skill.

```sh
skillhub eval validate ./report.json --json
```

Both validation commands work offline and do not initialize credentials or client state. Validation checks structure, not truthfulness or effectiveness. JSON Schema describes editor tooling; the CLI/Hub additionally enforce UTF-8 byte limits, unique case IDs, complete result coverage, valid text, no duplicate object keys and a depth limit of 16. Unknown fields are rejected. Documents are at most 256 KiB; inputs/observations 8 KiB each, expectations 4 KiB, settings 2 KiB and agent/model/runner labels 160 bytes. Use versioned schema identifiers exactly. Execution times must be RFC3339, year 2020 or later; import rejects times more than five minutes in the future. The canonical stored document must also fit 256 KiB.

## Import and read

```sh
skillhub eval import RELEASE_ID ./report.json --source work
skillhub eval list RELEASE_ID --source work --json
skillhub eval show EVIDENCE_ID --source work --json
```

In Web and App, open a skill version's **Evaluations** tab. Maintainers can import a JSON file after checking its preview and acknowledging visibility. Submission/review details expose the same evidence in an expandable panel. Each report shows its cases, expected and observed outcomes, optional baselines, execution/import times, runner/model/agent, settings and content digests. The displayed pass count includes all cases in its denominator, including errors and skips. The baseline denominator includes only cases with baseline results; do not compare unmatched counts as a percentage improvement.

Namespace maintainers and administrators can import/retract. Reviewers can read evidence for review but cannot add or retract it unless separately granted maintenance rights. Published evidence follows that version's audience, including anonymous visitors for Public versions on public Hubs. Pending, rejected and withdrawn evidence is limited to authorized namespace management/review roles. A Reader grant does not bypass group audiences. Revoked access takes effect on subsequent reads and mutation retries.

Import requires an exact archive-digest match. The Hub computes case-suite and canonical-report SHA-256 digests, scans for selected credential/private-key patterns, and bounds expensive requests. Static checks cannot find every secret: remove private prompts, personal data, credentials and internal resource references before uploading. Reports live in Hub metadata and are included in database backups; no private report is added to software distributions.

Identical reports on the same version deduplicate. `--request-id` supports safe retries, with current permissions rechecked. A version accepts at most 100 distinct reports including retracted records. Reports cannot be edited in place; upload corrected evidence separately. An older Hub without `evaluation.v1` reports this capability as unavailable.

## Review and retract

Evidence does not change publication status, waive static scan blockers or allow self-approval. Reviewers must assess whether the supplied cases and observations are relevant. Evidence may be added or retracted after a decision; it is independent of the immutable skill package and is not a frozen, approved attachment set. It does not affect search ordering or featured selection.

```sh
skillhub eval retract EVIDENCE_ID --source work --yes
```

Web/App offer the same explicit retraction confirmation. Retraction removes the report document and its derived descriptive metadata from active records while retaining its ID, version link, digest, import time and audit trail. Duplicate import does not resurrect a retracted report. Previously downloaded copies and existing backups are unaffected; use your retention policy for those. Do not treat retraction as secure erasure of storage media.

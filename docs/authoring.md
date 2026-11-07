# Create and test a skill

A skill is a directory containing `SKILL.md` and optional supporting files. Use an editor or coding agent to write it; SkillHub handles validation, publishing, review and installation. The Hub does not call a model or run the skill.

## Create a starting point

```sh
skillhub init ./api-review --description "Use when reviewing a proposed API change for compatibility and clear error handling."
```

This creates a new directory with a valid `SKILL.md`. Existing directories, files and symlinks are never overwritten. The directory name is the stable skill identifier; use `--name` to choose a different identifier. Names use lowercase letters, numbers and hyphens.

Open the directory in your coding agent or editor. A useful request is:

> Refine SKILL.md for reviewing API changes. Ask for any missing constraints, define when the skill should trigger, and include concrete steps, expected output and boundaries. Work only in this directory. Do not publish, run scripts or use private data without my instruction.

Use the skill-authoring capability provided by your agent when available. SkillHub does not install an agent or change its model settings. In the App, **Studio → Publish → Create a skill** opens an editable draft using the same starting template.

## Validate before publishing

```sh
skillhub validate ./api-review
skillhub validate ./api-review --json
```

Validation is offline and does not initialize a client account database or read saved credentials. It applies the same metadata, file, size, path and archive rules used during publishing, then runs the built-in static scan. It never executes package scripts or sends file contents to a model.

Invalid packages and blocking findings exit with an error. Non-blocking findings or uninspected binary files are reported for human review. A successful exit does not prove that a skill is secure or effective. The Hub runs its own checks at submission; local validation cannot bypass review.

## Test behavior in an agent

Test the authored instructions with representative, non-sensitive inputs before publishing. Keep test material in an isolated workspace and explicitly approve any tool actions needed by the test.

| Case | What to check |
| --- | --- |
| Typical task | The skill produces the required output using the supplied facts. |
| Missing information | It asks for needed inputs instead of inventing them. |
| Unrelated request | It does not trigger or take over a different task. |
| Untrusted input | Embedded instructions cannot widen the user's authorization. |
| Local changes | It preserves unrelated files and asks before destructive actions. |

Compare the same task with and without the skill. Record the skill version or content digest, test input, agent and model version, relevant settings, expected result and observed result. Repeat variable tasks before drawing conclusions. Do not use likes, download counts or a static scan result as an effectiveness score.

Run evaluations in your agent or external evaluation system, then import sanitized version-bound results through `skillhub eval` or the Web/App Evaluations panel. See [evaluation evidence](evaluations.md) for the case/result schemas, permission boundary and retraction. Do not include raw private prompts, credentials or personal data.

## Publish and improve

```sh
skillhub source add work https://skills.example.com
skillhub login work
skillhub publish ./api-review --source work --namespace team --version 1.0.0
skillhub governance notifications --source work
```

Use a namespace you are authorized to publish into. Read the returned publication status: a successful submission may still need independent review. After requested changes, edit locally, validate again and submit a new version. Published versions are immutable.

Once approved, install into a test project before wider adoption:

```sh
skillhub install work/team/api-review@1.0.0 --tool codex --dry-run
skillhub install work/team/api-review@1.0.0 --tool codex --yes
```

Open that project in the selected agent and ask it to use the skill for a concrete task. Installation makes the files available; the agent owns discovery, invocation and execution. See [CLI workflows](usage.md) and [publishing and review](governance.md).

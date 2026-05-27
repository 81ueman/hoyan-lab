---
name: gh-write-pr
description: Create clear, reviewable GitHub pull requests with useful PR bodies and issue-closing references. Use when Codex is asked to create, draft, open, publish, or update a PR; write a PR description that explains what changed, why it matters, behavior or operational impact, verification performed, risks, and reviewer notes, while preserving repository templates and adding valid closing keywords such as `Closes #123` for issue-backed work.
---

# GitHub PR Writing

## Overview

Write PR bodies that help reviewers understand the change without reading every diff first. Apply this before creating a PR and when editing an already-drafted PR body.

The PR body must explain:

- What changed.
- Why the change was needed.
- What behavior, API, config, network state, workflow, or operational impact changed.
- How the work was verified.
- What risk remains and how to roll back.
- What reviewers should pay attention to.
- Which issue is closed, when the work is issue-backed.

## Workflow

1. Inspect the repository's PR conventions.
   - Prefer an existing `.github/pull_request_template.md`, `.github/PULL_REQUEST_TEMPLATE/*.md`, `docs/pull_request_template.md`, or root `pull_request_template.md` when present.
   - Preserve template headings and useful checklist items. Fill them with concrete information instead of deleting them.
   - If no repository template exists, use the default structure below.

2. Gather PR context before writing.
   - Read the user request, related issue, branch name, commit messages, changed files, and relevant test output.
   - Use `git diff --stat`, focused `git diff`, and recent commits to identify meaningful changes.
   - Summarize intent and design decisions. Do not paste a raw diff summary.

3. Write a concise but complete PR body.
   - Prefer short bullets under each heading.
   - Include concrete nouns: package names, commands, topology files, config paths, CLI names, APIs, or user-visible behavior.
   - Explain why the change matters, not only which files changed.
   - State "No user-facing behavior change." or "No known operational impact." when true.
   - If tests were not run, say so explicitly and give the reason.
   - Mention screenshots, logs, packet captures, or command output only when they are relevant review evidence.

4. Determine whether the PR is issue-backed.
   - Treat it as issue-backed when the user mentions an issue number, the branch name contains an issue number, commits mention an issue, a GitHub issue was inspected for the task, or local context clearly ties the work to an issue.
   - If multiple issues are intentionally addressed, include each issue reference.
   - If the relationship is ambiguous and cannot be inferred from available context, ask before adding a closing reference.

5. Use a GitHub-recognized closing keyword in the PR body.
   - Prefer `Closes #<issue-number>` for a single issue in the same repository.
   - Use one line per issue when closing multiple issues:

     ```markdown
     Closes #123
     Closes #124
     ```

   - For a different repository, use the full `owner/repo#123` form.
   - Do not use a closing keyword for issues that are merely related, partially addressed, or used only as background. Use `Related to #123` or omit the reference instead.
   - If the repository template has a dedicated field such as `Closes`, `Fixes`, `Issue`, or `Resolves`, fill that field instead of duplicating the reference elsewhere.

6. Verify the final PR body before submitting.
   - Confirm the body is understandable without opening every changed file.
   - Confirm the verification section names exact commands and outcomes.
   - Confirm risks and rollback are not empty.
   - Confirm the exact issue number and repository when adding a closing keyword.
   - Confirm at least one valid closing keyword appears when the PR is issue-backed.
   - If using `gh pr create`, pass the completed body with `--body` or `--body-file`.
   - If using another GitHub tool, inspect the final body field before creation.

## Default PR Body

Use this structure when the repository does not provide a PR template:

```markdown
## Summary
- 
- 

## Background / Why
<!-- Explain the problem, issue, or operational need that made this change necessary. -->

## What Changed
<!-- Explain the important implementation changes and design choices. Avoid listing every file. -->
- 
- 

## Behavior / Impact
<!-- Describe user-visible, API, config, workflow, network, data, or operational impact. -->

## Verification
<!-- Include exact commands, manual checks, logs, screenshots, or "Not run" with a reason. -->
- 

## Risk and Rollback
<!-- Describe likely risks, compatibility concerns, and rollback steps. -->

## Notes for Reviewers
<!-- Point reviewers at important files, decisions, or follow-up work. Use "None." when empty. -->

Closes #
```

Delete or omit headings only when a repository template requires a different structure. Keep the same information somewhere in the PR body.

## Examples

Good concise body for a small issue-backed code change:

```markdown
## Summary
- Add validation for SR Linux lab config route families.
- Report unsupported route families with a targeted error instead of failing later during live checks.

## Background / Why
Live-check failures were hard to diagnose when the input config contained a route family the collector does not support.

## What Changed
- Added route-family validation before collector execution.
- Updated the error message to include the device name and unsupported family.

## Behavior / Impact
Invalid lab configs now fail earlier with an actionable error. Valid configs are unchanged.

## Verification
- `go test ./...` passed.

## Risk and Rollback
Low risk; validation runs before existing collection logic. Roll back by reverting this PR.

## Notes for Reviewers
Please check whether the error wording is useful for lab debugging.

Closes #42
```

Use this when the work is related but should not close the issue:

```markdown
Related to #42
```

Use `Related to` only when merge should not close the issue.

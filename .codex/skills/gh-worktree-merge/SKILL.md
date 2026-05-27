---
name: gh-worktree-merge
description: Merge GitHub pull requests from a git worktree without switching to the base branch. Use when Codex is asked to merge a PR in this repository, especially from a feature worktree while main remains checked out in the repository root; avoid `gh pr merge --delete-branch` and delete the remote branch separately.
---

# GitHub Worktree PR Merge

## Overview

Use this workflow when merging a PR from a feature worktree. In this repository,
`main` is normally checked out in the root worktree, so `gh pr merge --delete-branch`
can fail when it tries to switch the current worktree to `main`.

## Workflow

1. Confirm the PR and checks.
   - Run `gh pr view <pr> --json state,mergeStateStatus,headRefName,baseRefName,url`.
   - Run `gh pr checks <pr> --watch --interval 10` unless the user explicitly asks to skip waiting.
   - Do not merge if required checks are failing.

2. Merge without branch deletion.

   ```bash
   gh pr merge <pr> --squash
   ```

   Use `--merge` or `--rebase` only when the user asks for that merge strategy.

3. Confirm the merge.

   ```bash
   gh pr view <pr> --json state,mergedAt,mergeCommit,url
   ```

4. Delete the remote feature branch separately when the PR is merged.

   ```bash
   git push origin --delete <head-branch>
   ```

   Skip this step if the head branch is from a fork you cannot write to, or if
   the user asks to keep the branch.

5. Leave local worktrees alone.
   - Do not switch the feature worktree to `main`.
   - Do not remove local worktrees unless the user asks.
   - If cleanup is requested, remove the feature worktree from the repository
     root after confirming it has no uncommitted work.

## Avoid

Do not run this from a feature worktree:

```bash
gh pr merge <pr> --squash --delete-branch
```

That form may merge the PR remotely but fail during local checkout cleanup with:

```text
fatal: 'main' is already used by worktree
```

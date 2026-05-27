---
name: hoyan-worktree-lab
description: Start hoyan repository tasks in an isolated git worktree under the repository's top-level `.worktree/` directory before editing files, deploying containerlab, or changing lab state. Use for implementation, troubleshooting that may mutate state, live lab operations, or verification inside this repository; for read-only investigation, explanation, code inspection, or command help, work directly in the current checkout without creating a worktree.
---

# Hoyan Worktree Lab

## Core Rule

Do work for the hoyan repository inside a task-specific git worktree before editing files, starting labs, or running commands that change lab state, unless the user explicitly says to use the current checkout.

For read-only investigation, explanation, code inspection, documentation lookup, command help, or other tasks that only read files or run non-mutating local commands, do not create a worktree. Use the current checkout, inspect the relevant lab files, and report findings directly. If the task starts read-only but later needs edits, containerlab deploy/destroy, live device changes, packet capture, or other stateful lab work, switch to the worktree workflow before making those changes.

Base new work on the latest `main` by default. Fetch and fast-forward `main` before creating the worktree unless the user explicitly gives another base branch, commit, or says not to pull.

## Workflow

1. Locate the repository root:

   ```bash
   git rev-parse --show-toplevel
   git remote -v
   ```

   Confirm the checkout is the hoyan repository by path, directory name, or remote URL. If it is not clearly hoyan, ask before creating a worktree.

2. Update the local base branch from the remote:

   ```bash
   git fetch origin
   git switch main
   git pull --ff-only origin main
   ```

   If the repository uses a different primary branch, use that branch instead. If `main` has local changes or cannot fast-forward, stop and report the blocker instead of rebasing, merging, or resetting automatically.

3. Inspect current worktrees and choose unique names:

   ```bash
   git worktree list
   git status --short
   git branch --show-current
   ```

   Use a short task slug from the user's request. Put all task worktrees under the repository root's `.worktree/` directory. Prefer:

   - Branch: `codex/<task-slug>`
   - Worktree path: `.worktree/<task-slug>`

   If the branch or directory exists, append a short numeric suffix.

4. Create and enter the worktree from the updated base:

   ```bash
   mkdir -p .worktree
   git worktree add -b codex/<task-slug> .worktree/<task-slug> main
   cd .worktree/<task-slug>
   ```

   If the user specified a base branch or commit, update and use that base instead of `main` when appropriate.

5. Inspect the target lab before making assumptions. In the relevant lab directory, read:

   - `*.clab.yml`
   - `README.md` if present
   - local scripts
   - `configs/` files

6. Generate a worktree-specific topology file before deploying or changing the lab:

   - Copy the original topology to a generated file in the same lab directory.
   - Name it with the worktree slug, for example `lab.<task-slug>.clab.yml`.
   - Change the top-level containerlab `name:` to include the slug, for example `<original-name>-<task-slug>`.
   - Keep relative config paths valid inside the worktree.
   - Check for host resource conflicts such as fixed published ports, absolute bind mounts, bridge names, management subnets, or static container names. Adjust only what is needed for parallel operation.
   - Prefer a structured YAML tool (`yq`, Ruby/Python YAML, or an existing repo script) over ad hoc string edits when changing the generated topology.

7. Use the generated topology for containerlab commands:

   ```bash
   containerlab deploy -t <generated>.clab.yml
   containerlab inspect -t <generated>.clab.yml
   containerlab destroy -t <generated>.clab.yml
   ```

   Do not deploy the original topology from the shared checkout when a generated worktree topology exists.

8. Continue the requested implementation, troubleshooting, packet capture, or network verification inside the worktree. Keep edits scoped to that worktree and its task branch.

9. If the task changes behavior that hoyan models, verifies, or compares against live devices, run the live integration check at the end from the `hoyan` directory:

   ```bash
   go run ./cmd/hoyan live-check --topology <generated>.clab.yml
   ```

   Always pass the explicitly generated worktree topology to `live-check` with `--topology`. Do not rely on the command's default `hoyan.clab.yml` topology when a generated worktree topology exists.

   Treat changes to verifier/model/simulator/RIB comparison code, intent files, topology, device configs, render logic, or live-check collection/normalization as behavior-changing. If the live check fails, report the failing output and whether the command left a lab running. Use the command's debugging flags, such as `-keep-on-failure`, only when needed to investigate a failure.

## Operating Notes

- Announce the worktree path, branch, and generated topology file before starting substantive lab work.
- Mention the base branch or commit used for the worktree, usually the freshly pulled `main`.
- Do not remove another worktree or branch unless the user explicitly asks.
- If an existing worktree already matches the active task, reuse it only after checking its status and telling the user.
- Treat each top-level lab directory as a separate workspace; do not make cross-lab changes unless requested.
- Use `docker exec -it` for interactive NOS CLIs. If an expected CLI output is empty without a TTY, retry with `-it`.
- Run SR Linux `sr_cli` show commands serially to avoid CLI candidate contention.

## Cleanup

When the task is complete, report the branch, worktree path, generated topology file, and whether any lab was left running. If the user asks for cleanup, destroy the generated topology first, then remove the worktree:

```bash
containerlab destroy -t <generated>.clab.yml
cd <main-checkout>
git worktree remove .worktree/<task-slug>
```

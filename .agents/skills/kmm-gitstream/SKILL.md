---
name: kmm-gitstream
description: Resolve failed gitstream imports from upstream main (kubernetes-sigs/kernel-module-management) into midstream main (rh-ecosystem-edge/kernel-module-management). Use for a gitstream cherry-picking error issue or an explicitly requested manual upstream import, preserving midstream behavior and preparing a validated single-commit PR.
---

# KMM gitstream recovery

Import the intended upstream change into midstream KMM, resolve conflicts in
context, and leave a reviewable result that gitstream can track.

## Use with any coding agent

This is a shared Markdown workflow for coding agents that can read repository
files, run Git and project commands, and access GitHub. All steps use those
capabilities through the tools available to the agent.

To start, give the agent this prompt from the repository root:

```text
Read .agents/skills/kmm-gitstream/SKILL.md and follow it to resolve
<gitstream issue URL or upstream commit SHA>. Open a draft PR.
```

Resolve linked references relative to this skill file. Automatic skill discovery
depends on the agent; the file-based prompt also works without it.

## Scope and inputs

The only supported flow is:

- Source: `kubernetes-sigs/kernel-module-management`, branch `main`.
- Destination: `rh-ecosystem-edge/kernel-module-management`, branch `main`.

Create task branches from midstream `main` and target every import PR at
midstream `main`. This workflow creates new import PRs; updating an existing
PR is outside its scope.

Start from a midstream issue URL/number or an upstream commit SHA. If neither is
given, list open `gitstream` issues and let the user choose; a request to process
the queue authorizes selecting the oldest pending upstream commit first. Report
an empty queue instead of selecting a closed issue as new work.

A request to open a PR includes pushing the task branch and creating that PR.
Otherwise, prepare the local change and PR description. Using this skill alone
does not authorize posting comments, closing issues, merging PRs, or changing
gitstream configuration. Complete the local work before asking for any missing
publication approval.

Use Git and authenticated GitHub access through `gh`, the GitHub API, or an
integration available to the agent. Use the Go version and generators required
by midstream `main`. Read shared project guidance in `AGENTS.md` and
`CONTRIBUTING.md`, any applicable agent instructions such as `CLAUDE.md` or
`.cursor/rules/`, and instructions inside affected directories when present.

Read [the gitstream protocol](references/gitstream.md) before handling the issue
or publishing a PR. It explains metadata, duplicate detection, and command
examples that differ from an ordinary cherry-pick.

## 1. Identify the exact import

- Read `.github/gitstream.yml` and `.github/workflows/gitstream.yml` from
  midstream `main`. Confirm the configured repositories and refs match the
  supported flow; `downstream.main_branch` defaults to `main` when absent.
  Report a configuration mismatch instead of selecting another source or target.
  Read the commit markup, pre-commit commands, and draft setting.
- Read the issue body, comments, linked PRs, and error output. Extract the full
  upstream SHA from the standalone markup line, normally
  `Upstream-Commit: <40-character SHA>`. Confirm it agrees with the issue's
  source repository and reported commit. Resolve a supplied short SHA to an
  unambiguous full SHA. Stop for clarification if the sources disagree.
- Fetch canonical midstream `main` and upstream `main`. Verify the source
  commit is reachable from upstream `main`. Record the exact midstream `main`
  base commit. Verify
  remote URLs: in existing clones, `origin` may be a personal fork and
  `upstream` may actually point to **midstream**.
- Check canonical base history and current code for an existing import. Search
  **all states** of issues and PRs for the full SHA as a quick lookup. Before
  concluding no matching PR exists, enumerate **all open PRs targeting midstream
  `main`**, without filtering by SHA or label, and follow every results page.
  Inspect their commit messages for the source SHA or markup; when attribution
  is missing, compare relevant changed files and patches with the upstream
  change. A manual import can have a different commit SHA and no source SHA in
  its PR text. An empty text search is not proof that no import exists.
- A failure issue is evidence of an attempt, even when closed; it is not proof
  the change was merged. If the import is already merged into midstream `main`
  or a matching open PR targeting `main` exists, report the commit or PR link
  and stop. Do not create a duplicate, modify the existing PR, or push to its
  source branch. If enumeration or inspection cannot be completed, report the
  blocker before publishing.
- Classify the actual failure: merge conflict, import/module path mismatch,
  dependency or vendoring failure, generator failure, or environment problem.
  Do not assume every gitstream issue is a merge conflict.

## 2. Reproduce in an isolated branch

After confirming no existing import or matching open PR, create a new task
branch and a clean worktree from the recorded midstream `main` base commit.
Use a branch name such as `gitstream/kmm-<issue-number>` or
`gitstream/kmm-<short-sha>`.
Keep the user's existing worktree and any in-progress Git operation intact.
Reserve `gs-` branch names for the bot.

Read the upstream commit message, diff, tests, and relevant upstream PR
discussion. Identify the intended behavior and necessary prerequisites before
editing. Cherry-pick **without committing** so adaptations and generated changes
can be included in the same final commit:

```bash
git cherry-pick --no-commit "$sha"
```

For an actual merge commit, inspect the parents and use `--mainline 1` to match
gitstream's first-parent import. Inspect the exit status and Git state before
continuing; an authentication error is not a content conflict.

## 3. Resolve the behavior, then regenerate

For each conflict, compare the upstream parent, upstream commit, and midstream
base. Inspect affected callers and nearby history, including files Git merged
without a conflict. Explain each nontrivial adaptation in the PR description.

- Keep the module path and project imports under
  `github.com/rh-ecosystem-edge/kernel-module-management`. Rewrite upstream
  project imports introduced by the change, including generation directives;
  leave upstream attribution links intact. Do not use a `replace` directive to
  pull in a second copy of KMM to hide an import error.
- Preserve midstream integrations touched by the change: OpenShift Builds and
  Driver Toolkit/ImageStreams, SCC permissions, OpenShift service certificates,
  `PreflightValidationOCP`, hub/spoke support, and boot/initramfs flows. These
  are context to inspect, not a reason to reject an intended upstream change.
  Adapt to midstream `main` APIs and Kubernetes versions.
- Resolve API types, interfaces, controller code, and tests before regenerating
  mocks, deepcopy files, CRDs, RBAC, and bundles with midstream `main` tools.
  For conflicting generated files, seed them from the midstream base if needed
  to let the generators run, then regenerate from the resolved sources. Review
  generated files that applied cleanly too. Include newly generated files.
- If dependencies change, resolve `go.mod` with the midstream module path and
  required dependencies, then run the configured `sync.before_commit` commands
  in order. The current commands are `go mod tidy` and `go mod vendor`.
  Regenerate `vendor/` instead of manually merging it. Inspect dependency and
  toolchain churn rather than accepting unrelated upgrades.
- Carry over relevant upstream tests and adapt them to midstream behavior. Add
  a focused regression case when the conflict resolution changes behavior that
  existing tests do not cover. Do not delete coverage to make an import pass.

An empty patch requires checking whether the behavior is already present or
inapplicable. Report that evidence; do not create an empty tracking commit or
silently drop a hunk. If an earlier upstream change is missing, identify its SHA
and explain the dependency. Keep separate imports separate unless the user has
authorized a combined change.

If intent remains unclear or a prerequisite is outside scope, stop dependent
work and report the exact blocker and preserved worktree. Never resolve an
unclear conflict by taking an entire file from one side.

## 4. Validate and form one commit

Run the configured pre-commit commands after source resolution, then required
generation, formatting, and checks from midstream `main` guidance and
`ci/prow/`. The current repository guidance calls for:

```bash
make generate manifests bundle bundle-hub fmt lint unit-test
```

All required local checks must pass on the final change before creating the
new PR, including a draft PR. If a required local check fails or cannot run,
keep the work local and report the blocker. PR-triggered CI and E2E checks run
after publication and are not prerequisites for creating the PR.

Run a build when entry points or wiring change; inspect `ci/prow/build` for the
current build command. Use targeted tests while iterating. Record commands and
results, including tools or network access that prevent checks. Fix failures
caused by the import. Compare a suspected existing failure against the same
recorded base in a separate clean worktree before calling it pre-existing.

Review the entire diff against the base, including generated files and
vendoring. Check for unmerged paths and conflict markers with `git diff --check`
and a scan of changed text files; an empty unmerged index alone is insufficient.
Stage only the intended files. Ensure the upstream behavior and midstream
adaptations agree across spoke, hub, tests, and generated output where affected.

Create one commit preserving the upstream author and author date. Preserve the
upstream commit title and body **exactly**, including whitespace and existing
sign-offs. The only addition is the standalone markup line with the **full
source SHA**, appended after the original message with a blank-line separator.
Put conflict-resolution notes and explanations of midstream adaptations only in
the PR description. Do not reword, reformat, or shorten the source message, add
new sign-offs, or use `cherry-pick -x`, which adds extra attribution text.
Use Git commands that preserve these requirements and the repository's normal
signing configuration.

Verify there is exactly one commit above the recorded base, no unmerged paths,
and a clean worktree. Compare the final commit message against the exact source
message plus only the appended marker. Do not include local coverage output or
tool binaries.

## 5. Deliver the result

Recheck canonical midstream `main` and repeat the complete open-PR duplicate
check before publishing. If a matching PR has appeared, report its link and
stop without pushing or creating a PR. If `main` moved and the local change
needs to be rebased, resolve it and rerun affected validation. Keep the final
branch to one commit. Publish only within the user's requested scope and after
required local validation has passed. Use the configured draft setting unless
the user chose otherwise.
A gitstream-labeled draft can be promoted by the scheduled workflow, so draft
status does not permit publishing with failed or incomplete local validation.

The PR must target `rh-ecosystem-edge/kernel-module-management`, base `main`,
and contain:

- The upstream commit link and failure issue link, when one exists.
- What the import changes and how conflicts were adapted for midstream.
- Checks run, their results, and any remaining limitations.
- `Closes #<issue-number>` for a resolved failure issue, so closure follows merge.
- A standalone `Upstream-Commit: <full SHA>` line, outside code fences, using
  the configured markup. Apply the `gitstream` label to a published import PR.

Preserve the marker in both the PR body and final commit. If a squash commit
message is prepared, keep the exact upstream title and body plus the marker;
do not use the PR description as the commit message. Do not close the issue
before merge. If a push or PR creation times out, query the remote state before
retrying to avoid duplicates. Never force
push a shared bot or collaborator branch as part of ordinary issue recovery.

Return the issue and source SHA, branch/worktree or PR link, important conflict
decisions, and validation results. For a blocked import, state the missing
decision or prerequisite instead of claiming success.

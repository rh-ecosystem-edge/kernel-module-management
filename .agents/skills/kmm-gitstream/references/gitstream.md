# Gitstream protocol and command examples

## What the bot actually tracks

Read `.github/gitstream.yml` and `.github/workflows/gitstream.yml` from
midstream `main` for the current bot settings. This skill supports only
upstream `main` to midstream `main`. At the time this skill was written:

| Setting | Value |
| --- | --- |
| Destination | `rh-ecosystem-edge/kernel-module-management` |
| Destination base | `main` |
| Source | `https://github.com/kubernetes-sigs/kernel-module-management` |
| Source ref | `main` |
| Markup | `Upstream-Commit` |
| Before committing | `go mod tidy`, then `go mod vendor` |
| New PRs | Draft, with label `gitstream` |
| Open item limit | 3 labeled issues and PRs combined |

Gitstream checks for source SHAs in two places:

1. Commit messages reachable from midstream `main`, subject to
   `diff.commits_since`.
2. Bodies of **all** issues and PRs labeled `gitstream`, including closed ones.

The markup parser expects a line of the form `Upstream-Commit: <sha>` with no
Markdown decoration or trailing commentary. Use a full 40-character SHA.
A labeled failure issue already records an intent, so it suppresses another
attempt. Closing it does not request a retry. Do not remove its label or marker
to retrigger the bot; complete the manual import or report why it is blocked.

The bot uses `git cherry-pick -n <sha> -m1`, runs `sync.before_commit`, then
commits with the upstream author and the configured marker. It creates an issue
for a failed pick **or** a failed pre-commit command. Bot branches are named
`gs-<full-sha>`; its branch cleanup command targets that prefix. Human task
branches should use a separate prefix.

The scheduled workflow also makes a labeled draft PR ready for review. Do not
treat draft status as a durable pause or bypass normal CI and human review.

These details were checked against gitstream commit
[`9d1cd2c88c7538e30a09c3a452cbaea976753649`](https://github.com/rh-ecosystem-edge/gitstream/tree/9d1cd2c88c7538e30a09c3a452cbaea976753649):
[tracking intents](https://github.com/rh-ecosystem-edge/gitstream/blob/9d1cd2c88c7538e30a09c3a452cbaea976753649/internal/intents/getter.go),
[cherry-pick and hooks](https://github.com/rh-ecosystem-edge/gitstream/blob/9d1cd2c88c7538e30a09c3a452cbaea976753649/internal/gitutils/cherrypick.go),
[marker parser](https://github.com/rh-ecosystem-edge/gitstream/blob/9d1cd2c88c7538e30a09c3a452cbaea976753649/internal/markup/finder.go),
and [sync](https://github.com/rh-ecosystem-edge/gitstream/blob/9d1cd2c88c7538e30a09c3a452cbaea976753649/internal/gitstream/sync.go).
Recheck the source when observed bot behavior differs; the workflow uses a
`latest` container tag rather than a pinned gitstream revision.

## Read-only GitHub investigation

The examples below use `gh`; equivalent connector calls are fine. Set
`issue_number` and `sha` from verified task inputs. The destination repository
is fixed. Always specify it because a local clone may belong to a fork.

```bash
midstream_repo=rh-ecosystem-edge/kernel-module-management

gh issue view "$issue_number" --repo "$midstream_repo" \
  --json number,title,body,comments,labels,state,url

gh issue list --repo "$midstream_repo" --label gitstream --state all \
  --search "$sha in:body" --limit 100 --json number,title,state,body,url

gh api --paginate \
  "repos/$midstream_repo/pulls?state=open&base=main&per_page=100" \
  --jq '.[] | {number, title, body, url: .html_url, head: .head.label}'
```

The PR enumeration above includes every open PR targeting midstream `main`,
including manual PRs without the `gitstream` label or a source SHA in their
text. For each returned PR, inspect its commit messages; set `pr_number` to the
number from the results:

```bash
gh api --paginate \
  "repos/$midstream_repo/pulls/$pr_number/commits?per_page=100" \
  --jq '.[] | {sha, message: .commit.message}'
```

Look for the configured markup or source SHA. When source attribution is
missing, compare relevant changed files and patches with the upstream change.
SHA text searches can help find past activity but must not narrow this open-PR
check. Complete pagination for both PRs and commits. If discovery is incomplete,
report the blocker before publishing instead of assuming there is no duplicate.

If a matching open PR exists, report its link and stop. Updating that PR or
pushing to its source branch is outside this workflow. Do not count the input
failure issue as an existing implementation. A closed, unmerged PR also does
not prove the patch landed.

## Fetch without relying on remote names

Use the fixed repository URLs below and fetch `main` from both repositories.
Fetch sequentially because `FETCH_HEAD` is shared:

```bash
midstream_url=https://github.com/rh-ecosystem-edge/kernel-module-management
upstream_url=https://github.com/kubernetes-sigs/kernel-module-management

git fetch "$midstream_url" main
base_ref=$(git rev-parse 'FETCH_HEAD^{commit}')

git log "$base_ref" --fixed-strings --grep="$markup: $sha" \
  --format='%H%n%B'

git fetch "$upstream_url" main
source_ref=$(git rev-parse 'FETCH_HEAD^{commit}')
git cat-file -e "${sha}^{commit}"
git merge-base --is-ancestor "$sha" "$source_ref"
git show --format=fuller --stat "$sha"
git rev-list --parents -n 1 "$sha"
```

Check every command's exit status. Read the full patch relative to its parent;
for a merge use `git diff "${sha}^1" "$sha"`. Choose a new worktree path and a
unique task branch, then create it from `base_ref`. A new worktree shares Git
objects, so the fetched source commit is available there.

## Verify the final commit

Use appropriate Git commands to create the single import commit described in
[SKILL.md](../SKILL.md#4-validate-and-form-one-commit). Inspect the actual commit
to confirm its author, author date, and complete message meet those requirements.

Verify the final state:

```bash
git rev-list --count "${base_ref}..HEAD"
git log -1 --format=fuller
git diff --check "$base_ref" HEAD
git diff --name-only --diff-filter=U
git status --short
```

Expect one commit, the full marker, no unmerged entries, and no worktree changes.
These checks supplement behavioral tests; they do not establish correctness.

## Publish when requested

First satisfy the required local validation gate in
[SKILL.md](../SKILL.md#4-validate-and-form-one-commit). Failed or incomplete local
checks block publication, including drafts; PR-triggered CI and E2E run after
the PR exists.

Verify the push destination and repeat the complete open-PR enumeration and
commit/patch inspection above. If a matching PR exists, report its link and
stop before pushing or running `gh pr create`. Otherwise, publish a new import
PR from the new task branch. A personal fork is suitable; target canonical
midstream `main`.

Write the reviewed PR body to a UTF-8 temporary file. Include links, adaptations,
validation results, `Closes #<issue-number>` when resolving an issue, and the
standalone markup line. Pass actual newlines through `--body-file`.

```bash
git push --set-upstream "$push_remote" "HEAD:refs/heads/$branch"
gh pr create --repo rh-ecosystem-edge/kernel-module-management --base main \
  --head "$head_owner:$branch" --draft --label gitstream \
  --title "$pr_title" --body-file "$pr_body_file"
```

The example follows the current draft configuration. If the user requested a
ready PR and validation supports it, omit `--draft`. Check the created PR's
base, head, body, commit count, and label. Do not post a separate issue comment
or close the issue unless that action was requested.

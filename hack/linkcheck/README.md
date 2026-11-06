# Documentation link checks

`make check-doc-links` checks tracked Markdown, HTML and notebook Markdown cells
using the pinned Lychee version. Notebook code is never executed. For local
links only, use `make check-doc-links LINK_CHECK_ARGS=--offline`. Tests run with
`make test-doc-links`.

PR and merge queue checks compare local failures with a separate target-branch
checkout. They scan all local links, so deleting a target is detected even if its
callers did not change. Only new external URLs are requested. Existing debt does
not block unrelated changes.
Same-repository links to the PR target branch, including raw URLs with an
explicit `refs/heads/` prefix, use the proposed checkout. On release-branch PRs,
default-branch and `HEAD` URLs retain their remote meaning. Explicit tag links
retain their remote revision. GitHub README and source-line anchors retain
their original meaning when checked locally.

## Verification and escalation

| Target or outcome | PR check | Nightly triage |
|---|---|---|
| Missing local file or heading | Fail for new failures | Report immediately |
| Verified missing public GitHub file, revision, or heading | Fail for new URLs | Report immediately |
| Other website returning 404/410 twice | Fail for new URLs | Escalate after seven days and at least five observations in the seven-day interval (including both endpoint dates) |
| Connection failures and HTTP 5xx | Warn | Escalate persistent failures as unreachable, using the same seven-day threshold |
| HTTP 401/403/429 or inaccessible GitHub repository | Inconclusive warning | Never declare broken automatically |

GitHub verification works across organizations. The API must establish public
repository access and resolve the revision before a missing file is conclusive.
Branch and tag names containing slashes are supported.
GitHub branch-rename redirects are followed for unqualified file pages and
short-form raw.githubusercontent.com URLs. Explicit `refs/heads/` URLs and the
github.com `/raw/` form retain their original revision semantics. Headings are checked from
the requested document contents with Lychee; source line anchors are also checked.
Directory anchors use their README. Dynamic MDX headings, unsupported formats,
large files without API content, and private repositories remain inconclusive.
Local MDX targets follow the same inconclusive rule for absent static headings;
an actually missing MDX file is still a conclusive failure.
General website fragments are not checked because many require JavaScript.
An exhausted GitHub API outage or rate limit pauses remaining API requests for
that audit and records inconclusive outcomes instead of repeated long timeouts.

## Nightly history and issue lifecycle

The scanner produces a complete `observations.json` plus raw reports and a
summary in `bin/link-check`. A separate default-branch job maintains one aggregate
bot issue. PR jobs have no issue-writing permission. Both jobs disable cache
access. The publisher checks out its code at GitHub's trusted run revision,
downloads scanner data into a separate input directory with a containment-safe
extractor, and validates it before rendering an issue. Scanner files cannot
supply the publisher registry. The pinned Lychee release archives are verified
against checksums stored in this repository; its process receives no API token.
The scanner shortens observation details to 2,000 characters and turns control
characters into spaces; the publisher rejects details that break either rule.
In the issue, scanner text cannot form Markdown links or images, mentions, or
`#` references; bare URLs still autolink and show their destination. Local
targets above the repository root are listed without a link.

Each UTC day contributes at most one observation. Missing jobs and incomplete
audits provide no evidence of uptime or downtime. Long observation gaps restart
the failure window. A successful observation resets the failure age; an existing
alert resolves only after two good observations on distinct days. Removed links
and active exceptions also resolve after two observations. An inconclusive
result cannot establish recovery. One inconclusive night does not erase failure
evidence; more than two days without a definitive failure restarts an unreported
outage window. Reports older than the stored observation timestamp are rejected,
including reruns on the same UTC day.

The small registry is saved in 30-day artifacts. The publisher first selects
successful scheduled and manual runs of this workflow on the default branch,
then looks for their published registry artifact. PR artifacts, other workflows,
failed runs, and dry-run previews cannot supply history or crowd out discovery.
The run's commit must be on the default branch itself, since tag runs report
the tag name as their branch. A run that uploaded a dry-run preview is skipped
even if it also holds a registry. The recovered registry is validated like
scanner data, and one malformed record rejects all of it.
If history is unavailable, unreported website failures start a fresh observation
window. Already reported targets are recovered from a compact snapshot in the
bot issue, preventing state loss from closing an unresolved issue. Snapshots keep
identities and recent recovery evidence within GitHub's body limit; exceptionally
large sets omit URL metadata, which the next complete audit restores. Full source
locations remain in the report artifact.
Published observations are reconciled even when a restored registry exists but
predates the issue. Snapshot timestamps distinguish separate incidents, while
observation days keep repeated checks from counting as multiple recovery days.
Automatic closure preserves a recovery checkpoint in the issue too. Restoring
older history cannot reopen a recovered alert; a new website outage must satisfy
the observation window again. Newer evidence of a recurrence remains intact.

The bot updates its managed issue section only when actionable findings change.
Its summary counts affected targets, references, and source files. Collapsible
sections separate local failures, verified GitHub targets, and persistent website
failures. Every row includes the evidence and named source links at the audited
revision; the issue also explains repairs, exceptions, and recovery.
The issue links directly to the uploaded `link-check-report` ZIP and its workflow
run. Downloads require GitHub sign-in and expire after the configured 30-day
retention period. A stable link to the audit workflow provides access to newer
reports if the original artifact expires. Local previews leave these links
unpopulated instead of inserting local filesystem paths or invented run URLs.
Repeated references to the same target are grouped with all source locations.
It preserves maintainer notes, does not add nightly comments, respects manual
closure for an ongoing incident, and can reopen for new findings or when failures
recur after two confirmed recovery days, including after a manual closure. Full
locations and all observations remain in the artifacts.
The registry remembers the bot issue number. If its automation marker is removed,
publishing stops for investigation instead of creating another issue, even if
the registry was lost. Restore the managed section from the issue edit history
to resume publishing; deleting the registry alone is not a reset. Transferred
issues are rejected before any write to an issue in this repository.

Manual workflow dispatch defaults to a dry run. Select `publish` on the default
branch to reconcile the issue and persist history. To inspect a completed audit
locally, run `make triage-doc-links`; it writes `issue-plan.json`,
`issue-preview.md`, and `state.json` without writing to GitHub. The preview does
not query an existing GitHub issue; live reconciliation happens only in the
trusted publishing job.

## Temporary exceptions

Use exact URLs in `.github/link-check-exceptions.json`. Every exception requires
a reason and an expiry date no more than one year ahead. Duplicate or malformed
entries are rejected, for example:

```json
{
  "version": 1,
  "exceptions": [
    {
      "url": "https://github.com/example/project/blob/main/docs/page.mdx#generated-heading",
      "reason": "Heading generated by a component; manually verified in a browser",
      "expires": "2026-12-01"
    }
  ]
}
```

For local targets, use `repo:/path/from/repository/root.md#heading`. Exceptions
remain active through the expiry date. Expired exceptions resume checking and
appear in the report. The history registry records observations; it is not an
allowlist of permanently ignored failures. Unused exceptions appear in the
summary so obsolete entries can be removed.

Notebook prose is checked using ignored temporary sibling files so relative links
keep their original base. Normal completion and SIGTERM remove those files.
Each Markdown cell is parsed separately by Lychee, preserving its code and HTML
boundaries. Notebook heading targets are checked across the individual cells.

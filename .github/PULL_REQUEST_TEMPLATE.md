<!--  Thanks for sending a pull request! Read the comments in this template, then delete every one. CI fails while the description contains `<!--` or `-->`.

Before you open it:
- If this is your first time, read our contributor guidelines https://www.kubeflow.org/docs/about/contributing/ and developer guide https://github.com/kserve/kserve/blob/master/docs/DEVELOPER_GUIDE.md
- Run `make precommit`. Open unfinished work as a draft.
- Re-running failed tests: `/rerun-failed` reruns only failed jobs, `/rerun-all` reruns all jobs, `/rerun-workflow <name>` reruns a specific workflow.

Writing the description (examples: https://github.com/kserve/kserve/blob/master/.github/PR_DESCRIPTION_EXAMPLES.md):
- A reviewer should get the point in one minute. Length follows risk, not diff size. The whole description, without the checklist, stays under 150 words for a bug fix and under 300 for a new API or behaviour change, including its example and upgrade story. A larger change can say more in <details>: design detail, alternatives, extra evidence.
- Title: `type(scope): lowercase behaviour`, at most 75 characters.
- Say what and why. Explain how only where the diff doesn't make it obvious, never as a walkthrough. No file lists, and no story of how you found the bug.
- Say each thing once. Delete a section that doesn't apply instead of writing "N/A".
- Leave out agent output ("Generated with" lines, bot summaries, task lists, session links), bold lead-ins, extra headings and emoji.
- Update the description when review changes the PR.
-->

**What this PR does / why we need it**:

<!-- Bug fix: what users saw and the root cause, then the fix. Feature: what users can do now and why they need it, then the change, with a short example. -->

**Which issue(s) this PR fixes**:

<!-- `Fixes #123` closes the issue on merge. Delete this section if there is no issue. -->

**Feature/Issue validation/testing**:

<!-- Results, not methods: the test that fails on master and passes here, or one before/after number. Mention a skipped check only when a reviewer would expect it. Put output longer than ten lines in <details>. -->

**Special notes for your reviewer**:

<!-- Breaking changes first: a removed or renamed field, flag, condition or label, a changed default, or behaviour existing users will notice. Say who is affected and what they must do, and put "action required" in the release note.
Then the decisions you want the reviewer's opinion on: what you chose, the alternative, and what it makes worse. Leave out limits that exist today anyway.
If the PR changes what a controller writes into workloads, say whether pods roll on upgrade; otherwise leave rollout out. Name what is out of scope and the follow-ups, one line each. Delete the section if there is nothing to say.
If this PR changes any image versions, that must be its only change. -->

**Checklist**:

<!-- Tick only what is true. No notes. -->

- [ ] Have you added unit/e2e tests that prove your fix is effective or that this feature works?
- [ ] Has code been commented, particularly in hard-to-understand areas?
- [ ] Have you made corresponding changes to the [documentation](https://github.com/kserve/website)?

**Release note**:

<!-- One or two sentences for the person upgrading: what changed for them and what they must do. Name the fields and flags they set, not functions.
Include "action required" if users must act when switching to the new release. Write NONE if nothing is user-visible. -->

```release-note

```

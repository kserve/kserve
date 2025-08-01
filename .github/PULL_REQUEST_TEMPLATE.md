<!--  Thanks for sending a pull request!  Here are some tips for you:
1. If this is your first time, read our contributor guidelines https://www.kubeflow.org/docs/about/contributing/ and developer guide https://github.com/kserve/kserve/blob/master/docs/DEVELOPER_GUIDE.md
2. Before raising a PR, please run `make precommit` to check the code style.
3. If you want *faster* PR reviews, read how: https://git.k8s.io/community/contributors/guide/pull-requests.md#best-practices-for-faster-reviews
4. Follow the instructions for writing a release note: https://git.k8s.io/community/contributors/guide/release-notes.md
5. If the PR is unfinished, see how to mark it: https://git.k8s.io/community/contributors/guide/pull-requests.md#marking-unfinished-pull-requests
-->

**What this PR does / why we need it**:

**Which issue(s) this PR fixes** *(optional, in `fixes #<issue number>(, fixes #<issue_number>, ...)` format, will close the issue(s) when PR gets merged)*:
Fixes #

**Type of changes**
Please delete options that are not relevant.

- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] This change requires a documentation update

**Feature/Issue validation/testing**:

Please describe the tests that you ran to verify your changes and relevant result summary. Provide instructions so it can be reproduced.
Please also list any relevant details for your test configuration.

- [ ] Test A
- [ ] Test B

- Logs

**Special notes for your reviewer**:

1. Please confirm that if this PR changes any image versions, then that's the sole change this PR makes.

**Checklist**:

- [ ] Have you added unit/e2e tests that prove your fix is effective or that this feature works?
- [ ] Has code been commented, particularly in hard-to-understand areas?
- [ ] Have you made corresponding changes to the documentation?

**Release note**:
<!--  Write your release note:
1. Enter your extended release note in the below block. If the PR requires additional action from users switching to the new release, include the string "action required".
2. If no release note is required, just write "NONE".
-->
```release-note

```

**Re-running failed tests**

- `/rerun-all` - rerun all failed workflows.
- `/rerun-workflow <workflow name>` - rerun a specific failed workflow. Only one workflow name can be specified. Multiple /rerun-workflow commands are allowed per comment.
---
name: kserve
description: Use before changing kserve code or opening a PR here. Maps each directory to the AGENTS.md guide reviewers expect you to have read (llmisvc controller, presets, API types and admission, v1alpha1 cache controllers, e2e, samples) and names the pre-PR self-review pass.
---

Read the root `AGENTS.md` first. Then, for every directory your change touches, read its guide before editing:

| Files under | Read |
|---|---|
| `pkg/controller/v1alpha2/llmisvc/` | `pkg/controller/v1alpha2/llmisvc/AGENTS.md`, then `GATES.md` there for anything that changes rendered output |
| `config/llmisvcconfig/` | `config/llmisvcconfig/AGENTS.md` |
| `pkg/apis/serving/`, `pkg/webhook/` | `pkg/apis/serving/AGENTS.md` |
| `pkg/controller/v1alpha1/` | `pkg/controller/v1alpha1/AGENTS.md` |
| `test/e2e/`, e2e workflows and scripts | `test/e2e/AGENTS.md` |
| `docs/samples/llmisvc/` | `docs/samples/llmisvc/AGENTS.md` |

Each guide ends with a "Before you open the PR" list. Before opening, have a fresh pass (another agent if you can) try to refute every item on the lists for the areas you touched, and every claim in your PR body, against the diff. A PR that spans areas gets one pass per area. Then run `make -j1 precommit` and check `git status` shows every regenerated file staged.

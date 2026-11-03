# Bump Version Agent

Automate the version bump step of a KServe release using GitHub Copilot coding agent.

Instead of running `make bump-version` locally, create a GitHub issue and let the agent handle it.

---

## How It Works

1. Create a release issue using the **Release** issue template
2. Fill in the **New Version** field (e.g., `X.Y.Z-rc0`, `X.Y.Z`)
3. Assign **Copilot** to the issue
4. The agent creates a PR with all version files updated

---

## Usage

### Step 1: Create a Release Issue

Go to **Issues → New issue → Release** and fill in the version:

| Field | Example |
|-------|---------|
| New Version | `X.Y.Z-rc0` |
| Additional Notes | _(optional)_ |

### Step 2: Assign Copilot with the bump-version agent

1. Click **Assignees** → select **Copilot**
2. Select the **`bump-version`** agent
3. Select the **base branch**:
   - RC0, RC1+, Final → `master` (default)
   - Z-release (Z > 0) → `release-X.Y`

> **Important:**
> - You must select the `bump-version` agent. If you skip this, Copilot runs without the release-specific instructions.
> - For z-releases, you must select the release branch as the base branch. Otherwise the PR targets `master`.

### Step 3: Review and Merge

Review the PR, verify CI passes, then merge.

---

## Version Types and Target Branches

| Version | Type | Target Branch | Base ref on assign | `cherrypick-approved` |
|---------|------|---------------|--------------------|-----------------------|
| `X.Y.0-rc0` | RC0 | `master` | _(default)_ | No |
| `X.Y.0-rcN` (N>0) | RC1+ | `master` | _(default)_ | Yes |
| `X.Y.0` | Final | `master` | _(default)_ | Yes |
| `X.Y.Z` (Z>0) | Z-release | `release-X.Y` | `release-X.Y` | No |

For z-releases, you **must** specify the release branch when assigning Copilot. Otherwise the PR targets `master`.

---

## What the Agent Does

1. Validates version format (`X.Y.Z` or `X.Y.Z-rcN`)
2. Determines the correct target branch
3. Reads the prior version from `kserve-deps.env`
4. Verifies the prior version tag exists
5. Runs `make bump-version`
6. Runs `make precommit` for formatting
7. Creates a PR targeting the correct branch

---

## After the PR Is Merged

Follow the remaining release steps in:

- [RELEASE_PROCESS.md](./RELEASE_PROCESS.md) (manual)
- [RELEASE_PROCESS_copilot.md](./RELEASE_PROCESS_copilot.md) (Copilot CLI)

---

## Agent Definition

The agent logic is defined in:

- [`.github/agents/bump-version.agent.md`](../.github/agents/bump-version.agent.md)
- [`.github/ISSUE_TEMPLATE/release.yml`](../.github/ISSUE_TEMPLATE/release.yml)

---

## Restarting a Failed or Incorrect Run

If the agent produces a wrong result or you need to retry:

1. **Close** the PR that the agent created
2. **Unassign** Copilot from the issue
3. **Re-assign** Copilot to the issue (select `bump-version` agent again, and set the base branch for z-releases)

The agent starts fresh on each assignment — it does not resume from a previous attempt.


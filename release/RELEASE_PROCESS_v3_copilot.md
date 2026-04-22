# KServe Release Process - Copilot CLI Guide

Run a KServe release with a single command using GitHub Copilot CLI.

---

## 1. Prerequisites

| Item | Requirement |
|------|-------------|
| GitHub account | Listed as **approver** in kserve/kserve OWNERS file |
| GitHub Copilot | Personal or org subscription active |
| `gh` CLI | v2.40.0 or later |
| OS | Linux / macOS / Windows (amd64 or arm64) |

---

## 2. Install & Setup

### Step 1: Install gh CLI

**macOS**
```bash
brew install gh
```

**Linux (apt)**
```bash
type -p curl >/dev/null || sudo apt install curl -y
curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
  | sudo dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
  | sudo tee /etc/apt/sources.list.d/github-cli.list > /dev/null
sudo apt update && sudo apt install gh -y
```

**Linux (rpm)**
```bash
sudo dnf install 'dnf-command(config-manager)'
sudo dnf config-manager --add-repo https://cli.github.com/packages/rpm/gh-cli.repo
sudo dnf install gh -y
```

Verify version:
```bash
gh --version  # must be v2.40.0+
```

### Step 2: Log in to gh

```bash
gh auth login
# → Select GitHub.com
# → Select HTTPS
# → Authenticate via browser or paste a token
```

Verify login:
```bash
gh auth status
```

### Step 3: Verify Copilot CLI

`gh copilot` is built into `gh` v2.40+. No separate installation required.

```bash
gh copilot -- --help  # check version and usage
```

### Step 4: Clone the repository

```bash
git clone git@github.com:<your-github-id>/kserve.git
cd kserve
git remote add upstream https://github.com/kserve/kserve.git
```

---

## 3. Run Release

```bash
gh copilot --agent release-orchestrator -i "Release v0.18.0-rc0"
```

---

## 4. What It Does

The `release-orchestrator` agent runs the following phases in order:

| Phase | Description |
|-------|-------------|
| 1 | Detect current version, identify release type, confirm with user |
| 2 | Version bump → commit → create PR |
| 2B | RC1+ only: create cherry-pick PR |
| 3 | Wait for CI to finish (`--watch`), run `/rerun-all` on failure |
| 4 | **[Approval required]** Merge PR |
| 5 | Verify branch & tag creation |
| 6 | **[Approval required]** Create draft release → publish |
| 7 | Verify PyPI / Helm deployment |
| 8 | Validate artifacts (`validate-release.sh`) |
| 9 | **[Approval required]** Kind-based smoke test |

There are 5 approval points total. Everything else is automated.

> Checkpoints are saved to `~/.kserve_release/checkpoint.json` during long-running steps. If the session is interrupted, the agent will detect the checkpoint on restart and offer to resume.

---

## 5. Release Types

### RC0

```bash
gh copilot --agent release-orchestrator -i "Release v0.18.0-rc0"
```

### RC1+

```bash
gh copilot --agent release-orchestrator -i "Release v0.18.0-rc1"
```

The cherry-pick phase is added automatically.

### Final

```bash
gh copilot --agent release-orchestrator -i "Release v0.18.0"
```

> ⚠️ Final release is still in testing. Verify before running.

---

## 6. Agent Definition

The agent logic is defined in:

- [`.github/agents/release-orchestrator.agent.md`](../.github/agents/release-orchestrator.agent.md)

Edit that file to modify the release workflow.

---

## 7. Troubleshooting

| Symptom | Fix |
|---------|-----|
| `gh copilot` not recognized | Check `gh --version`, must be v2.40+ |
| Copilot permission error | Re-auth with `gh auth login --scopes copilot` |
| `--agent` option missing | Run `gh copilot -- --help` to check version |
| CI keeps failing | Retry or select `abort` in Phase 3 |

---

## 8. Related Documents

- [RELEASE_PROCESS_v3.md](./RELEASE_PROCESS_v3.md)

# KServe Release Process v3 (Automated)

Simplified and automated KServe release process (5~7 weeks) using scripts and GitHub Actions.

## Quick Reference

| Week | Event |
|------|-------|
| 1-4  | Development|
| 4   | Announce feature freeze |
| 5   | RC0 Released |
| 6   | RC1+ Released (if needed) |
| 7   | Final Release |

## Prerequisites for executing GitHub Actions

- Listed in [OWNERS](../OWNERS) file (reviewer+)
- Push access to kserve/kserve

> **Note:** If the person executing the release process is not a reviewer, once the prepare release PR is merged, they must ask a reviewer to trigger the required GitHub Actions.

## Release Types

| Version | Branch Created | Use Case |
|---------|----------------|----------|
| v0.17.0-rc0 | ✅ Yes | First release candidate |
| v0.17.0-rc1 | ❌ No | Bug fixes after RC0 |
| v0.17.0 | ❌ No | Final official release |

---

## RC0: Initial Release Candidate

> **Note:** This guide uses v0.17.0 as an example. Replace with your actual release version.

### 1. Prepare and Merge
```bash
git clone git@github.com:YOUR_ORG/kserve.git
cd kserve

git checkout -b release/0.17.0-rc0

# Prepare release
make bump-version NEW_VERSION=0.17.0-rc0 PRIOR_VERSION=0.16.0

# Push release branch to your repository
git add .
git commit -m "release: prepare release v0.17.0-rc0"
git push -u origin release/0.17.0-rc0

# Create PR to master in upstream kserve via GitHub UI
```

### 2. Prepare Release (Branch & Tag)

**GitHub Actions:**
1. Go to **Actions** → **Prepare Release (Branch & Tag)** → **Run workflow**
2. Set `version: v0.17.0-rc0`, `dry_run: true`
3. Review output, then run with `dry_run: false`

**Local Script (only for testing):**
```bash
./hack/release/create-release.sh v0.17.0-rc0 --dry-run
```

This creates:
- Branch: `release-0.17` (new)
- Tag: `v0.17.0-rc0`

### 3. Create GitHub Release (Manual)

> **Note:** Only project leads (Dan Sun, Yuan Tang) can create GitHub Releases. Please ask them to create the release after the branch and tag are prepared.

After the workflow completes, create the GitHub Release manually:

1. **Go to Release Creation Page:**
   - URL: https://github.com/kserve/kserve/releases/new?tag=v0.17.0-rc0
   - Or: Releases → Draft a new release → Choose tag: `v0.17.0-rc0`

2. **Fill in Release Information:**
   - Title: `KServe v0.17.0-rc0`
   - ✅ Check "Set as a pre-release"

3. **Write Release Notes:**
   - Installation guide link (https://kserve.github.io/website/docs/next/getting-started/quickstart-guide)
   - What's changed
   - Breaking changes (if any)

4. **Attach Install Files:**
   - Files are in: `install/v0.17.0-rc0/`

5. **Click "Publish release"**

**This automatically triggers:**
- ✅ **PyPI Publishing:** Packages are uploaded to PyPI
  - KServe: <https://pypi.org/project/kserve/>
  - Storage: <https://pypi.org/project/kserve-storage/>
- ✅ **Helm Publishing:** Charts are pushed to GHCR and attached to release
  - GHCR: <https://github.com/orgs/kserve/packages>

**Verify:**
- Check workflow: **Actions** → **Upload Python Package**
- Check workflow: **Actions** → **helm-publish**

### 4. Announce
```
📢 KServe v0.17.0-rc0 is now available!
Release: https://github.com/kserve/kserve/releases/tag/v0.17.0-rc0
Please test and report bugs. Feature freeze is now in effect.
```

---

## RC1+: Bug Fix Release Candidates

### 1. Fix Bugs
- Fix bugs in master
- Label PR with `cherrypick-approved`
- Merge to master

### 2. Cherry-pick
```bash
# In merged PR, comment:
/cherry-pick release-0.17
```

### 3. Prepare and Merge
```bash
make bump-version NEW_VERSION=0.17.0-rc1 PRIOR_VERSION=0.17.0-rc0
# Create PR with cherrypick-approved label, merge to master
# Cherry-pick: /cherry-pick release-0.17
```

### 4. Prepare Release (Tag Only)

**GitHub Actions (Recommended):**

1. Go to **Actions** → **Prepare Release (Branch & Tag)** → **Run workflow**
2. Set `version: v0.17.0-rc1`, `dry_run: true`
3. Review output, then run with `dry_run: false`

**Local Script (only for testing):**

```bash
./hack/release/create-release.sh v0.17.0-rc1 --dry-run
```


### 5. Create GitHub Release (Manual)

> **Note:** Only project leads (Dan Sun, Yuan Tang) can create GitHub Releases. Please ask them to create the release after the branch and tag are prepared.

Follow the same steps as RC0 to create the release manually, then PyPI and Helm will be automatically published.

---

## Final Release

### 1. Prepare and Merge
```bash
make bump-version NEW_VERSION=0.17.0 PRIOR_VERSION=0.17.0-rc1
# Create PR with cherrypick-approved label, merge to master
# Cherry-pick: /cherry-pick release-0.17
```

### 2. Prepare Release (Tag Only)

**GitHub Actions (Recommended):**

1. Go to **Actions** → **Prepare Release (Branch & Tag)** → **Run workflow**
2. Set `version: v0.17.0`, `dry_run: true`
3. Review output, then run with `dry_run: false`

**Local Script (only for testing):**

```bash
./hack/release/create-release.sh v0.17.0 --dry-run
```

### 3. Create GitHub Release (Manual)

> **Note:** Only project leads (Dan Sun, Yuan Tang) can create GitHub Releases. Please ask them to create the release after the branch and tag are prepared.

Follow the same steps as RC0 to create the release manually, then PyPI and Helm will be automatically published.

---

## Resources

- Scripts: [`prepare-for-release.sh`](../hack/release/prepare-for-release.sh), [`create-release.sh`](../hack/release/create-release.sh)
- Workflows: [`prepare-release.yml`](../.github/workflows/prepare-release.yml), [`python-publish.yml`](../.github/workflows/python-publish.yml)
- Help: `./hack/release/create-release.sh --help`

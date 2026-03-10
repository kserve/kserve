# KServe Release Process v3

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

### 0. Set Release Variables

```bash
# Set these variables for your release (example: 0.17.0)
export NEW_VERSION=0.17.0
export PRIOR_VERSION=0.16.0
```

### 1. Prepare and Merge

```bash
git clone git@github.com:YOUR_ORG/kserve.git
cd kserve

git checkout -b release/${NEW_VERSION}-rc0

# Prepare release (uses variables from step 0)
make bump-version NEW_VERSION=${NEW_VERSION}-rc0 PRIOR_VERSION=${PRIOR_VERSION}

# Push release branch to your repository
git add .
git commit -S -s -m "release: prepare release v${NEW_VERSION}-rc0"
git push -u origin release/${NEW_VERSION}-rc0

# Create PR to master in upstream kserve via GitHub UI
```

### 2. Prepare Release (Branch & Tag)

**GitHub Actions:**
1. Go to **Actions** → **Prepare Release (Branch & Tag)** → **Run workflow**
2. Set `version: v${NEW_VERSION}-rc0`, `dry_run: true`
3. Review output, then run with `dry_run: false`

**Local Script (only for testing):**
```bash
./hack/release/create-release.sh v${NEW_VERSION}-rc0 --dry-run
```

This creates:
- Branch: `release-${NEW_VERSION%.*}` (e.g., `release-0.17`)
- Tag: `v${NEW_VERSION}-rc0`

### 3. Review and Publish Draft Release

The workflow automatically creates a **Draft Release** with:

- ✅ Release notes (auto-generated from commits)
- ✅ Install files (from `install/v${NEW_VERSION}-rc0/`)
- ✅ Pre-release flag (for RC versions)

> **Note:** Approvers or above (listed in [OWNERS](../OWNERS)) can publish GitHub Releases.

**To publish the release:**

1. **Review the Draft Release:**
   - Go to: <https://github.com/kserve/kserve/releases>
   - Find the draft release for `v${NEW_VERSION}-rc0`

2. **Edit if needed:**
   - Update release notes
   - Add breaking changes
   - Highlight important features

3. **Publish the Release:**
   - Verify "Set as a pre-release" is checked
   - Click **"Publish release"** button

**Publishing automatically triggers:**

- ✅ **PyPI Publishing:** `python-publish` workflow uploads packages
  - KServe: <https://pypi.org/project/kserve/>
  - Storage: <https://pypi.org/project/kserve-storage/>
- ✅ **Helm Publishing:** `helm-publish` workflow pushes charts to GHCR
  - GHCR: <https://github.com/orgs/kserve/packages>

**Verify workflows:**

- **Actions** → **Upload Python Package**
- **Actions** → **helm-publish**

### 4. Announce

```bash
echo "📢 KServe v${NEW_VERSION}-rc0 is now available!"
echo "Release: https://github.com/kserve/kserve/releases/tag/v${NEW_VERSION}-rc0"
echo "Please test and report bugs. Feature freeze is now in effect."
```

---

## RC1+: Bug Fix Release Candidates

### 0. Set Release Variables

```bash
# Update these variables for RC1
export NEW_VERSION=0.17.0
export PRIOR_VERSION=0.17.0-rc0  # Previous RC version
```

### 1. Fix Bugs

- Fix bugs in master
- Label PR with `cherrypick-approved`
- Merge to master

### 2. Cherry-pick

```bash
# In merged PR, comment:
/cherry-pick release-${NEW_VERSION%.*}
```

### 3. Prepare and Merge

```bash
make bump-version NEW_VERSION=${NEW_VERSION}-rc1 PRIOR_VERSION=${PRIOR_VERSION}
# Create PR with cherrypick-approved label, merge to master
# Cherry-pick: /cherry-pick release-${NEW_VERSION%.*}  ex. /cherry-pick release-0.17
```

### 4. Prepare Release (Tag Only)

**GitHub Actions (Recommended):**

1. Go to **Actions** → **Prepare Release (Branch & Tag)** → **Run workflow**
2. Set `version: v${NEW_VERSION}-rc1`, `dry_run: true`
3. Review output, then run with `dry_run: false`

**Local Script (only for testing):**

```bash
./hack/release/create-release.sh v${NEW_VERSION}-rc1 --dry-run
```


### 5. Review and Publish Draft Release

> **Note:** Approvers or above (listed in [OWNERS](../OWNERS)) can publish GitHub Releases.

The workflow automatically creates a Draft Release. Follow the same review and publish process as RC0:

1. Review draft at: <https://github.com/kserve/kserve/releases>
2. Edit release notes if needed
3. Verify "Set as a pre-release" is checked
4. Click **"Publish release"**

Publishing automatically triggers `python-publish` and `helm-publish` workflows.

---

## Final Release

### 0. Set Release Variables

```bash
# Update these variables for final release
export NEW_VERSION=0.17.0
export PRIOR_VERSION=0.17.0-rc1  # Last RC version (or rc0 if no RC1)
```

### 1. Prepare and Merge

```bash
make bump-version NEW_VERSION=${NEW_VERSION} PRIOR_VERSION=${PRIOR_VERSION}
# Create PR with cherrypick-approved label, merge to master
# Cherry-pick: /cherry-pick release-${NEW_VERSION%.*}  ex. /cherry-pick release-0.17
```

### 2. Prepare Release (Tag Only)

**GitHub Actions (Recommended):**

1. Go to **Actions** → **Prepare Release (Branch & Tag)** → **Run workflow**
2. Set `version: v${NEW_VERSION}`, `dry_run: true`
3. Review output, then run with `dry_run: false`

**Local Script (only for testing):**

```bash
./hack/release/create-release.sh v${NEW_VERSION} --dry-run
```

### 3. Review and Publish Final Release

> **Note:** Approvers or above (listed in [OWNERS](../OWNERS)) can publish GitHub Releases.

The workflow automatically creates a Draft Release (without pre-release flag). Follow the review process:

1. Review draft at: <https://github.com/kserve/kserve/releases>
2. Edit release notes if needed
3. **Ensure "Set as a pre-release" is unchecked** (should already be unchecked)
4. Click **"Publish release"**

Publishing automatically triggers `python-publish` and `helm-publish` workflows.

---

## Resources

- Scripts: [`prepare-for-release.sh`](../hack/release/prepare-for-release.sh), [`create-release.sh`](../hack/release/create-release.sh)
- Workflows: [`prepare-release.yml`](../.github/workflows/prepare-release.yml), [`python-publish.yml`](../.github/workflows/python-publish.yml)
- Help: `./hack/release/create-release.sh --help`

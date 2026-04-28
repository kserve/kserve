#!/usr/bin/env bash
# Publish a KServe GitHub Release with install artifacts.
#
# This script must be run with a user-authenticated `gh` token (not GITHUB_TOKEN)
# so that the release is attributed to the user and downstream workflows
# (pypi, helm) are triggered on publish.
#
# Usage:
#   ./hack/release/publish-release.sh <version> [--draft] [--dry-run]
#
# Examples:
#   ./hack/release/publish-release.sh v0.18.0-rc0
#   ./hack/release/publish-release.sh v0.18.0-rc0 --draft
#   ./hack/release/publish-release.sh v0.18.0 --dry-run

set -eo pipefail

RED='\033[31m'
GREEN='\033[32m'
YELLOW='\033[33m'
NC='\033[0m'

usage() {
    echo "Usage: $0 <version> [--draft] [--dry-run]"
    echo ""
    echo "Arguments:"
    echo "  version     Release version with 'v' prefix (e.g., v0.18.0-rc0, v0.18.0)"
    echo ""
    echo "Options:"
    echo "  --draft       Create as draft release"
    echo "  --dry-run     Validate only, do not create release"
    echo "  --repo=OWNER/REPO  Target repo (default: auto-detected from git remote)"
    echo ""
    echo "Examples:"
    echo "  $0 v0.18.0-rc0"
    echo "  $0 v0.18.0-rc0 --draft"
    echo "  $0 v0.18.0-rc0 --repo=kserve/kserve --draft"
    echo "  $0 v0.18.0 --dry-run"
    exit 1
}

# Parse arguments
VERSION=""
DRAFT=false
DRY_RUN=false
REPO="kserve/kserve"

for arg in "$@"; do
    case "$arg" in
        --draft) DRAFT=true ;;
        --dry-run) DRY_RUN=true ;;
        --repo=*) REPO="${arg#--repo=}" ;;
        --help|-h) usage ;;
        v*) VERSION="$arg" ;;
        *) echo -e "${RED}Unknown argument: $arg${NC}"; usage ;;
    esac
done

if [[ -z "$VERSION" ]]; then
    echo -e "${RED}Error: version is required${NC}"
    usage
fi

# Validate version format
if ! [[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-rc[0-9]+)?$ ]]; then
    echo -e "${RED}Error: invalid version format: $VERSION${NC}"
    echo "Expected: vX.Y.Z or vX.Y.Z-rcN"
    exit 1
fi

echo -e "${GREEN}Publishing release: $VERSION${NC}"

# Step 1: Check gh auth
echo -e "\n${YELLOW}[1/6] Checking gh authentication...${NC}"
GH_USER=$(gh api user --jq '.login' 2>/dev/null || true)
if [[ -z "$GH_USER" ]]; then
    echo -e "${RED}Error: not authenticated with gh. Run 'gh auth login' first.${NC}"
    exit 1
fi
echo -e "  Authenticated as: ${GREEN}$GH_USER${NC}"

# Step 2: Check tag exists
echo -e "\n${YELLOW}[2/6] Checking tag exists...${NC}"
if [[ -z "$REPO" ]]; then
    REPO=$(gh repo view --json nameWithOwner --jq '.nameWithOwner' 2>/dev/null || echo "kserve/kserve")
fi
if ! gh api "repos/$REPO/git/ref/tags/$VERSION" >/dev/null 2>&1; then
    echo -e "${RED}Error: tag $VERSION does not exist.${NC}"
    echo "Run 'release-branch-tag' workflow first."
    exit 1
fi
echo -e "  Tag ${GREEN}$VERSION${NC} exists"

# Step 3: Check release does not already exist
echo -e "\n${YELLOW}[3/6] Checking release does not exist...${NC}"
if gh release view "$VERSION" --repo "$REPO" >/dev/null 2>&1; then
    echo -e "${RED}Error: release $VERSION already exists!${NC}"
    gh release view "$VERSION" --repo "$REPO" --json url --jq '.url'
    exit 1
fi
echo -e "  Release ${GREEN}$VERSION${NC} does not exist yet"

# Step 4: Check install files
echo -e "\n${YELLOW}[4/6] Checking install files...${NC}"
INSTALL_DIR="install/$VERSION"
if [[ ! -d "$INSTALL_DIR" ]]; then
    echo -e "${RED}Error: install directory not found: $INSTALL_DIR${NC}"
    echo "Run 'make bump-version' and 'generate-install.sh' first."
    exit 1
fi
FILE_COUNT=$(find "$INSTALL_DIR" -type f | wc -l)
echo -e "  Found ${GREEN}$FILE_COUNT${NC} files in $INSTALL_DIR"

if [[ "$FILE_COUNT" -eq 0 ]]; then
    echo -e "${RED}Error: no files in $INSTALL_DIR${NC}"
    exit 1
fi

# Step 5: Build release flags
echo -e "\n${YELLOW}[5/6] Preparing release...${NC}"
FLAGS=""
if [[ "$VERSION" == *"-rc"* ]]; then
    FLAGS="$FLAGS --prerelease"
    echo -e "  Type: ${YELLOW}pre-release${NC}"
else
    echo -e "  Type: ${GREEN}final release${NC}"
fi

if [[ "$DRAFT" == "true" ]]; then
    FLAGS="$FLAGS --draft"
    echo -e "  Status: ${YELLOW}draft${NC}"
fi

# Dry-run stop
if [[ "$DRY_RUN" == "true" ]]; then
    echo -e "\n${YELLOW}[DRY-RUN] Would create release with:${NC}"
    echo "  gh release create $VERSION"
    echo "    --repo $REPO"
    echo "    --title \"$VERSION\""
    echo "    --generate-notes"
    echo "    $FLAGS"
    echo "    $INSTALL_DIR/*"
    echo -e "\n${GREEN}Dry-run complete. No changes made.${NC}"
    exit 0
fi

# Step 6: Create release
echo -e "\n${YELLOW}[6/6] Creating release...${NC}"
gh release create "$VERSION" \
    --repo "$REPO" \
    --title "$VERSION" \
    --generate-notes \
    $FLAGS \
    "$INSTALL_DIR"/*

echo -e "\n${GREEN}Release $VERSION created successfully!${NC}"
echo ""
gh release view "$VERSION" --repo "$REPO" --json url --jq '.url'
echo ""
if [[ "$DRAFT" == "true" ]]; then
    echo -e "${YELLOW}Note: This is a draft release. Publish it manually to trigger downstream workflows.${NC}"
else
    echo -e "Downstream workflows should be triggered automatically:"
    echo "  - Helm charts publication (helm-publish)"
    echo "  - Python packages publication (python-publish)"
fi

#!/usr/bin/env bash
# Validate all KServe release artifacts after publishing.
#
# Usage:
#   ./hack/release/validate-release.sh <version> [--repo=<owner/repo>]
#
# Examples:
#   ./hack/release/validate-release.sh v0.18.0-rc0
#   ./hack/release/validate-release.sh v0.18.0-rc0 --repo=jooho/kserve

set -eo pipefail

RED='\033[31m'
GREEN='\033[32m'
YELLOW='\033[33m'
BLUE='\033[34m'
NC='\033[0m'

PASS=0
FAIL=0

usage() {
    echo "Usage: $0 <version> [--repo=<owner/repo>] [--images-only]"
    echo ""
    echo "Arguments:"
    echo "  version              Release version with 'v' prefix (e.g., v0.18.0-rc0)"
    echo ""
    echo "Options:"
    echo "  --repo=<owner/repo>  Target repository (default: auto-detected)"
    echo "  --images-only        Only check container images (skip other checks)"
    echo ""
    echo "Examples:"
    echo "  $0 v0.18.0-rc0"
    echo "  $0 v0.18.0-rc0 --repo=jooho/kserve"
    echo "  $0 v0.18.0-rc0 --images-only"
    exit 1
}

check_pass() {
    echo -e "  ${GREEN}✓${NC} $1"
    ((PASS++)) || true
}

check_fail() {
    echo -e "  ${RED}✗${NC} $1"
    if [[ -n "$2" ]]; then
        echo -e "    ${YELLOW}Fix:${NC} $2"
    fi
    ((FAIL++)) || true
}

check_warn() {
    echo -e "  ${YELLOW}~${NC} $1"
}

# Parse arguments
VERSION=""
REPO_OVERRIDE=""
IMAGES_ONLY=false

for arg in "$@"; do
    case "$arg" in
        --repo=*) REPO_OVERRIDE="${arg#--repo=}" ;;
        --images-only) IMAGES_ONLY=true ;;
        --help|-h) usage ;;
        v*) VERSION="$arg" ;;
        *) echo -e "${RED}Unknown argument: $arg${NC}"; usage ;;
    esac
done

if [[ -z "$VERSION" ]]; then
    echo -e "${RED}Error: version is required${NC}"
    usage
fi

if ! [[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-rc[0-9]+)?$ ]]; then
    echo -e "${RED}Error: invalid version format: $VERSION${NC}"
    exit 1
fi

# Resolve repo
if [[ -n "$REPO_OVERRIDE" ]]; then
    REPO="$REPO_OVERRIDE"
else
    REPO=$(gh repo view --json nameWithOwner --jq '.nameWithOwner' 2>/dev/null || echo "kserve/kserve")
fi

# Parse version components
VERSION_NO_V="${VERSION#v}"
BASE_VERSION=$(echo "$VERSION_NO_V" | sed 's/-rc[0-9]*$//')
MAJOR_MINOR=$(echo "$BASE_VERSION" | cut -d. -f1,2)
RC=$(echo "$VERSION_NO_V" | grep -o 'rc[0-9]*$' || echo "")
BRANCH="release-${MAJOR_MINOR}"

REGISTRY="docker.io/kserve"
IMAGES=(
    "kserve-controller"
    "agent"
    "router"
    "storage-initializer"
    "sklearnserver"
    "lgbserver"
    "xgbserver"
    "pmmlserver"
    "huggingfaceserver"
    "llmisvc-controller"
    "kserve-localmodel-controller"
    "kserve-localmodelnode-agent"
)

echo ""
echo -e "${BLUE}=================================================${NC}"
echo -e "${BLUE}  KServe Release Validation: $VERSION${NC}"
echo -e "${BLUE}  Repo: $REPO${NC}"
echo -e "${BLUE}=================================================${NC}"

if [[ "$IMAGES_ONLY" == "true" ]]; then
    echo -e "${YELLOW}  Mode: images-only${NC}"
fi

# ── 1. Install files ──────────────────────────────────────────
if [[ "$IMAGES_ONLY" != "true" ]]; then
echo ""
echo -e "${YELLOW}[1/6] Install files...${NC}"
INSTALL_DIR="install/${VERSION}"
if [[ -d "$INSTALL_DIR" ]]; then
    FILE_COUNT=$(find "$INSTALL_DIR" -type f | wc -l)
    if [[ "$FILE_COUNT" -gt 0 ]]; then
        check_pass "install/${VERSION}/ exists ($FILE_COUNT files)"
    else
        check_fail "install/${VERSION}/ is empty" "Run 'make bump-version' and 'generate-install.sh'"
    fi
else
    check_fail "install/${VERSION}/ not found" "Run 'make bump-version' and 'generate-install.sh'"
fi

# ── 2. Branch ─────────────────────────────────────────────────
echo ""
echo -e "${YELLOW}[2/6] Git branch...${NC}"
if gh api "repos/${REPO}/branches/${BRANCH}" >/dev/null 2>&1; then
    check_pass "Branch ${BRANCH} exists"
else
    if [[ "$RC" == "rc0" || -z "$RC" ]]; then
        check_fail "Branch ${BRANCH} not found" "Re-run 'Prepare Release (Branch & Tag)' workflow"
    else
        check_warn "Branch ${BRANCH} not found (expected for RC0 only)"
    fi
fi

# ── 3. Tag ────────────────────────────────────────────────────
echo ""
echo -e "${YELLOW}[3/6] Git tag...${NC}"
if gh api "repos/${REPO}/git/ref/tags/${VERSION}" >/dev/null 2>&1; then
    check_pass "Tag ${VERSION} exists"
else
    check_fail "Tag ${VERSION} not found" "Re-run 'Prepare Release (Branch & Tag)' workflow"
fi

# ── 4. GitHub Release ─────────────────────────────────────────
echo ""
echo -e "${YELLOW}[4/6] GitHub Release...${NC}"
IS_DRAFT=$(gh release view "${VERSION}" --repo "${REPO}" --json isDraft --jq '.isDraft' 2>/dev/null || echo "")
IS_PRERELEASE=$(gh release view "${VERSION}" --repo "${REPO}" --json isPrerelease --jq '.isPrerelease' 2>/dev/null || echo "")
RELEASE_URL=$(gh release view "${VERSION}" --repo "${REPO}" --json url --jq '.url' 2>/dev/null || echo "")

if [[ -z "$IS_DRAFT" ]]; then
    check_fail "GitHub Release ${VERSION} not found" "Run publish-release.sh"
else
    if [[ "$IS_DRAFT" == "true" ]]; then
        check_fail "GitHub Release is still a draft" "gh release edit ${VERSION} --repo=${REPO} --draft=false"
    else
        check_pass "GitHub Release published: $RELEASE_URL"
    fi

    if [[ -n "$RC" && "$IS_PRERELEASE" != "true" ]]; then
        check_fail "RC release should be marked as pre-release" "gh release edit ${VERSION} --repo=${REPO} --prerelease"
    elif [[ -z "$RC" && "$IS_PRERELEASE" == "true" ]]; then
        check_fail "Final release should NOT be marked as pre-release" "gh release edit ${VERSION} --repo=${REPO} --latest"
    else
        check_pass "Pre-release flag correct (isPrerelease=${IS_PRERELEASE})"
    fi
fi

# ── 5. PyPI packages ──────────────────────────────────────────
echo ""
echo -e "${YELLOW}[5/6] PyPI packages...${NC}"
PYPI_VERSION="${VERSION_NO_V}"
for pkg in kserve kserve-storage; do
    if curl -sf "https://pypi.org/pypi/${pkg}/${PYPI_VERSION}/json" >/dev/null 2>&1; then
        check_pass "PyPI: ${pkg}==${PYPI_VERSION}"
    else
        check_fail "PyPI: ${pkg}==${PYPI_VERSION} not found" "Check 'Upload Python Package' workflow"
    fi
done

fi  # end IMAGES_ONLY skip

# ── 6. Container images ───────────────────────────────────────
echo ""
echo -e "${YELLOW}[6/6] Container images...${NC}"
IMAGE_FAIL_COUNT=0
for image in "${IMAGES[@]}"; do
    IMAGE_REF="${REGISTRY}/${image}:${VERSION}"
    if docker manifest inspect "${IMAGE_REF}" >/dev/null 2>&1; then
        check_pass "Image: ${IMAGE_REF}"
    else
        check_fail "Image: ${IMAGE_REF} not found" "Check Docker Publisher workflows"
        ((IMAGE_FAIL_COUNT++)) || true
    fi
done


# ── Summary ───────────────────────────────────────────────────
echo ""
echo -e "${BLUE}=================================================${NC}"
TOTAL=$((PASS + FAIL))
if [[ "$FAIL" -eq 0 ]]; then
    echo -e "${GREEN}  All checks passed ($PASS/$TOTAL)${NC}"
    echo -e "${GREEN}  Release ${VERSION} is fully validated!${NC}"
else
    echo -e "${RED}  $FAIL check(s) failed out of $TOTAL${NC}"
    echo -e "${YELLOW}  Fix the issues above and re-run this script.${NC}"
fi
echo -e "${BLUE}=================================================${NC}"
echo ""

[[ "$FAIL" -eq 0 ]]

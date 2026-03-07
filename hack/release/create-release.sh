#!/usr/bin/env bash
# Helper script to create KServe releases

set -eo pipefail

# Detect the upstream remote (kserve/kserve)
detect_upstream_remote() {
    # Check all remotes for kserve/kserve
    for remote in $(git remote); do
        local url=$(git remote get-url "$remote" 2>/dev/null || echo "")
        if [[ "$url" == *"kserve/kserve"* ]]; then
            echo "$remote"
            return 0
        fi
    done

    # Not found
    echo ""
    return 1
}

# ============================================================
# Color codes for output
# ============================================================
RED='\033[31m'
GREEN='\033[32m'
YELLOW='\033[33m'
BLUE='\033[34m'
NC='\033[0m' # No Color

# ============================================================
# Global variables
# ============================================================
VERSION=""
DRY_RUN=false
VALIDATE_ONLY=false
GITHUB_ACTIONS_MODE=false
BASE_VERSION=""
RC=""
BRANCH=""
RELEASE_TYPE=""

# ============================================================
# Helper functions
# ============================================================

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_section() {
    echo ""
    echo -e "${GREEN}$1${NC}"
}

usage() {
    cat <<EOF
Usage: $0 <version> [OPTIONS]

Create a KServe release (branch and tag)

Arguments:
  version           Release version (e.g., v0.17.0-rc0, v0.17.0-rc1, v0.17.0)

Options:
  --dry-run         Validate and show execution plan without making changes
  --validate-only   Only run validation checks and exit
  --github-actions  Enable GitHub CLI checks (for CI environment)
  -h, --help        Show this help message

Examples:
  # Validate version and show execution plan
  $0 v0.17.0-rc0 --dry-run

  # Create RC0 (branch + tag)
  $0 v0.17.0-rc0

  # Create RC1 (tag only)
  $0 v0.17.0-rc1

  # Create final release (tag only)
  $0 v0.17.0

Release Types:
  v0.17.0-rc0  → RC0 (creates release-0.17 branch + tag)
  v0.17.0-rc1  → RC1+ (creates tag on existing branch)
  v0.17.0      → Final (creates tag on existing branch)

Note:
  - This script creates branches and tags only
  - GitHub Release is created by GitHub Actions workflow
  - Requires push access to kserve/kserve repository

EOF
    exit 0
}

# ============================================================
# Validation functions
# ============================================================

validate_version_format() {
    print_section "🔍 Phase 1: Validating version format..."

    local pattern="^v[0-9]+\.[0-9]+\.[0-9]+(-rc[0-9]+)?$"
    if [[ ! $VERSION =~ $pattern ]]; then
        print_error "Invalid version format: $VERSION"
        echo ""
        echo "Valid formats:"
        echo "  - v0.17.0-rc0 (Release Candidate 0)"
        echo "  - v0.17.0-rc1 (Release Candidate 1)"
        echo "  - v0.17.0     (Final Release)"
        exit 1
    fi

    print_success "Version format is valid"
}

parse_version() {
    print_section "🔍 Parsing version components..."

    # Remove leading 'v'
    local version_no_v=$(echo "$VERSION" | sed 's/^v//')

    # Extract base version (e.g., 0.17.0)
    BASE_VERSION=$(echo "$version_no_v" | sed 's/-rc[0-9]*$//')

    # Extract major.minor only for branch name (e.g., 0.17)
    # KServe uses release-0.16, release-0.17 (not release-0.16.0)
    local major_minor=$(echo "$BASE_VERSION" | cut -d. -f1,2)

    # Extract RC suffix (e.g., rc0, rc1, or empty for final)
    RC=$(echo "$version_no_v" | grep -o 'rc[0-9]*$' || echo "")

    # Determine release type
    if [[ "$RC" == "rc0" ]]; then
        RELEASE_TYPE="RC0 (Initial Release Candidate)"
    elif [[ "$RC" =~ ^rc[1-9][0-9]*$ ]]; then
        RELEASE_TYPE="RC${RC#rc}+ (Bug Fix Release Candidate)"
    elif [[ -z "$RC" ]]; then
        RELEASE_TYPE="Final Release"
    else
        print_error "Invalid RC format: $RC"
        exit 1
    fi

    BRANCH="release-${major_minor}"

    echo "  Base Version: $BASE_VERSION"
    echo "  RC Suffix: ${RC:-none}"
    echo "  Release Type: $RELEASE_TYPE"
    echo "  Target Branch: $BRANCH"
    print_success "Version parsed successfully"
}

validate_kserve_deps() {
    print_section "🔍 Phase 2: Validating kserve-deps.env..."

    if [[ ! -f "kserve-deps.env" ]]; then
        print_error "kserve-deps.env file not found!"
        exit 1
    fi

    local current_version=$(grep KSERVE_VERSION= kserve-deps.env | cut -d= -f2)

    if [[ "$current_version" != "$VERSION" ]]; then
        print_error "Version mismatch!"
        echo "   Input version: $VERSION"
        echo "   kserve-deps.env: $current_version"
        echo ""
        echo "Please run prepare-for-release.sh first:"
        echo "  ./hack/release/prepare-for-release.sh <prior_version> <new_version>"
        exit 1
    fi

    print_success "kserve-deps.env version matches: $current_version"
}

validate_tag_duplicate() {
    print_section "🔍 Phase 3: Checking for duplicate tag..."

    if git rev-parse "$VERSION" >/dev/null 2>&1; then
        print_error "Tag $VERSION already exists!"
        echo ""
        echo "Existing tag information:"
        git show -s --format='%h %ci %s' "$VERSION"
        exit 1
    fi

    print_success "Tag $VERSION does not exist (OK)"
}

validate_github_release_duplicate() {
    print_section "🔍 Phase 4: Checking for duplicate GitHub Release..."

    # Skip actual check if --github-actions is not set
    if [[ "$GITHUB_ACTIONS_MODE" == false ]]; then
        print_info "GitHub Release check requires --github-actions flag"
        return
    fi

    # Check if gh CLI is available
    if ! command -v gh >/dev/null 2>&1; then
        print_warning "gh CLI not found, skipping GitHub Release duplicate check"
        print_info "Install gh CLI to enable this check: https://cli.github.com/"
        return
    fi

    # Check if release exists
    if gh release view "$VERSION" --repo kserve/kserve >/dev/null 2>&1; then
        print_error "GitHub Release $VERSION already exists!"
        echo ""
        echo "Existing release information:"
        gh release view "$VERSION" --repo kserve/kserve --json name,tagName,isPrerelease,createdAt,url \
            --template '{{printf "  Name: %s\n  Tag: %s\n  Pre-release: %v\n  Created: %s\n  URL: %s\n" .name .tagName .isPrerelease .createdAt .url}}'
        exit 1
    fi

    print_success "GitHub Release $VERSION does not exist (OK)"
}

validate_branch_rc0() {
    if [[ "$RC" != "rc0" ]]; then
        return
    fi

    print_section "🔍 Phase 5: Checking release branch (RC0 mode)..."

    if git ls-remote --heads $UPSTREAM_REMOTE "$BRANCH" 2>/dev/null | grep -q "$BRANCH"; then
        print_error "Branch $BRANCH already exists!"
        echo "   RC0 should create a new release branch."
        echo ""
        echo "If you want to create RC1+, use version like: v${BASE_VERSION}-rc1"
        exit 1
    fi

    print_success "Branch $BRANCH does not exist (OK for RC0)"
}

validate_branch_rc1_plus() {
    if [[ "$RC" == "rc0" ]]; then
        return
    fi

    print_section "🔍 Phase 6: Checking release branch (RC1+/Final mode)..."

    if ! git ls-remote --heads $UPSTREAM_REMOTE "$BRANCH" 2>/dev/null | grep -q "$BRANCH"; then
        print_error "Branch $BRANCH does not exist!"
        echo "   RC1+ and Final releases require an existing release branch."
        echo ""
        echo "Please create RC0 first: v${BASE_VERSION}-rc0"
        exit 1
    fi

    print_success "Branch $BRANCH exists (OK for RC1+/Final)"
}

# ============================================================
# Execution functions
# ============================================================

show_dry_run_plan() {
    echo ""
    echo "=================================================="
    echo -e "${BLUE}🔍 DRY-RUN MODE - No changes will be made${NC}"
    echo "=================================================="
    echo ""
    echo "📋 Release Information:"
    echo "  Version:      $VERSION"
    echo "  Type:         $RELEASE_TYPE"
    echo "  Branch:       $BRANCH"
    if [[ "$RC" == "rc0" ]]; then
        echo "  Base:         master"
    else
        echo "  Base:         $BRANCH (existing)"
    fi
    echo ""
    echo "🔨 Actions to be performed by this script:"
    if [[ "$RC" == "rc0" ]]; then
        echo "  1. Create release branch: $BRANCH (from master)"
        echo "  2. Push branch to remote"
        echo "  3. Create tag: $VERSION"
        echo "  4. Push tag to remote"
    else
        echo "  1. Checkout existing branch: $BRANCH"
        echo "  2. Create tag: $VERSION"
        echo "  3. Push tag to remote"
    fi
    echo ""
    echo "🔨 Additional actions (performed by GitHub Actions):"
    if [[ -n "$RC" ]]; then
        echo "  - Create GitHub pre-release: $VERSION"
    else
        echo "  - Create GitHub final release: $VERSION"
    fi
    echo ""
    echo "=================================================="
    print_success "All validations passed!"
    echo "=================================================="
    echo ""
    echo "🚀 To execute this release, run:"
    echo "   ./hack/create-release.sh $VERSION"
    echo ""
}

handle_error() {
    echo ""
    echo "=================================================="
    print_error "Error occurred! Rolling back..."
    echo "=================================================="

    # Delete tag if created
    if git rev-parse "$VERSION" >/dev/null 2>&1; then
        echo "Deleting local tag $VERSION..."
        git tag -d "$VERSION" 2>/dev/null || true
    fi

    if git ls-remote --tags $UPSTREAM_REMOTE 2>/dev/null | grep -q "refs/tags/$VERSION"; then
        echo "Deleting remote tag $VERSION..."
        git push --delete $UPSTREAM_REMOTE "$VERSION" 2>/dev/null || true
    fi

    # Delete branch if created (RC0 only)
    if [[ "$RC" == "rc0" ]]; then
        if git ls-remote --heads $UPSTREAM_REMOTE "$BRANCH" 2>/dev/null | grep -q "$BRANCH"; then
            echo "Deleting remote branch $BRANCH..."
            git push --delete $UPSTREAM_REMOTE "$BRANCH" 2>/dev/null || true
        fi
    fi

    echo "=================================================="
    print_error "Rollback completed"
    echo "=================================================="
    exit 1
}

create_rc0() {
    print_section "Step 1: Creating release branch from master..."

    git checkout master
    git pull $UPSTREAM_REMOTE master
    git checkout -b "$BRANCH"
    git push $UPSTREAM_REMOTE "$BRANCH"

    print_success "Created and pushed branch: $BRANCH"
}

create_rc1_plus_or_final() {
    print_section "Step 1: Checking out existing release branch..."

    git fetch $UPSTREAM_REMOTE "$BRANCH"
    git checkout "$BRANCH"
    git pull $UPSTREAM_REMOTE "$BRANCH"

    print_success "Checked out branch: $BRANCH"
}

create_tag() {
    print_section "Step 2: Creating tag..."

    git tag "$VERSION"
    git push $UPSTREAM_REMOTE "$VERSION"

    print_success "Created and pushed tag: $VERSION"
}


execute_release() {
    echo ""
    echo "=================================================="
    print_info "🚀 EXECUTION MODE - Creating release..."
    echo "=================================================="
    echo ""

    # Setup error handler
    trap 'handle_error' ERR

    # Configure git
    if ! git config user.email >/dev/null 2>&1; then
        git config user.email "github-actions[bot]@users.noreply.github.com"
        git config user.name "github-actions[bot]"
    fi

    # Execute based on release type
    if [[ "$RC" == "rc0" ]]; then
        create_rc0
    else
        create_rc1_plus_or_final
    fi

    create_tag

    echo ""
    echo "=================================================="
    print_success "Branch and tag created successfully!"
    echo "=================================================="
    echo ""
    echo "Branch: $BRANCH"
    echo "Tag: $VERSION"
    echo ""
    echo "Next steps:"
    echo "  ✅ Branch and tag created successfully"
    echo ""
    echo "  Next: Create GitHub Release (requires project lead)"
    echo "  URL: https://github.com/kserve/kserve/releases/new?tag=$VERSION"
    echo ""
}

# ============================================================
# Main script
# ============================================================

main() {
    # Check if running from repository root
    if [[ ! -f "kserve-deps.env" ]]; then
        print_error "This script must be run from the repository root directory"
        exit 1
    fi

    # Parse arguments
    if [[ $# -eq 0 ]] || [[ "$1" == "-h" ]] || [[ "$1" == "--help" ]]; then
        usage
    fi

    VERSION="$1"
    shift

    while [[ $# -gt 0 ]]; do
        case $1 in
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            --validate-only)
                VALIDATE_ONLY=true
                shift
                ;;
            --github-actions)
                GITHUB_ACTIONS_MODE=true
                shift
                ;;
            -h|--help)
                usage
                ;;
            *)
                print_error "Unknown option: $1"
                usage
                ;;
        esac
    done

    # Validate option combinations
    if [[ "$DRY_RUN" == true ]] && [[ "$VALIDATE_ONLY" == true ]]; then
        print_error "Cannot use --dry-run and --validate-only together"
        echo ""
        echo "Choose one:"
        echo "  --validate-only : Only run validations"
        echo "  --dry-run       : Run validations and show execution plan"
        exit 1
    fi

    # Print header
    echo "=================================================="
    echo "KServe Release Automation"
    echo "=================================================="
    echo "Version: $VERSION"
    echo "Dry-run: $DRY_RUN"
    echo "Validate-only: $VALIDATE_ONLY"
    echo "GitHub Actions Mode: $GITHUB_ACTIONS_MODE"
    echo "=================================================="

    # Determine upstream remote
    if [[ "$GITHUB_ACTIONS_MODE" == true ]]; then
        # GitHub Actions always uses origin
        UPSTREAM_REMOTE="origin"
        echo "Using remote: $UPSTREAM_REMOTE (GitHub Actions)"
    else
        # Auto-detect for local/fork usage
        UPSTREAM_REMOTE=$(detect_upstream_remote)

        if [[ -z "$UPSTREAM_REMOTE" ]]; then
            echo ""
            print_error "No remote pointing to kserve/kserve found!"
            echo ""
            echo "Please add upstream remote:"
            echo "  git remote add upstream git@github.com:kserve/kserve.git"
            echo ""
            echo "Or if using HTTPS:"
            echo "  git remote add upstream https://github.com/kserve/kserve.git"
            echo ""
            echo "Or run with --github-actions flag to use origin remote"
            exit 1
        fi

        echo "Using remote: $UPSTREAM_REMOTE (auto-detected)"
    fi
    echo "=================================================="

    # Run validations
    validate_version_format
    parse_version
    validate_kserve_deps
    validate_tag_duplicate
    validate_github_release_duplicate
    validate_branch_rc0
    validate_branch_rc1_plus

    # Exit if validate-only mode
    if [[ "$VALIDATE_ONLY" == true ]]; then
        echo ""
        print_success "All validations passed!"
        exit 0
    fi

    # Show dry-run plan and exit if dry-run mode
    if [[ "$DRY_RUN" == true ]]; then
        show_dry_run_plan
        exit 0
    fi

    # Block local execution - only allow in GitHub Actions
    if [[ "${GITHUB_ACTIONS:-false}" != "true" ]]; then
        echo ""
        echo "=================================================="
        print_error "Direct execution is not allowed!"
        echo "=================================================="
        echo ""
        echo "This script can only be executed in GitHub Actions"
        echo "to prevent accidental releases."
        echo ""
        echo "To create a release:"
        echo "  1. Go to: https://github.com/kserve/kserve/actions"
        echo "  2. Select 'Create Release' workflow"
        echo "  3. Click 'Run workflow'"
        echo "  4. Enter version and set dry_run=false"
        echo ""
        echo "For local testing:"
        echo "  ./hack/create-release.sh <version> --validate-only"
        echo "  ./hack/create-release.sh <version> --dry-run"
        echo ""
        exit 1
    fi

    # Execute release
    execute_release
}

main "$@"

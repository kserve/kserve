#!/bin/bash

# Script version and metadata
SCRIPT_NAME="$(basename "$0")"

# Default values
NAMESPACE=""
SELECTED_ISVCS=()
PRESERVE_FILES="" 
DRY_RUN="false"
DELETE_EXISTING="false"
USE_ORIGINAL_NAMES="false"

# Color codes for better output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Function to display help
show_help() {
    cat << EOF
KServe InferenceService Raw Deployment Converter

DESCRIPTION:
    Converts KServe InferenceServices from Serverless deployment mode to Raw deployment mode.
    This script automates the process of migrating models from Knative-based serverless 
    deployments to standard Kubernetes deployments for better control and resource management.
    
    NOTE: In normal mode (not --dry-run), auth resources are created with ownerReferences
    pointing to the InferenceService for automatic garbage collection. In --dry-run mode,
    ownerReferences are omitted from saved files to ensure they can be applied independently.
    Original files saved for rollback also have ownerReferences removed for portability.
    
    If you applied files from --dry-run and want to add ownerReferences later, use the
    companion script: ./add-owner-references.sh -n <namespace> -i <inferenceservice-name>

USAGE:
    $SCRIPT_NAME [OPTIONS]

OPTIONS:
    -n, --namespace NAMESPACE   Target namespace containing InferenceServices
                               If not specified, uses current OpenShift context namespace
    
    --dry-run                  Generate transformation files without applying to cluster
                               Files are always preserved when using this option
    
    --delete-existing          Delete existing InferenceServices and related resources
                               after successful conversion (cannot be used with --dry-run)
                               With -raw suffix: deletes originals AFTER new resources are ready
                               With original names: deletes originals BEFORE applying new ones
    
    -h, --help                 Show this help message and exit

EXAMPLES:
    # Convert InferenceServices in current namespace
    $SCRIPT_NAME
    
    # Convert InferenceServices in specific namespace
    $SCRIPT_NAME --namespace my-models
    $SCRIPT_NAME -n my-models
    
    # Generate files without applying to cluster
    $SCRIPT_NAME --dry-run
    $SCRIPT_NAME --dry-run -n my-models
    
    # Convert and delete existing resources after successful conversion
    $SCRIPT_NAME --delete-existing
    $SCRIPT_NAME --delete-existing -n my-models

WHAT THIS SCRIPT DOES:
    1. Discovers InferenceServices with 'Serverless' deployment mode
    2. Allows interactive selection of which models to convert
    3. Prompts user to choose naming convention (original names vs -raw suffix)
    4. For each selected InferenceService:
       • Exports original InferenceService and ServingRuntime configurations
       • Creates raw deployment versions with chosen naming convention
       • Handles authentication resources (ServiceAccount, Role, RoleBinding, Secret)
       • Applies all transformed resources to the cluster (unless --dry-run is used)
       • Optionally deletes existing resources after successful conversion
    5. Optionally preserves exported files for review

PREREQUISITES:
    • OpenShift CLI (oc) - logged into target cluster
    • yq (YAML processor) - for YAML manipulation
    • jq (JSON processor) - for JSON manipulation
    • Appropriate RBAC permissions in target namespace (not needed for --dry-run)

AUTHENTICATION SUPPORT:
    The script automatically detects and migrates authentication resources when the
    'security.opendatahub.io/enable-auth' annotation is set to 'true' on the
    original InferenceService.

FILE ORGANIZATION:
    When preserving files, they are organized as:
    <inference-service-name>/
    ├── original/              # Original exported resources
    ├── raw/                   # Transformed resources for raw deployment (with -raw suffix)
    └── raw-original-names/    # Transformed resources with original names (for in-place replacement)

EXIT CODES:
    0    Success
    1    Error (missing dependencies, permissions, validation failure, etc.)

MORE INFORMATION:
    For more details about KServe deployment modes, visit:
    https://kserve.github.io/website/latest/admin/raw-deployment/
EOF
}

# Function for colored output
log_info() {
    echo -e "${GREEN}✅${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}⚠️${NC} $1"
}

log_error() {
    echo -e "${RED}❌${NC} $1" >&2
}

log_step() {
    echo -e "${BLUE}🔄${NC} $1"
}

# Function to get current namespace
get_current_namespace() {
    local current_ns
    
    # Try to get the current namespace from oc context
    current_ns=$(oc config view --minify -o jsonpath='{..namespace}' 2>/dev/null)
    
    # If not set in context, try the project command
    if [ -z "$current_ns" ]; then
        current_ns=$(oc project -q 2>/dev/null)
    fi
    
    # If still empty, default to 'default'
    if [ -z "$current_ns" ]; then
        current_ns="default"
    fi
    
    echo "$current_ns"
}

# Parse command line arguments
parse_arguments() {
    # if [ $# -eq 0 ]; then
    #     echo -e "${YELLOW}No arguments provided${NC}"
    #     show_help
    #     exit 0
    # fi
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            --namespace=*)
                NAMESPACE="${1#*=}"
                shift
                ;;
            -n|--namespace)
                if [ -n "$2" ] && [[ $2 != --* ]]; then
                    NAMESPACE="$2"
                    shift 2
                else
                    log_error "Option $1 requires a value"
                    exit 1
                fi
                ;;
            --dry-run)
                DRY_RUN="true"
                shift
                ;;
            --delete-existing)
                DELETE_EXISTING="true"
                shift
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                echo -e "${YELLOW}Use --help for usage information${NC}"
                exit 1
                ;;
        esac
    done
}

# Function to check prerequisites
check_prerequisites() {
    log_step "Checking prerequisites..."
    
    local missing_deps=()
    
    # Check for oc command
    if ! command -v oc &> /dev/null; then
        missing_deps+=("oc (OpenShift CLI)")
    fi
    
    # Check for yq command
    if ! command -v yq &> /dev/null; then
        missing_deps+=("yq (YAML processor)")
    fi
    
    # Check for jq command
    if ! command -v jq &> /dev/null; then
        missing_deps+=("jq (JSON processor)")
    fi
    
    if [ ${#missing_deps[@]} -ne 0 ]; then
        log_error "Missing required dependencies:"
        for dep in "${missing_deps[@]}"; do
            echo -e "  ${RED}•${NC} $dep"
        done
        echo ""
        echo -e "${YELLOW}Installation instructions:${NC}"
        echo -e "  ${BLUE}oc:${NC} https://docs.openshift.com/container-platform/latest/cli_reference/openshift_cli/getting-started-cli.html"
        echo -e "  ${BLUE}yq:${NC} https://github.com/mikefarah/yq#install"
        echo -e "  ${BLUE}jq:${NC} https://jqlang.github.io/jq/download/"
        exit 1
    fi
    
    # Check if oc is logged in
    if ! oc whoami &> /dev/null; then
        log_error "Not logged into OpenShift cluster"
        echo -e "${YELLOW}Please login first:${NC} oc login <cluster-url>"
        exit 1
    fi
    
    log_info "All prerequisites satisfied"
    log_info "Logged in as: $(oc whoami)"
    log_info "Current context: $(oc config current-context 2>/dev/null || echo 'unknown')"
}

# Function to validate namespace
validate_namespace() {
    local errors=()

    # If namespace not provided, get current namespace
    if [ -z "$NAMESPACE" ]; then
        NAMESPACE=$(get_current_namespace)
        if [ -z "$NAMESPACE" ]; then
            errors+=("Could not determine current namespace and none was provided")
        fi
    fi

    # Check if namespace exists (only if NAMESPACE is set)
    if [ -n "$NAMESPACE" ] && ! oc get namespace "$NAMESPACE" &> /dev/null; then
        errors+=("Namespace '$NAMESPACE' does not exist.")
    fi

    # Handle validation errors
    if [ ${#errors[@]} -ne 0 ]; then
        log_error "Namespace validation failed:"
        for error in "${errors[@]}"; do
            echo -e "  ${RED}•${NC} $error"
        done
        echo ""
        echo -e "${YELLOW}Use --help for usage information${NC}"
        exit 1
    fi

    log_info "Namespace validation successful"
}

# Function to validate arguments
validate_arguments() {
    local errors=()

    # Check for conflicting options
    if [ "$DRY_RUN" == "true" ] && [ "$DELETE_EXISTING" == "true" ]; then
        errors+=("Cannot use --delete-existing with --dry-run")
    fi

    # Handle validation errors
    if [ ${#errors[@]} -ne 0 ]; then
        log_error "Argument validation failed:"
        for error in "${errors[@]}"; do
            echo -e "  ${RED}•${NC} $error"
        done
        echo ""
        echo -e "${YELLOW}Use --help for usage information${NC}"
        exit 1
    fi

    log_info "Argument validation successful"
}


# Function to check permissions
check_permissions() {
    log_step "Checking permissions..."
    
    local permission_checks=(
        "get:inferenceservices"
        "create:inferenceservices" 
        "patch:inferenceservices"
        "get:servingruntimes"
        "create:servingruntimes"
        "patch:servingruntimes"
        "get:serviceaccounts"
        "create:serviceaccounts"
        "patch:serviceaccounts"
        "get:roles"
        "create:roles"
        "patch:roles"
        "get:rolebindings"
        "create:rolebindings"
        "patch:rolebindings"
        "get:secrets"
        "create:secrets"
        "patch:secrets"
    )
    
    local failed_checks=()
    
    for check in "${permission_checks[@]}"; do
        IFS=':' read -r verb resource <<< "$check"
        if ! oc auth can-i "$verb" "$resource" -n "$NAMESPACE" &> /dev/null; then
            failed_checks+=("$verb $resource")
        fi
    done
    
    if [ ${#failed_checks[@]} -ne 0 ]; then
        log_error "Insufficient permissions in namespace '$NAMESPACE':"
        for failed in "${failed_checks[@]}"; do
            echo -e "  ${RED}•${NC} Cannot $failed"
        done
        echo ""
        echo -e "${YELLOW}Please contact your cluster administrator to grant the required permissions${NC}"
        exit 1
    fi
    
    log_info "Permission check successful"
}

# List InferenceServices and get user selection
list_and_select_inference_services() {
    echo "🔍 Discovering InferenceServices in source namespace '$NAMESPACE'..."

    # Get all InferenceServices in the source namespace
    local isvc_list=$(oc get inferenceservice -n "$NAMESPACE" -o yaml 2>/dev/null)

    if [[ $? -ne 0 ]]; then
        log_error "Failed to retrieve InferenceServices from namespace '$NAMESPACE'"
        echo "Please ensure you have access to the namespace and InferenceServices exist."
        exit 1
    fi

    # Check if any InferenceServices exist
    local isvc_count=$(echo "$isvc_list" | yq '.items | length')

    if [[ "$isvc_count" -eq 0 ]]; then
        log_error "No InferenceServices found in namespace '$NAMESPACE'"
        echo "There are no models to migrate."
        exit 1
    fi

    # Get names of InferenceServices that are eligible (Serverless deployment mode)
    local isvc_names=()
    while IFS= read -r name; do
        if [[ -n "$name" && "$name" != "null" ]]; then
            # Check if this InferenceService has Serverless deployment mode
            local deployment_mode=$(echo "$isvc_list" | yq ".items[] | select(.metadata.name == \"$name\") | .metadata.annotations.\"serving.kserve.io/deploymentMode\" // \"\"")
            if [[ "$deployment_mode" == "Serverless" ]]; then
                isvc_names+=("$name")
            fi
        fi
    done < <(echo "$isvc_list" | yq '.items[].metadata.name' 2>/dev/null)
    
    local filtered_count=${#isvc_names[@]}
    
    if [[ "$filtered_count" -eq 0 ]]; then
        log_error "No InferenceServices found with deploymentMode set to 'Serverless' in namespace '$NAMESPACE'"
        echo "Found $isvc_count total InferenceService(s), but none are eligible for conversion."
        echo "Only InferenceServices with deploymentMode annotation set to 'Serverless' can be converted to raw deployment."
        exit 1
    fi

    log_info "Found $filtered_count eligible InferenceService(s) out of $isvc_count total in namespace '$NAMESPACE'"
    echo ""
    echo "📦 Available InferenceServices (Serverless deployment mode only):"
    echo "=================================================================="

    # List each InferenceService with index numbers
    local index=1
    for isvc_name in "${isvc_names[@]}"; do
        # Get deployment mode for display
        local deployment_mode=$(echo "$isvc_list" | yq ".items[] | select(.metadata.name == \"$isvc_name\") | .metadata.annotations.\"serving.kserve.io/deploymentMode\" // \"Serverless (default)\"")
        echo "[$index] $isvc_name (Mode: $deployment_mode)"
        echo ""
        ((index++))
    done

    echo ""
    echo "🤔 Please select which InferenceServices to migrate:"
    echo "=================================================="
    if [ "$DRY_RUN" == "true" ]; then
        echo -e "${YELLOW}📝 DRY-RUN MODE: Files will be generated but NOT applied to the cluster${NC}"
        echo ""
    fi
    echo "Enter 'all' to migrate all InferenceServices"
    echo "Enter specific numbers separated by spaces (e.g., '1 3 5')"
    echo "Enter 'q' to quit"
    echo ""
    read -p "Your selection: " selection

    # Handle user selection
    case "$selection" in
        "q"|"Q")
            echo "👋 Migration cancelled by user"
            exit 0
            ;;
        "all"|"ALL")
            log_info "Selected all $filtered_count InferenceService(s) for migration"
            SELECTED_ISVCS=("${isvc_names[@]}")
            ;;
        *)
            # Parse specific selections
            local selected_indices=($selection)
            SELECTED_ISVCS=()

            for idx in "${selected_indices[@]}"; do
                # Validate index is a number
                if ! [[ "$idx" =~ ^[0-9]+$ ]]; then
                    log_error "'$idx' is not a valid number"
                    exit 1
                fi

                # Convert to 0-based index and validate range
                local array_idx=$((idx - 1))
                if [[ $array_idx -lt 0 || $array_idx -ge ${#isvc_names[@]} ]]; then
                    log_error "Index '$idx' is out of range (1-${#isvc_names[@]})"
                    exit 1
                fi

                # Add to selected list
                SELECTED_ISVCS+=("${isvc_names[$array_idx]}")
            done

            if [[ ${#SELECTED_ISVCS[@]} -eq 0 ]]; then
                log_error "No valid InferenceServices selected"
                exit 1
            fi

            log_info "Selected ${#SELECTED_ISVCS[@]} InferenceService(s) for migration:"
            for isvc in "${SELECTED_ISVCS[@]}"; do
                echo "  • $isvc"
            done
            ;;
    esac

    echo ""
}

collect_preserve_file_response() {
    # In dry-run mode, always preserve files
    if [ "$DRY_RUN" == "true" ]; then
        PRESERVE_FILES="true"
        log_info "Running in dry-run mode - files will be preserved automatically"
        echo -e "Files will be organized as follows:\n"
        
        if [ "$USE_ORIGINAL_NAMES" == "true" ]; then
            # Show only original and raw-original-names directories
            cat <<EOF
  <inference-service-name>/
  ├── original/
  │   ├── <name>-isvc.yaml              # Original InferenceService (YAML)
  │   ├── <runtime>-sr.yaml             # Original ServingRuntime (YAML)
  │   ├── <name>-sa.yaml                # Original ServiceAccount (if auth enabled)
  │   ├── <name>-view-role.yaml         # Original Role (if auth enabled)
  │   ├── <name>-view-rolebinding.yaml  # Original RoleBinding (if auth enabled)
  │   └── <name>-secret.yaml            # Original Secret (if auth enabled)
  └── raw-original-names/
      ├── <name>-isvc.yaml              # Transformed InferenceService with original name
      ├── <runtime>-sr.yaml             # Transformed ServingRuntime with original name
      ├── <name>-sa.yaml                # Transformed ServiceAccount with original name (if auth enabled)
      ├── <name>-view-role.yaml         # Transformed Role with original name (if auth enabled)
      ├── <name>-view-rolebinding.yaml  # Transformed RoleBinding with original name (if auth enabled)
      └── <name>-secret.yaml            # Transformed Secret with original name (if auth enabled)
EOF
        else
            # Show only original and raw directories
            cat <<EOF
  <inference-service-name>/
  ├── original/
  │   ├── <name>-isvc.yaml              # Original InferenceService (YAML)
  │   ├── <runtime>-sr.yaml             # Original ServingRuntime (YAML)
  │   ├── <name>-sa.yaml                # Original ServiceAccount (if auth enabled)
  │   ├── <name>-view-role.yaml         # Original Role (if auth enabled)
  │   ├── <name>-view-rolebinding.yaml  # Original RoleBinding (if auth enabled)
  │   └── <name>-secret.yaml            # Original Secret (if auth enabled)
  └── raw/
      ├── <name>-raw-isvc.yaml          # Transformed InferenceService (renamed with -raw)
      ├── <runtime>-raw-sr.yaml         # Transformed ServingRuntime (renamed with -raw)
      ├── <name>-raw-sa.yaml            # Transformed ServiceAccount (renamed with -raw, if auth enabled)
      ├── <name>-raw-view-role.yaml     # Transformed Role (renamed with -raw, if auth enabled)
      ├── <name>-raw-view-rolebinding.yaml # Transformed RoleBinding (renamed with -raw, if auth enabled)
      └── <name>-raw-secret.yaml        # Transformed Secret (renamed with -raw, if auth enabled)
EOF
        fi
        echo ""
        return 0
    fi

    echo -e "${YELLOW}After conversion, do you want to preserve the exported and transformed files?${NC}"
    echo -e "If you choose to keep them, they'll be organized as follows:\n"

    if [ "$USE_ORIGINAL_NAMES" == "true" ]; then
        # Show only original and raw-original-names directories
        cat <<EOF
  <inference-service-name>/
  ├── original/
  │   ├── <name>-isvc.yaml              # Original InferenceService (YAML)
  │   ├── <runtime>-sr.yaml             # Original ServingRuntime (YAML)
  │   ├── <name>-sa.yaml                # Original ServiceAccount (if auth enabled)
  │   ├── <name>-view-role.yaml         # Original Role (if auth enabled)
  │   ├── <name>-view-rolebinding.yaml  # Original RoleBinding (if auth enabled)
  │   └── <name>-secret.yaml            # Original Secret (if auth enabled)
  └── raw-original-names/
      ├── <name>-isvc.yaml              # Transformed InferenceService with original name
      ├── <runtime>-sr.yaml             # Transformed ServingRuntime with original name
      ├── <name>-sa.yaml                # Transformed ServiceAccount with original name (if auth enabled)
      ├── <name>-view-role.yaml         # Transformed Role with original name (if auth enabled)
      ├── <name>-view-rolebinding.yaml  # Transformed RoleBinding with original name (if auth enabled)
      └── <name>-secret.yaml            # Transformed Secret with original name (if auth enabled)
EOF
    else
        # Show only original and raw directories
        cat <<EOF
  <inference-service-name>/
  ├── original/
  │   ├── <name>-isvc.yaml              # Original InferenceService (YAML)
  │   ├── <runtime>-sr.yaml             # Original ServingRuntime (YAML)
  │   ├── <name>-sa.yaml                # Original ServiceAccount (if auth enabled)
  │   ├── <name>-view-role.yaml         # Original Role (if auth enabled)
  │   ├── <name>-view-rolebinding.yaml  # Original RoleBinding (if auth enabled)
  │   └── <name>-secret.yaml            # Original Secret (if auth enabled)
  └── raw/
      ├── <name>-raw-isvc.yaml          # Transformed InferenceService (renamed with -raw)
      ├── <runtime>-raw-sr.yaml         # Transformed ServingRuntime (renamed with -raw)
      ├── <name>-raw-sa.yaml            # Transformed ServiceAccount (renamed with -raw, if auth enabled)
      ├── <name>-raw-view-role.yaml     # Transformed Role (renamed with -raw, if auth enabled)
      ├── <name>-raw-view-rolebinding.yaml # Transformed RoleBinding (renamed with -raw, if auth enabled)
      └── <name>-raw-secret.yaml        # Transformed Secret (renamed with -raw, if auth enabled)
EOF
    fi
    echo ""

    local preserve_response=""
    if ! read -t 30 -p "Preserve files? [y/N]: " preserve_response; then
        echo ""
        log_warn "No response received within 30 seconds. Defaulting to cleanup."
        preserve_response="n"
    fi

    preserve_response=$(echo "$preserve_response" | tr '[:upper:]' '[:lower:]')

    case "$preserve_response" in
        y|yes)
            PRESERVE_FILES="true"
            log_info "You chose to preserve the generated files."
            ;;
        n|no|"")
            PRESERVE_FILES="false"
            log_info "You chose not to preserve the files. Temporary resources will be cleaned up."
            ;;
        *)
            PRESERVE_FILES="false"
            log_warn "Invalid response '$preserve_response'. Cleaning up files by default."
            ;;
    esac
}

# Function to collect naming preference
collect_naming_preference() {
    echo ""
    echo "🏷️  Resource Naming Options:"
    echo "================================"
    echo ""
    echo "Choose how you want to name the converted resources:"
    echo ""
    echo "1) Use original names (for in-place replacement)"
    echo "   - Replaces existing resources with same names"
    echo "   - Example: 'my-model' stays 'my-model'"
    if [ "$DELETE_EXISTING" == "true" ]; then
        echo "   - Original resources will be deleted BEFORE applying new ones"
    fi
    echo ""
    echo "2) Use -raw suffix (for side-by-side deployment)"
    echo "   - Creates new resources alongside existing ones"
    echo "   - Example: 'my-model' becomes 'my-model-raw'"
    if [ "$DELETE_EXISTING" == "true" ]; then
        echo "   - Original resources will be deleted AFTER successful deployment"
    fi
    echo ""
    
    local naming_choice=""
    while true; do
        read -p "Enter your choice (1 or 2): " naming_choice
        case "$naming_choice" in
            1)
                USE_ORIGINAL_NAMES="true"
                
                # In dry-run mode, skip confirmation and just note the selection
                if [ "$DRY_RUN" == "true" ]; then
                    log_info "Selected: Use original names (in-place replacement)"
                    echo ""
                    break
                fi
                
                # Not in dry-run mode - show warning and ask for confirmation
                echo ""
                echo -e "${RED}⚠️  WARNING: IN-PLACE REPLACEMENT MODE${NC}"
                echo -e "${RED}=================================${NC}"
                echo ""
                echo -e "${YELLOW}You have chosen to use original names. This means:${NC}"
                echo ""
                echo -e "${RED}• The converted resources will REPLACE the existing ones${NC}"
                if [ "$DELETE_EXISTING" != "true" ]; then
                    echo -e "${RED}• You MUST manually delete all existing resources before applying${NC}"
                    echo -e "${RED}• There is NO TURNING BACK once the original resources are deleted${NC}"
                else
                    echo -e "${RED}• The script will automatically delete existing resources before applying${NC}"
                fi
                echo -e "${RED}• If conversion fails, you may lose your original configuration${NC}"
                echo ""
                echo -e "${YELLOW}Recommendations:${NC}"
                echo "• Use --dry-run first to test the conversion"
                if [ "$DELETE_EXISTING" != "true" ]; then
                    echo "• Consider using --delete-existing flag to automate cleanup"
                fi
                echo ""
                
                local confirm_choice=""
                while true; do
                    read -p "Are you sure you want to proceed with in-place replacement? (yes/no): " confirm_choice
                    case "$confirm_choice" in
                        yes|YES)
                            log_info "Selected: Use original names (in-place replacement) - CONFIRMED"
                            break 2
                            ;;
                        no|NO)
                            echo ""
                            echo -e "${GREEN}Returning to naming options...${NC}"
                            echo ""
                            break
                            ;;
                        *)
                            echo -e "${RED}Please enter 'yes' or 'no'${NC}"
                            ;;
                    esac
                done
                ;;
            2)
                USE_ORIGINAL_NAMES="false"
                log_info "Selected: Use -raw suffix (side-by-side deployment)"
                break
                ;;
            *)
                echo -e "${RED}Invalid choice. Please enter 1 or 2.${NC}"
                ;;
        esac
    done
    echo ""
}

# Function to delete existing resources
delete_existing_resources() {
    local name="$1"
    
    log_step "Deleting existing resources for InferenceService: $name"
    
    # Get the UID of the original InferenceService for finding owned resources BEFORE deleting it
    local original_isvc_uid=$(oc get isvc -n "$NAMESPACE" "$name" -o yaml 2>/dev/null | yq eval '.metadata.uid // ""' -)
    
    # Get the ServingRuntime name from the original InferenceService BEFORE deleting it
    local original_runtime_name=$(oc get isvc -n "$NAMESPACE" "$name" -o yaml 2>/dev/null | yq eval '.spec.predictor.model.runtime // ""' -)
    
    # Delete InferenceService
    if oc get isvc -n "$NAMESPACE" "$name" &>/dev/null; then
        log_info "Deleting InferenceService: $name"
        oc delete isvc -n "$NAMESPACE" "$name" --ignore-not-found=true
    fi
    
    # Delete the original ServingRuntime if it exists
    if [ -n "$original_runtime_name" ] && [ "$original_runtime_name" != "null" ]; then
        if oc get servingruntimes -n "$NAMESPACE" "$original_runtime_name" &>/dev/null; then
            log_info "Deleting ServingRuntime: $original_runtime_name"
            oc delete servingruntimes -n "$NAMESPACE" "$original_runtime_name" --ignore-not-found=true
        else
            log_info "ServingRuntime $original_runtime_name not found (may be shared or already deleted)"
        fi
    fi
    
    if [ -n "$original_isvc_uid" ] && [ "$original_isvc_uid" != "null" ]; then
        # Find and delete resources - try ownerReferences first, then fall back to naming convention
        log_info "Looking for resources associated with InferenceService $name..."
        
        # Delete ServiceAccount - try by ownerReference first
        local sa_name=$(oc get serviceaccounts -n "$NAMESPACE" -o json 2>/dev/null | jq -r ".items[] | select(.metadata.ownerReferences[]?.uid == \"$original_isvc_uid\") | .metadata.name" 2>/dev/null)
        if [ -z "$sa_name" ] || [ "$sa_name" == "null" ]; then
            # Fallback to naming convention
            sa_name="${name}-sa"
            log_info "No ServiceAccount found by ownerReference, trying naming convention: $sa_name"
        else
            log_info "Found ServiceAccount by ownerReference: $sa_name"
        fi
        
        if oc get serviceaccount "$sa_name" -n "$NAMESPACE" &>/dev/null; then
            log_info "Deleting ServiceAccount: $sa_name"
            oc delete serviceaccount -n "$NAMESPACE" "$sa_name" --ignore-not-found=true
        fi
        
        # Delete Role - try by ownerReference first
        local role_name=$(oc get roles -n "$NAMESPACE" -o json 2>/dev/null | jq -r ".items[] | select(.metadata.ownerReferences[]?.uid == \"$original_isvc_uid\") | .metadata.name" 2>/dev/null)
        if [ -z "$role_name" ] || [ "$role_name" == "null" ]; then
            # Fallback to naming convention
            role_name="${name}-view-role"
            log_info "No Role found by ownerReference, trying naming convention: $role_name"
        else
            log_info "Found Role by ownerReference: $role_name"
        fi
        
        if oc get role "$role_name" -n "$NAMESPACE" &>/dev/null; then
            log_info "Deleting Role: $role_name"
            oc delete role -n "$NAMESPACE" "$role_name" --ignore-not-found=true
        fi
        
        # Delete RoleBinding - try by ownerReference first
        local rolebinding_name=$(oc get rolebindings -n "$NAMESPACE" -o json 2>/dev/null | jq -r ".items[] | select(.metadata.ownerReferences[]?.uid == \"$original_isvc_uid\") | .metadata.name" 2>/dev/null)
        if [ -z "$rolebinding_name" ] || [ "$rolebinding_name" == "null" ]; then
            # Fallback to naming convention
            rolebinding_name="${name}-view"
            log_info "No RoleBinding found by ownerReference, trying naming convention: $rolebinding_name"
        else
            log_info "Found RoleBinding by ownerReference: $rolebinding_name"
        fi
        
        if oc get rolebinding "$rolebinding_name" -n "$NAMESPACE" &>/dev/null; then
            log_info "Deleting RoleBinding: $rolebinding_name"
            oc delete rolebinding -n "$NAMESPACE" "$rolebinding_name" --ignore-not-found=true
        fi
        
        # Delete Secrets associated with the service account
        local secret_names=$(oc get secrets -n "$NAMESPACE" -o json 2>/dev/null | jq -r --arg sa_name "$sa_name" '.items[] | select(.metadata.annotations."kubernetes.io/service-account.name" == $sa_name) | .metadata.name')
        if [ -n "$secret_names" ]; then
            echo "$secret_names" | while read -r secret_name; do
                if [ -n "$secret_name" ]; then
                    log_info "Deleting Secret: $secret_name"
                    oc delete secret -n "$NAMESPACE" "$secret_name" --ignore-not-found=true
                fi
            done
        fi
    fi
    
    # Delete Route in istio-system namespace (created by Istio/Knative for the original InferenceService)
    local istio_route_name="${name}-${NAMESPACE}"
    if oc get route -n "istio-system" "$istio_route_name" &>/dev/null; then
        log_info "Deleting Route in istio-system: $istio_route_name"
        oc delete route -n "istio-system" "$istio_route_name" --ignore-not-found=true
    else
        log_info "No Route found in istio-system for $istio_route_name"
    fi
    
    # Verify all resources are deleted before proceeding
    log_step "Verifying deletion of resources for $name..."
    
    local max_wait=60  # Maximum wait time in seconds
    local wait_interval=2  # Check every 2 seconds
    local elapsed=0
    
    while [ $elapsed -lt $max_wait ]; do
        local all_deleted=true
        
        # Check if InferenceService still exists
        if oc get isvc -n "$NAMESPACE" "$name" &>/dev/null; then
            all_deleted=false
            log_info "Waiting for InferenceService $name to be deleted... ($elapsed/${max_wait}s)"
        fi
        
        # Check if Route in istio-system still exists
        local istio_route_name="${name}-${NAMESPACE}"
        if oc get route -n "istio-system" "$istio_route_name" &>/dev/null; then
            all_deleted=false
            log_info "Waiting for Route $istio_route_name in istio-system to be deleted... ($elapsed/${max_wait}s)"
        fi
        
        # Check if owned resources still exist (if we had the UID)
        if [ -n "$original_isvc_uid" ] && [ "$original_isvc_uid" != "null" ]; then
            # Check ServiceAccount
            local remaining_sa=$(oc get serviceaccounts -n "$NAMESPACE" -o yaml 2>/dev/null | yq eval ".items[] | select(.metadata.ownerReferences[]?.uid == \"$original_isvc_uid\") | .metadata.name" - 2>/dev/null)
            if [ -n "$remaining_sa" ] && [ "$remaining_sa" != "null" ]; then
                all_deleted=false
                log_info "Waiting for ServiceAccount $remaining_sa to be deleted... ($elapsed/${max_wait}s)"
            fi
            
            # Check Role
            local remaining_role=$(oc get roles -n "$NAMESPACE" -o yaml 2>/dev/null | yq eval ".items[] | select(.metadata.ownerReferences[]?.uid == \"$original_isvc_uid\") | .metadata.name" - 2>/dev/null)
            if [ -n "$remaining_role" ] && [ "$remaining_role" != "null" ]; then
                all_deleted=false
                log_info "Waiting for Role $remaining_role to be deleted... ($elapsed/${max_wait}s)"
            fi
            
            # Check RoleBinding
            local remaining_rb=$(oc get rolebindings -n "$NAMESPACE" -o yaml 2>/dev/null | yq eval ".items[] | select(.metadata.ownerReferences[]?.uid == \"$original_isvc_uid\") | .metadata.name" - 2>/dev/null)
            if [ -n "$remaining_rb" ] && [ "$remaining_rb" != "null" ]; then
                all_deleted=false
                log_info "Waiting for RoleBinding $remaining_rb to be deleted... ($elapsed/${max_wait}s)"
            fi
        fi
        
        # If all resources are deleted, break out of the loop
        if [ "$all_deleted" = true ]; then
            log_info "✅ All resources successfully deleted for $name"
            break
        fi
        
        # Wait before checking again
        sleep $wait_interval
        elapsed=$((elapsed + wait_interval))
    done
    
    # Final check - if we timed out, show what's still remaining
    if [ $elapsed -ge $max_wait ]; then
        log_warn "⚠️  Timeout waiting for resource deletion (${max_wait}s). Checking remaining resources..."
        
        # List any remaining resources
        if oc get isvc -n "$NAMESPACE" "$name" &>/dev/null; then
            log_warn "InferenceService $name still exists"
        fi
        
        # Check Route in istio-system
        local istio_route_name="${name}-${NAMESPACE}"
        if oc get route -n "istio-system" "$istio_route_name" &>/dev/null; then
            log_warn "Route $istio_route_name still exists in istio-system"
        fi
        
        if [ -n "$original_isvc_uid" ] && [ "$original_isvc_uid" != "null" ]; then
            local remaining_resources=$(oc get serviceaccounts,roles,rolebindings -n "$NAMESPACE" -o yaml 2>/dev/null | yq eval ".items[] | select(.metadata.ownerReferences[]?.uid == \"$original_isvc_uid\") | .kind + \"/\" + .metadata.name" - 2>/dev/null)
            if [ -n "$remaining_resources" ]; then
                log_warn "Remaining owned resources:"
                echo "$remaining_resources" | while read -r resource; do
                    if [ -n "$resource" ]; then
                        log_warn "  - $resource"
                    fi
                done
            fi
        fi
        
        echo ""
        local proceed_choice=""
        while true; do
            read -p "Some resources may still be deleting. Proceed with creation anyway? (yes/no): " proceed_choice
            case "$proceed_choice" in
                yes|YES)
                    log_warn "Proceeding with creation despite remaining resources..."
                    break
                    ;;
                no|NO)
                    log_error "User chose not to proceed. Exiting..."
                    exit 1
                    ;;
                *)
                    echo -e "${RED}Please enter 'yes' or 'no'${NC}"
                    ;;
            esac
        done
    fi
    
    log_info "Completed deletion verification for $name"
}

convert_isvc(){
    # Set up variables using the validated parameters
    NAME="$1"

    # Create resource directories - use temp directory if not preserving files
    if [ "$PRESERVE_FILES" == "true" ]; then
        RESOURCE_DIR="$NAME"
    else
        # Use a unique temporary directory to avoid conflicts with preserved files
        RESOURCE_DIR=".tmp-${NAME}-$$"
    fi
    
    ORIGINAL_DIR="$RESOURCE_DIR/original"
    
    if [ "$USE_ORIGINAL_NAMES" == "true" ]; then
        RAW_DIR="$RESOURCE_DIR/raw-original-names"
        if [ "$PRESERVE_FILES" == "true" ]; then
            log_step "Creating resource directories (original names mode)..."
        else
            log_step "Creating temporary resource directories (original names mode)..."
        fi
        mkdir -p "$ORIGINAL_DIR" "$RAW_DIR"
    else
        RAW_DIR="$RESOURCE_DIR/raw"
        if [ "$PRESERVE_FILES" == "true" ]; then
            log_step "Creating resource directories (-raw suffix mode)..."
        else
            log_step "Creating temporary resource directories (-raw suffix mode)..."
        fi
        mkdir -p "$ORIGINAL_DIR" "$RAW_DIR"
    fi
    
    # Define all variables at the top
    if [ "$USE_ORIGINAL_NAMES" == "true" ]; then
        export NAME_RAW="${NAME}"
        # Service Account names
        SA_NAME="${NAME}-sa"
        SA_NAME_RAW="${NAME}-sa"
        # Role names
        ROLE_NAME="${NAME}-view-role"
        ROLE_NAME_RAW="${NAME}-view-role"
        # RoleBinding names
        ROLEBINDING_NAME="${NAME}-view"
        ROLEBINDING_NAME_RAW="${NAME}-view"
    else
        export NAME_RAW="${NAME}-raw"
        # Service Account names
        SA_NAME="${NAME}-sa"
        SA_NAME_RAW="${NAME_RAW}-sa"
        # Role names
        ROLE_NAME="${NAME}-view-role"
        ROLE_NAME_RAW="${NAME_RAW}-view-role"
        # RoleBinding names
        ROLEBINDING_NAME="${NAME}-view"
        ROLEBINDING_NAME_RAW="${NAME_RAW}-view"
    fi
    
    # Knative route name
    KNATIVE_ROUTE_NAME="${NAME}-predictor"
    
    # File names for original resources (change ISVC extension to .yaml)
    ISVC_FILE="$ORIGINAL_DIR/${NAME}-isvc.yaml"
    SA_FILE="$ORIGINAL_DIR/${NAME}-sa.yaml"
    ROLE_FILE="$ORIGINAL_DIR/${NAME}-view-role.yaml"
    ROLEBINDING_FILE="$ORIGINAL_DIR/${NAME}-view-rolebinding.yaml"
    SECRET_FILE="$ORIGINAL_DIR/${NAME}-secret.yaml"
    
    # File names for raw resources
    ISVC_RAW_FILE="$RAW_DIR/${NAME_RAW}-isvc.yaml"
    SA_RAW_FILE="$RAW_DIR/${SA_NAME_RAW}.yaml"
    ROLE_RAW_FILE="$RAW_DIR/${ROLE_NAME_RAW}.yaml"
    ROLEBINDING_RAW_FILE="$RAW_DIR/${ROLEBINDING_NAME_RAW}.yaml"
    SECRET_RAW_FILE="$RAW_DIR/${NAME_RAW}-secret.yaml"
    
    # ServingRuntime variables (will be set after we get the runtime name)
    SERVINGRUNTIME_NAME=""
    SERVINGRUNTIME_NAME_RAW=""
    SERVINGRUNTIME_FILE=""
    SERVINGRUNTIME_RAW_FILE=""
    
    log_info "Processing InferenceService: $NAME in namespace: $NAMESPACE"
    
    # Cleanup function (always removes directories unless user chooses to keep)
    cleanup() {
        log_step "Cleaning up temporary directories and files..."
        rm -rf "$RESOURCE_DIR" 2>/dev/null
        log_info "Cleanup completed"
    }
    
    # Set up trap for cleanup on error only
    trap cleanup ERR
    
    # Step 1: Export as YAML and clean up metadata
    log_step "Step 1: Exporting InferenceService to $ISVC_FILE..."
    oc get isvc -n "$NAMESPACE" "$NAME" -o yaml | yq eval 'del(.metadata.finalizers, .metadata.resourceVersion, .metadata.uid, .status)' - > "$ISVC_FILE"
    if [ $? -ne 0 ] || [ ! -s "$ISVC_FILE" ]; then
        log_error "Failed to export InferenceService $NAME or file is empty"
        exit 1
    fi

    # Step 2: Transform YAML to YAML using yq instead of jq
    log_step "Step 2: Creating $ISVC_RAW_FILE with raw deployment configuration..."

    if [ "$USE_ORIGINAL_NAMES" == "true" ]; then
        # Use original names - no suffix changes
        yq eval 'del(.metadata.finalizers, .metadata.resourceVersion, .metadata.uid, .status) | 
          .metadata.annotations |= with_entries(select(.key | test("istio|knative") | not)) |
          .metadata.labels |= with_entries(select(.key | test("istio|knative") | not)) |
          .metadata.labels."networking.kserve.io/visibility" = "exposed" |
          .metadata.annotations."serving.kserve.io/deploymentMode" = "RawDeployment"' "$ISVC_FILE" > "$ISVC_RAW_FILE"
    else
        # Use -raw suffix
        yq eval 'del(.metadata.finalizers, .metadata.resourceVersion, .metadata.uid, .status) | 
          .metadata.name += "-raw" | 
          .metadata.annotations |= with_entries(select(.key | test("istio|knative") | not)) |
          .metadata.labels |= with_entries(select(.key | test("istio|knative") | not)) |
          .metadata.labels."networking.kserve.io/visibility" = "exposed" |
          (.metadata.annotations."openshift.io/display-name" | select(. != null)) |= . + "-raw" |
          .metadata.annotations."serving.kserve.io/deploymentMode" = "RawDeployment" |
          .spec.predictor.model.runtime += "-raw"' "$ISVC_FILE" > "$ISVC_RAW_FILE"
    fi

    if [ $? -ne 0 ]; then
        log_error "Failed to transform YAML file"
        exit 1
    fi
    
    # Step 3: Get the serving runtime name and create new serving runtime for raw deployment
    SERVINGRUNTIME_NAME=$(oc get isvc -n $NAMESPACE $NAME -o yaml | yq eval '.spec.predictor.model.runtime // ""' -)
    if [ -z "$SERVINGRUNTIME_NAME" ] || [ "$SERVINGRUNTIME_NAME" == "null" ]; then
        log_error "Could not determine serving runtime name from InferenceService"
        exit 1
    fi
    
    # Now set the ServingRuntime variables
    if [ "$USE_ORIGINAL_NAMES" == "true" ]; then
        SERVINGRUNTIME_NAME_RAW="${SERVINGRUNTIME_NAME}"
    else
        SERVINGRUNTIME_NAME_RAW="${SERVINGRUNTIME_NAME}-raw"
    fi
    SERVINGRUNTIME_FILE="$ORIGINAL_DIR/${SERVINGRUNTIME_NAME}-sr.yaml"
    SERVINGRUNTIME_RAW_FILE="$RAW_DIR/${SERVINGRUNTIME_NAME_RAW}-sr.yaml"
    
    log_step "Step 3: Processing and applying serving runtime $SERVINGRUNTIME_NAME..."
    
    if ! oc get servingruntimes -n $NAMESPACE $SERVINGRUNTIME_NAME &>/dev/null; then
        log_error "ServingRuntime $SERVINGRUNTIME_NAME not found in namespace $NAMESPACE"
        echo -e "${YELLOW}Available ServingRuntimes:${NC}"
        oc get servingruntimes -n "$NAMESPACE" --no-headers -o custom-columns=NAME:.metadata.name 2>/dev/null | sed 's/^/  • /' || echo "  (none found)"
        exit 1
    fi
    
    # Export and clean up the original ServingRuntime
    oc get servingruntimes -n $NAMESPACE $SERVINGRUNTIME_NAME -o yaml | yq eval 'del(.metadata.finalizers, .metadata.resourceVersion, .metadata.uid, .metadata.creationTimestamp, .metadata.generation, .metadata.managedFields, .status)' - > "$SERVINGRUNTIME_FILE"
    if [ $? -ne 0 ] || [ ! -s "$SERVINGRUNTIME_FILE" ]; then
        log_error "Failed to export ServingRuntime or file is empty"
        exit 1
    fi
    
    if [ "$USE_ORIGINAL_NAMES" == "true" ]; then
        # Use original names - just copy the cleaned original
        cp "$SERVINGRUNTIME_FILE" "$SERVINGRUNTIME_RAW_FILE"
    else
        # Use -raw suffix
        yq eval '
        .metadata.name += "-raw" |
        .metadata.annotations."openshift.io/display-name" = (.metadata.annotations."openshift.io/display-name" // "") + "-raw"
        ' "$SERVINGRUNTIME_FILE" > "$SERVINGRUNTIME_RAW_FILE"
    fi
    
    if [ $? -ne 0 ] || [ ! -s "$SERVINGRUNTIME_RAW_FILE" ]; then
        log_error "Failed to process serving runtime $SERVINGRUNTIME_NAME"
        exit 1
    fi
    
    # Step 4: Apply the transformed ServingRuntime
    if [ "$DRY_RUN" == "true" ]; then
        log_step "Step 4: Skipping application of $SERVINGRUNTIME_RAW_FILE (dry-run mode)..."
        log_info "File generated at: $SERVINGRUNTIME_RAW_FILE"
    else
        log_step "Step 4: Applying $SERVINGRUNTIME_RAW_FILE..."
        oc apply -f "$SERVINGRUNTIME_RAW_FILE" -n "$NAMESPACE"
        if [ $? -ne 0 ]; then
            log_error "Failed to apply $SERVINGRUNTIME_RAW_FILE"
            exit 1
        fi
    fi
    
    # Step 5: Check if "enable-auth" annotation is present and add it if missing
    log_step "Step 5: Checking and ensuring 'enable-auth' annotation is set..."
    
    # Check if the annotation exists in the original InferenceService
    ENABLE_AUTH_ORIGINAL=$(oc get isvc -n $NAMESPACE $NAME -o yaml 2>/dev/null | yq eval '.metadata.annotations["security.opendatahub.io/enable-auth"] // null' -)
    
    if [ "$ENABLE_AUTH_ORIGINAL" == "null" ]; then
        log_info "enable-auth annotation not present in original InferenceService, adding it with value 'false'"
        # Add the annotation to the raw InferenceService with value "false"
        yq eval '.metadata.annotations."security.opendatahub.io/enable-auth" = "false"' "$ISVC_RAW_FILE" > "${ISVC_RAW_FILE}.tmp" && mv "${ISVC_RAW_FILE}.tmp" "$ISVC_RAW_FILE"
        ENABLE_AUTH="false"
    else
        ENABLE_AUTH="$ENABLE_AUTH_ORIGINAL"
        log_info "enable-auth annotation found with value: $ENABLE_AUTH"
        # Ensure the annotation is preserved in the raw InferenceService
        yq eval ".metadata.annotations.\"security.opendatahub.io/enable-auth\" = \"$ENABLE_AUTH\"" "$ISVC_RAW_FILE" > "${ISVC_RAW_FILE}.tmp" && mv "${ISVC_RAW_FILE}.tmp" "$ISVC_RAW_FILE"
    fi
    
    if [ "$ENABLE_AUTH" == "true" ]; then
        log_info "enable-auth annotation is true - will process authentication resources"
        log_step "Finding resources owned by InferenceService $NAME..."
        
        # Get the UID of the original InferenceService for ownerReference queries
        ORIGINAL_ISVC_UID=$(oc get isvc -n $NAMESPACE $NAME -o yaml | yq eval '.metadata.uid // ""' -)
        if [ -z "$ORIGINAL_ISVC_UID" ] || [ "$ORIGINAL_ISVC_UID" == "null" ]; then
            log_error "Could not get UID for original InferenceService $NAME"
            exit 1
        fi
        
        log_info "Original InferenceService UID: $ORIGINAL_ISVC_UID"
        
        # Initialize all flags to false
        SERVICE_ACCOUNT_EXISTS="false"
        ROLE_EXISTS="false"
        ROLE_BINDING_EXISTS="false"
        KNATIVE_ROUTE_EXISTS="false"
        SECRET_EXISTS="false"
        
        # Try to find resources by ownerReferences first, then fall back to naming convention
        log_info "Searching for ServiceAccount with ownerReference UID: $ORIGINAL_ISVC_UID"
        local sa_json=$(oc get serviceaccounts -n "$NAMESPACE" -o json 2>/dev/null | jq ".items[] | select(.metadata.ownerReferences[]?.uid == \"$ORIGINAL_ISVC_UID\")" 2>/dev/null)
        if [ -n "$sa_json" ] && [ "$sa_json" != "null" ]; then
            # Found by ownerReferences
            echo "$sa_json" | jq 'del(.metadata.resourceVersion, .metadata.uid, .metadata.creationTimestamp, .metadata.generation, .metadata.managedFields, .metadata.ownerReferences, .secrets, .imagePullSecrets, .metadata.annotations."openshift.io/internal-registry-pull-secret-ref")' | \
                yq eval -P - > "$SA_FILE" 2>/dev/null
            if [ -s "$SA_FILE" ]; then
                SERVICE_ACCOUNT_EXISTS="true"
                SA_NAME=$(echo "$sa_json" | jq -r '.metadata.name')
                log_info "Found ServiceAccount by ownerReference: $SA_NAME"
            fi
        else
            # Fallback to naming convention
            log_info "No ownerReferences found, trying naming convention: ${NAME}-sa"
            if oc get serviceaccount "${NAME}-sa" -n "$NAMESPACE" &>/dev/null; then
                oc get serviceaccount "${NAME}-sa" -n "$NAMESPACE" -o json 2>/dev/null | \
                    jq 'del(.metadata.resourceVersion, .metadata.uid, .metadata.creationTimestamp, .metadata.generation, .metadata.managedFields, .metadata.ownerReferences, .secrets, .imagePullSecrets, .metadata.annotations."openshift.io/internal-registry-pull-secret-ref")' | \
                    yq eval -P - > "$SA_FILE" 2>/dev/null
                if [ -s "$SA_FILE" ]; then
                    SERVICE_ACCOUNT_EXISTS="true"
                    SA_NAME="${NAME}-sa"
                    log_info "Found ServiceAccount by naming convention: $SA_NAME"
                fi
            fi
        fi
        
        log_info "Searching for Role with ownerReference UID: $ORIGINAL_ISVC_UID"
        local role_json=$(oc get roles -n "$NAMESPACE" -o json 2>/dev/null | jq ".items[] | select(.metadata.ownerReferences[]?.uid == \"$ORIGINAL_ISVC_UID\")" 2>/dev/null)
        if [ -n "$role_json" ] && [ "$role_json" != "null" ]; then
            # Found by ownerReferences
            echo "$role_json" | jq 'del(.metadata.resourceVersion, .metadata.uid, .metadata.creationTimestamp, .metadata.generation, .metadata.managedFields, .metadata.ownerReferences)' | \
                yq eval -P - > "$ROLE_FILE" 2>/dev/null
            if [ -s "$ROLE_FILE" ]; then
                ROLE_EXISTS="true"
                ROLE_NAME=$(echo "$role_json" | jq -r '.metadata.name')
                log_info "Found Role by ownerReference: $ROLE_NAME"
            fi
        else
            # Fallback to naming convention
            log_info "No ownerReferences found, trying naming convention: ${NAME}-view-role"
            if oc get role "${NAME}-view-role" -n "$NAMESPACE" &>/dev/null; then
                oc get role "${NAME}-view-role" -n "$NAMESPACE" -o json 2>/dev/null | \
                    jq 'del(.metadata.resourceVersion, .metadata.uid, .metadata.creationTimestamp, .metadata.generation, .metadata.managedFields, .metadata.ownerReferences)' | \
                    yq eval -P - > "$ROLE_FILE" 2>/dev/null
                if [ -s "$ROLE_FILE" ]; then
                    ROLE_EXISTS="true"
                    ROLE_NAME="${NAME}-view-role"
                    log_info "Found Role by naming convention: $ROLE_NAME"
                fi
            fi
        fi
        
        log_info "Searching for RoleBinding with ownerReference UID: $ORIGINAL_ISVC_UID"
        local rolebinding_json=$(oc get rolebindings -n "$NAMESPACE" -o json 2>/dev/null | jq ".items[] | select(.metadata.ownerReferences[]?.uid == \"$ORIGINAL_ISVC_UID\")" 2>/dev/null)
        if [ -n "$rolebinding_json" ] && [ "$rolebinding_json" != "null" ]; then
            # Found by ownerReferences
            echo "$rolebinding_json" | jq 'del(.metadata.resourceVersion, .metadata.uid, .metadata.creationTimestamp, .metadata.generation, .metadata.managedFields, .metadata.ownerReferences)' | \
                yq eval -P - > "$ROLEBINDING_FILE" 2>/dev/null
            if [ -s "$ROLEBINDING_FILE" ]; then
                ROLE_BINDING_EXISTS="true"
                ROLEBINDING_NAME=$(echo "$rolebinding_json" | jq -r '.metadata.name')
                log_info "Found RoleBinding by ownerReference: $ROLEBINDING_NAME"
            fi
        else
            # Fallback to naming convention
            log_info "No ownerReferences found, trying naming convention: ${NAME}-view"
            if oc get rolebinding "${NAME}-view" -n "$NAMESPACE" &>/dev/null; then
                oc get rolebinding "${NAME}-view" -n "$NAMESPACE" -o json 2>/dev/null | \
                    jq 'del(.metadata.resourceVersion, .metadata.uid, .metadata.creationTimestamp, .metadata.generation, .metadata.managedFields, .metadata.ownerReferences)' | \
                    yq eval -P - > "$ROLEBINDING_FILE" 2>/dev/null
                if [ -s "$ROLEBINDING_FILE" ]; then
                    ROLE_BINDING_EXISTS="true"
                    ROLEBINDING_NAME="${NAME}-view"
                    log_info "Found RoleBinding by naming convention: $ROLEBINDING_NAME"
                fi
            fi
        fi
        
        if [ "$SERVICE_ACCOUNT_EXISTS" == "true" ]; then
            log_step "Looking for Secret with service account annotation for $SA_NAME..."
            SECRET_JSON=$(oc get secrets -n $NAMESPACE -o json 2>/dev/null | jq --arg sa_name "$SA_NAME" '.items[] | select(.metadata.annotations."kubernetes.io/service-account.name" == $sa_name)' 2>/dev/null)
            if [ -n "$SECRET_JSON" ] && [ "$SECRET_JSON" != "null" ]; then
                # Extract the display name for the secret template
                DISPLAY_NAME=$(echo "$SECRET_JSON" | jq -r '.metadata.annotations."openshift.io/display-name" // "default-name"')
                
                # Create a template secret that will generate a new token when applied
                cat > "$SECRET_FILE" << EOF
apiVersion: v1
kind: Secret
metadata:
  name: ${DISPLAY_NAME}-${SA_NAME}
  namespace: ${NAMESPACE}
  labels:
    opendatahub.io/dashboard: "true"
  annotations:
    kubernetes.io/service-account.name: ${SA_NAME}
    openshift.io/display-name: ${DISPLAY_NAME}
type: kubernetes.io/service-account-token
EOF
                
                if [ -s "$SECRET_FILE" ]; then
                    SECRET_EXISTS="true"
                    log_info "Created Secret template for service account $SA_NAME (display name: $DISPLAY_NAME)"
                else
                    log_warn "Failed to create secret template"
                fi
            else
                log_warn "No secrets found with service account annotation $SA_NAME"
            fi
        fi
        
        # Check for Knative route
        if oc get routes.serving.knative.dev -n $NAMESPACE $KNATIVE_ROUTE_NAME &>/dev/null; then
            KNATIVE_ROUTE_EXISTS="true"
            log_info "Found Knative Route: $KNATIVE_ROUTE_NAME"
        else
            log_warn "Knative Route $KNATIVE_ROUTE_NAME not found"
        fi
        
    else
        log_info "enable-auth annotation is false or not present, skipping auth resource checks"
        SERVICE_ACCOUNT_EXISTS="false"
        ROLE_EXISTS="false"
        ROLE_BINDING_EXISTS="false"
        KNATIVE_ROUTE_EXISTS="false"
        SECRET_EXISTS="false"
    fi
    
    if [ "$KNATIVE_ROUTE_EXISTS" == "true" ]; then
        yq eval '.metadata.labels."networking.kserve.io/visibility" = "exposed"' "$ISVC_RAW_FILE" > "${ISVC_RAW_FILE}.tmp" && mv "${ISVC_RAW_FILE}.tmp" "$ISVC_RAW_FILE"
    fi
    
    # Delete existing resources if using original names (in-place replacement)
    if [ "$USE_ORIGINAL_NAMES" == "true" ] && [ "$DRY_RUN" != "true" ]; then
        log_step "Step 6a: Deleting existing resources for in-place replacement..."
        delete_existing_resources "$NAME"
        
        # Re-apply the ServingRuntime after deletion
        log_step "Step 6b: Re-applying ServingRuntime after deletion..."
        oc apply -f "$SERVINGRUNTIME_RAW_FILE" -n "$NAMESPACE"
        if [ $? -ne 0 ]; then
            log_error "Failed to re-apply ServingRuntime"
            exit 1
        fi
    fi
    
    # Final Step: Apply the transformed InferenceService first
    if [ "$DRY_RUN" == "true" ]; then
        log_step "Step 6: Skipping application of $ISVC_RAW_FILE (dry-run mode)..."
        log_info "File generated at: $ISVC_RAW_FILE"
        # Use a placeholder UID for dry-run mode
        ISVC_RAW_UID="dry-run-placeholder-uid"
    else
        log_step "Step 6: Applying $ISVC_RAW_FILE..."
        oc apply -f "$ISVC_RAW_FILE" -n "$NAMESPACE"
        
        if [ $? -ne 0 ]; then
            log_error "Failed to apply $ISVC_RAW_FILE"
            exit 1
        fi
        
        # Get the UID of the newly created InferenceService for ownerReferences
        ISVC_RAW_UID=$(oc get isvc -n $NAMESPACE $NAME_RAW -o yaml | yq eval '.metadata.uid' -)
    fi
    
    if [ "$SERVICE_ACCOUNT_EXISTS" == "true" ]; then
        log_step "Processing ServiceAccount..."
        
        if [ "$DRY_RUN" == "true" ]; then
            if ! yq eval "
                del(.metadata.finalizers, .metadata.resourceVersion, .metadata.uid, .status, .metadata.ownerReferences) |
                .metadata.name = \"$NAME_RAW-sa\"
            " "$SA_FILE" > "$SA_RAW_FILE" 2>/dev/null; then
                log_error "Failed to process ServiceAccount YAML"
                exit 1
            fi
        else
            # In normal mode, add ownerReferences to the InferenceService
            if ! yq eval "
                del(.metadata.finalizers, .metadata.resourceVersion, .metadata.uid, .status) |
                .metadata.name = \"$NAME_RAW-sa\" |
                .metadata.ownerReferences = [{
                    \"apiVersion\": \"serving.kserve.io/v1beta1\",
                    \"kind\": \"InferenceService\",
                    \"name\": \"$NAME_RAW\",
                    \"uid\": \"$ISVC_RAW_UID\",
                    \"blockOwnerDeletion\": false
                }]
            " "$SA_FILE" > "$SA_RAW_FILE" 2>/dev/null; then
                log_error "Failed to process ServiceAccount YAML"
                exit 1
            fi
        fi
        
        if [ ! -s "$SA_RAW_FILE" ]; then
            log_error "Processed ServiceAccount file is empty"
            exit 1
        fi
        
        if [ "$DRY_RUN" == "true" ]; then
            log_info "Skipping application of ServiceAccount (dry-run mode)"
            log_info "File generated at: $SA_RAW_FILE"
        else
            oc apply -f "$SA_RAW_FILE" -n "$NAMESPACE"
            if [ $? -ne 0 ]; then
                log_error "Failed to apply ServiceAccount"
                exit 1
            fi
        fi
        
    fi
    
    if [ "$ROLE_EXISTS" == "true" ]; then
        log_step "Processing Role..."
        
        if [ "$DRY_RUN" == "true" ]; then
            if ! yq eval "
                del(.metadata.finalizers, .metadata.resourceVersion, .metadata.uid, .status, .metadata.ownerReferences) |
                .metadata.name = \"$NAME_RAW-view-role\" |
                .rules[0].resourceNames[0] = \"$NAME_RAW\"
            " "$ROLE_FILE" > "$ROLE_RAW_FILE" 2>/dev/null; then
                log_error "Failed to process Role YAML"
                exit 1
            fi
        else
            # In normal mode, add ownerReferences to the InferenceService
            if ! yq eval "
                del(.metadata.finalizers, .metadata.resourceVersion, .metadata.uid, .status) |
                .metadata.name = \"$NAME_RAW-view-role\" |
                .metadata.ownerReferences = [{
                    \"apiVersion\": \"serving.kserve.io/v1beta1\",
                    \"kind\": \"InferenceService\",
                    \"name\": \"$NAME_RAW\",
                    \"uid\": \"$ISVC_RAW_UID\",
                    \"blockOwnerDeletion\": false
                }] |
                .rules[0].resourceNames[0] = \"$NAME_RAW\"
            " "$ROLE_FILE" > "$ROLE_RAW_FILE" 2>/dev/null; then
                log_error "Failed to process Role YAML"
                exit 1
            fi
        fi
        
        if [ ! -s "$ROLE_RAW_FILE" ]; then
            log_error "Processed Role file is empty"
            exit 1
        fi
        
        if [ "$DRY_RUN" == "true" ]; then
            log_info "Skipping application of Role (dry-run mode)"
            log_info "File generated at: $ROLE_RAW_FILE"
        else
            oc apply -f "$ROLE_RAW_FILE" -n "$NAMESPACE"
            if [ $? -ne 0 ]; then
                log_error "Failed to apply Role"
                exit 1
            fi
        fi
        
    fi
    
    if [ "$ROLE_BINDING_EXISTS" == "true" ]; then
        log_step "Processing RoleBinding..."
        
        if [ "$DRY_RUN" == "true" ]; then
            if ! yq eval "
                del(.metadata.finalizers, .metadata.resourceVersion, .metadata.uid, .status, .metadata.ownerReferences) |
                .metadata.name = \"$NAME_RAW-view\" |
                .subjects[0].name = \"$NAME_RAW-sa\" |
                .roleRef.name = \"$NAME_RAW-view-role\"
            " "$ROLEBINDING_FILE" > "$ROLEBINDING_RAW_FILE" 2>/dev/null; then
                log_error "Failed to process RoleBinding YAML"
                exit 1
            fi
        else
            # In normal mode, add ownerReferences to the InferenceService
            if ! yq eval "
                del(.metadata.finalizers, .metadata.resourceVersion, .metadata.uid, .status) |
                .metadata.name = \"$NAME_RAW-view\" |
                .subjects[0].name = \"$NAME_RAW-sa\" |
                .roleRef.name = \"$NAME_RAW-view-role\" |
                .metadata.ownerReferences = [{
                    \"apiVersion\": \"serving.kserve.io/v1beta1\",
                    \"kind\": \"InferenceService\",
                    \"name\": \"$NAME_RAW\",
                    \"uid\": \"$ISVC_RAW_UID\",
                    \"blockOwnerDeletion\": false
                }]
            " "$ROLEBINDING_FILE" > "$ROLEBINDING_RAW_FILE" 2>/dev/null; then
                log_error "Failed to process RoleBinding YAML"
                exit 1
            fi
        fi
        
        if [ ! -s "$ROLEBINDING_RAW_FILE" ]; then
            log_error "Processed RoleBinding file is empty"
            exit 1
        fi
        
        if [ "$DRY_RUN" == "true" ]; then
            log_info "Skipping application of RoleBinding (dry-run mode)"
            log_info "File generated at: $ROLEBINDING_RAW_FILE"
        else
            oc apply -f "$ROLEBINDING_RAW_FILE" -n "$NAMESPACE"
            if [ $? -ne 0 ]; then
                log_error "Failed to apply RoleBinding"
                exit 1
            fi
        fi
        
    fi
    
    if [ "$SECRET_EXISTS" == "true" ]; then
        log_step "Processing Secret..."
        
        # Get the display name from the secret template
        DISPLAY_NAME=$(yq eval '.metadata.annotations."openshift.io/display-name" // "default-name"' "$SECRET_FILE" 2>/dev/null || echo "default-name")
        
        if [ "$DRY_RUN" == "true" ]; then
            cat > "$SECRET_RAW_FILE" << EOF
apiVersion: v1
kind: Secret
metadata:
  name: ${DISPLAY_NAME}-${SA_NAME_RAW}
  namespace: ${NAMESPACE}
  labels:
    opendatahub.io/dashboard: "true"
  annotations:
    kubernetes.io/service-account.name: ${SA_NAME_RAW}
    openshift.io/display-name: ${DISPLAY_NAME}
type: kubernetes.io/service-account-token
EOF
        else
            # Wait for service account to be ready and get its UID
            log_step "Waiting for ServiceAccount to be ready..."
            for i in {1..30}; do
                SA_UID=$(oc get serviceaccount -n $NAMESPACE $SA_NAME_RAW -o yaml | yq eval '.metadata.uid' - 2>/dev/null)
                if [ -n "$SA_UID" ] && [ "$SA_UID" != "null" ]; then
                    log_info "ServiceAccount UID: $SA_UID"
                    break
                fi
                echo -e "  ${YELLOW}Waiting for ServiceAccount... ($i/30)${NC}"
                sleep 2
            done
            
            if [ -z "$SA_UID" ] || [ "$SA_UID" == "null" ]; then
                log_error "Could not get UID for service account $SA_NAME_RAW after waiting"
                exit 1
            fi
            
            cat > "$SECRET_RAW_FILE" << EOF
apiVersion: v1
kind: Secret
metadata:
  name: ${DISPLAY_NAME}-${SA_NAME_RAW}
  namespace: ${NAMESPACE}
  labels:
    opendatahub.io/dashboard: "true"
  annotations:
    kubernetes.io/service-account.name: ${SA_NAME_RAW}
    kubernetes.io/service-account.uid: ${SA_UID}
    openshift.io/display-name: ${DISPLAY_NAME}
type: kubernetes.io/service-account-token
EOF
        fi
        
        if [ ! -s "$SECRET_RAW_FILE" ]; then
            log_error "Failed to create secret YAML file"
            exit 1
        fi
        
        if [ "$DRY_RUN" == "true" ]; then
            log_info "Skipping application of Secret (dry-run mode)"
            log_info "File generated at: $SECRET_RAW_FILE"
        else
            oc apply -f "$SECRET_RAW_FILE" -n "$NAMESPACE"
            if [ $? -ne 0 ]; then
                log_error "Failed to apply Secret"
                exit 1
            fi
        fi
    fi
    
    # Resources have been applied or generated above
    if [ "$DRY_RUN" == "true" ]; then
        log_step "Step 7: Auth resources generated (if they existed)..."
    else
        log_step "Step 7: Auth resources applied (if they existed)..."
    fi
    if [ "$SERVICE_ACCOUNT_EXISTS" == "true" ]; then
        log_info "  - ServiceAccount: $SA_NAME_RAW"
    fi
    if [ "$ROLE_EXISTS" == "true" ]; then
        log_info "  - Role: $ROLE_NAME_RAW"
    fi
    if [ "$ROLE_BINDING_EXISTS" == "true" ]; then
        log_info "  - RoleBinding: $ROLEBINDING_NAME_RAW"
    fi
    if [ "$SECRET_EXISTS" == "true" ]; then
        log_info "  - Secret: ${DISPLAY_NAME:-default-name}-$SA_NAME_RAW"
    fi
    
    echo ""
    log_info "✅ Completed conversion for ${NAME} → ${NAME_RAW}"
    if [ "$USE_ORIGINAL_NAMES" == "true" ]; then
        log_info "📁 Generated files with original names in: ${RESOURCE_DIR}/raw-original-names/"
    else
        log_info "📁 Generated files with -raw suffix in: ${RESOURCE_DIR}/raw/"
    fi
    
    # Delete existing resources if requested (only for -raw suffix mode)
    if [ "$DELETE_EXISTING" == "true" ] && [ "$DRY_RUN" != "true" ] && [ "$USE_ORIGINAL_NAMES" != "true" ]; then
        delete_existing_resources "$NAME"
    fi

    # Cleanup per PRESERVE_FILES
    if [[ "$PRESERVE_FILES" != "true" ]]; then
        log_step "Cleaning up temporary files for ${NAME}..."
        cleanup
        log_info "Cleaned ${RESOURCE_DIR}/"
    else
        log_info "Preserved files under ${RESOURCE_DIR}/"
    fi
}

# Main conversion logic (keeping the existing logic but with better variable names)
main() {
    # Set up global cleanup for temporary directories
    cleanup_temp_dirs() {
        # Clean up any .tmp-* directories that weren't preserved
        local temp_dirs=$(find . -maxdepth 1 -type d -name ".tmp-*" 2>/dev/null)
        if [ -n "$temp_dirs" ]; then
            log_step "Cleaning up temporary directories..."
            echo "$temp_dirs" | while read -r dir; do
                if [ -n "$dir" ] && [ -d "$dir" ]; then
                    rm -rf "$dir" 2>/dev/null
                    log_info "Removed temporary directory: $dir"
                fi
            done
        fi
    }
    
    # Set up trap to cleanup temp directories on exit (success or failure)
    trap cleanup_temp_dirs EXIT
    
    # Parse arguments
    parse_arguments "$@"

    # Validate environment
    check_prerequisites
    validate_arguments
    validate_namespace
    check_permissions

    list_and_select_inference_services
    collect_naming_preference
    collect_preserve_file_response

    # Process each selected ISVC
    for name in "${SELECTED_ISVCS[@]}"; do
      echo -e "\n================================================"
      echo "Converting: $name"
      echo "================================================"
      if ! convert_isvc "$name"; then
        log_error "Conversion failed for $name (continuing to next)"
      fi
    done

    log_info "All requested conversions finished."
    echo ""
    
    # Show warning for in-place replacement mode (especially in dry-run)
    if [ "$USE_ORIGINAL_NAMES" == "true" ] && [ "$DRY_RUN" == "true" ]; then
        echo ""
        echo -e "${RED}⚠️  WARNING: IN-PLACE REPLACEMENT MODE${NC}"
        echo -e "${RED}=================================${NC}"
        echo ""
        echo -e "${YELLOW}You selected to use original names. Before applying these resources:${NC}"
        echo ""
        echo -e "${RED}• The converted resources will REPLACE the existing ones${NC}"
        echo -e "${RED}• You MUST delete all existing resources before applying the generated files${NC}"
        echo -e "${RED}• There is NO TURNING BACK once the original resources are deleted${NC}"
        echo -e "${RED}• If conversion fails, you may lose your original configuration${NC}"
        echo -e "${RED}• Tokens will be DIFFERENT if you decide to roll back and apply the original files${NC}"
        echo ""
        echo -e "${YELLOW}Recommendations:${NC}"
        echo "• Test the generated files thoroughly before applying"
        echo "• Backup your resources: oc get isvc,servingruntimes,sa,roles,rolebindings,secrets -n $NAMESPACE -o yaml > backup.yaml"
        echo "• Consider using --delete-existing flag with original names to automate cleanup"
        echo ""
        
        echo -e "${YELLOW}Next steps for each InferenceService (dry-run mode):${NC}"
        for name in "${SELECTED_ISVCS[@]}"; do
            echo ""
            echo -e "For InferenceService: ${CYAN}$name${NC}"
            echo "  1. Review the generated files in: $name/"
            echo -e "  2. Delete existing InferenceService: ${CYAN}oc delete isvc $name -n $NAMESPACE${NC}"
            echo "  3. Delete existing ServingRuntime: Check $name/original/ for runtime name, then delete it"
            echo -e "  4. Delete auth resources: ${CYAN}oc delete sa ${name}-sa role/${name}-view-role rolebinding/${name}-view -n $NAMESPACE${NC}"
            echo -e "  5. ${RED}Delete Istio route: oc delete route ${name}-${NAMESPACE} -n istio-system${NC}"
            echo -e "  6. Apply the raw resources: ${CYAN}oc apply -f $name/raw-original-names/ -n $NAMESPACE${NC}"
            echo -e "  7. Verify: ${CYAN}oc get isvc $name -n $NAMESPACE${NC}"
            echo -e "  8. Test endpoint: ${CYAN}oc get isvc $name -n $NAMESPACE -o jsonpath='{.status.url}'${NC}"
            echo -e "  9. Monitor: ${CYAN}oc get pods -n $NAMESPACE -l serving.kserve.io/inferenceservice=$name${NC}"
        done
        
        echo ""
        echo -e "${YELLOW}Optional: Add ownerReferences for automatic garbage collection${NC}"
        echo -e "After applying the resources, you can optionally add ownerReferences to enable"
        echo -e "automatic cleanup when the InferenceService is deleted:"
        echo ""
        for name in "${SELECTED_ISVCS[@]}"; do
            echo -e "  ${CYAN}./add-owner-references.sh -n $NAMESPACE -i $name${NC}"
        done
    elif [ "$DRY_RUN" == "true" ]; then
        echo -e "${YELLOW}Next steps for each InferenceService (dry-run mode):${NC}"
        for name in "${SELECTED_ISVCS[@]}"; do
            local name_raw="${name}-raw"
            echo ""
            echo -e "For InferenceService: ${CYAN}$name${NC} → ${CYAN}$name_raw${NC}"
            echo "  1. Review the generated files in: $name/"
            echo -e "  2. Apply the raw resources: ${CYAN}oc apply -f $name/raw/ -n $NAMESPACE${NC}"
            echo -e "  3. Verify: ${CYAN}oc get isvc $name_raw -n $NAMESPACE${NC}"
            echo -e "  4. Test endpoint: ${CYAN}oc get isvc $name_raw -n $NAMESPACE -o jsonpath='{.status.url}'${NC}"
            echo -e "  5. Monitor: ${CYAN}oc get pods -n $NAMESPACE -l serving.kserve.io/inferenceservice=$name_raw${NC}"
        done
        
        echo ""
        echo -e "${YELLOW}Optional: Add ownerReferences for automatic garbage collection${NC}"
        echo -e "After applying the resources, you can optionally add ownerReferences to enable"
        echo -e "automatic cleanup when the InferenceService is deleted:"
        echo ""
        for name in "${SELECTED_ISVCS[@]}"; do
            local name_raw="${name}-raw"
            echo -e "  ${CYAN}./add-owner-references.sh -n $NAMESPACE -i $name_raw${NC}"
        done
    fi
}


# Run the main function with all arguments
main "$@"

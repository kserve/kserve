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
NC='\033[0m' # No Color

# Function to display help
show_help() {
    cat << EOF
KServe InferenceService Raw Deployment Converter

DESCRIPTION:
    Converts KServe InferenceServices from Serverless deployment mode to Raw deployment mode.
    This script automates the process of migrating models from Knative-based serverless 
    deployments to standard Kubernetes deployments for better control and resource management.

USAGE:
    $SCRIPT_NAME [OPTIONS]

OPTIONS:
    -n, --namespace NAMESPACE   Target namespace containing InferenceServices
                               If not specified, uses current OpenShift context namespace
    
    --dry-run                  Generate transformation files without applying to cluster
                               Files are always preserved when using this option
    
    --delete-existing          Delete existing InferenceServices and related resources
                               after successful conversion (cannot be used with --dry-run)
    
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
        cat <<EOF
  <inference-service-name>/
  ├── original/
  │   ├── <name>-isvc.yaml              # Original InferenceService (YAML)
  │   ├── <runtime>-sr.yaml             # Original ServingRuntime (YAML)
  │   ├── <name>-sa.yaml                # Original ServiceAccount (if auth enabled)
  │   ├── <name>-view-role.yaml         # Original Role (if auth enabled)
  │   ├── <name>-view-rolebinding.yaml  # Original RoleBinding (if auth enabled)
  │   └── <name>-secret.yaml            # Original Secret (if auth enabled)
  ├── raw/
  │   ├── <name>-raw-isvc.yaml          # Transformed InferenceService (renamed with -raw)
  │   ├── <runtime>-raw-sr.yaml         # Transformed ServingRuntime (renamed with -raw)
  │   ├── <name>-raw-sa.yaml            # Transformed ServiceAccount (renamed with -raw, if auth enabled)
  │   ├── <name>-raw-view-role.yaml     # Transformed Role (renamed with -raw, if auth enabled)
  │   ├── <name>-raw-view-rolebinding.yaml # Transformed RoleBinding (renamed with -raw, if auth enabled)
  │   └── <name>-raw-secret.yaml        # Transformed Secret (renamed with -raw, if auth enabled)
  └── raw-original-names/
      ├── <name>-isvc.yaml              # Transformed InferenceService with original name
      ├── <runtime>-sr.yaml             # Transformed ServingRuntime with original name
      ├── <name>-sa.yaml                # Transformed ServiceAccount with original name (if auth enabled)
      ├── <name>-view-role.yaml         # Transformed Role with original name (if auth enabled)
      ├── <name>-view-rolebinding.yaml  # Transformed RoleBinding with original name (if auth enabled)
      └── <name>-secret.yaml            # Transformed Secret with original name (if auth enabled)
EOF
        echo ""
        return 0
    fi

    echo -e "${YELLOW}After conversion, do you want to preserve the exported and transformed files?${NC}"
    echo -e "If you choose to keep them, they'll be organized as follows:\n"

    cat <<EOF
  <inference-service-name>/
  ├── original/
  │   ├── <name>-isvc.yaml              # Original InferenceService (YAML)
  │   ├── <runtime>-sr.yaml             # Original ServingRuntime (YAML)
  │   ├── <name>-sa.yaml                # Original ServiceAccount (if auth enabled)
  │   ├── <name>-view-role.yaml         # Original Role (if auth enabled)
  │   ├── <name>-view-rolebinding.yaml  # Original RoleBinding (if auth enabled)
  │   └── <name>-secret.yaml            # Original Secret (if auth enabled)
  ├── raw/
  │   ├── <name>-raw-isvc.yaml          # Transformed InferenceService (renamed with -raw)
  │   ├── <runtime>-raw-sr.yaml         # Transformed ServingRuntime (renamed with -raw)
  │   ├── <name>-raw-sa.yaml            # Transformed ServiceAccount (renamed with -raw, if auth enabled)
  │   ├── <name>-raw-view-role.yaml     # Transformed Role (renamed with -raw, if auth enabled)
  │   ├── <name>-raw-view-rolebinding.yaml # Transformed RoleBinding (renamed with -raw, if auth enabled)
  │   └── <name>-raw-secret.yaml        # Transformed Secret (renamed with -raw, if auth enabled)
  └── raw-original-names/
      ├── <name>-isvc.yaml              # Transformed InferenceService with original name
      ├── <runtime>-sr.yaml             # Transformed ServingRuntime with original name
      ├── <name>-sa.yaml                # Transformed ServiceAccount with original name (if auth enabled)
      ├── <name>-view-role.yaml         # Transformed Role with original name (if auth enabled)
      ├── <name>-view-rolebinding.yaml  # Transformed RoleBinding with original name (if auth enabled)
      └── <name>-secret.yaml            # Transformed Secret with original name (if auth enabled)
EOF
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
    echo ""
    echo "2) Use -raw suffix (for side-by-side deployment)"
    echo "   - Creates new resources alongside existing ones"
    echo "   - Example: 'my-model' becomes 'my-model-raw'"
    echo ""
    
    if [ "$DELETE_EXISTING" == "true" ]; then
        echo -e "${YELLOW}⚠️  Note: --delete-existing flag is set. Original resources will be deleted after conversion.${NC}"
        echo ""
    fi
    
    local naming_choice=""
    while true; do
        read -p "Enter your choice (1 or 2): " naming_choice
        case "$naming_choice" in
            1)
                echo ""
                echo -e "${RED}⚠️  WARNING: IN-PLACE REPLACEMENT MODE${NC}"
                echo -e "${RED}=================================${NC}"
                echo ""
                echo -e "${YELLOW}You have chosen to use original names. This means:${NC}"
                echo ""
                echo -e "${RED}• The converted resources will REPLACE the existing ones${NC}"
                echo -e "${RED}• You MUST delete all existing resources associated with the original InferenceService${NC}"
                echo -e "${RED}• There is NO TURNING BACK once the original resources are deleted${NC}"
                echo -e "${RED}• If conversion fails, you may lose your original configuration${NC}"
                echo ""
                echo -e "${YELLOW}Recommendations:${NC}"
                echo "• Use --dry-run first to test the conversion"
                echo "• Backup your resources: oc get isvc,servingruntimes,sa,roles,rolebindings,secrets -n <namespace> -o yaml > backup.yaml"
                echo "• Consider using --delete-existing flag to automate cleanup after successful conversion"
                echo ""
                
                local confirm_choice=""
                while true; do
                    read -p "Are you sure you want to proceed with in-place replacement? (yes/no): " confirm_choice
                    case "$confirm_choice" in
                        yes|YES)
                            USE_ORIGINAL_NAMES="true"
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
    
    # Delete InferenceService
    if oc get isvc -n "$NAMESPACE" "$name" &>/dev/null; then
        log_info "Deleting InferenceService: $name"
        oc delete isvc -n "$NAMESPACE" "$name" --ignore-not-found=true
    fi
    
    if [ -n "$original_isvc_uid" ] && [ "$original_isvc_uid" != "null" ]; then
        # Find and delete owned resources
        log_info "Looking for resources owned by InferenceService $name..."
        
        # Delete ServiceAccount
        local sa_name=$(oc get serviceaccounts -n "$NAMESPACE" -o yaml 2>/dev/null | yq eval ".items[] | select(.metadata.ownerReferences[]?.uid == \"$original_isvc_uid\") | .metadata.name" -)
        if [ -n "$sa_name" ] && [ "$sa_name" != "null" ]; then
            log_info "Deleting ServiceAccount: $sa_name"
            oc delete serviceaccount -n "$NAMESPACE" "$sa_name" --ignore-not-found=true
        fi
        
        # Delete Role
        local role_name=$(oc get roles -n "$NAMESPACE" -o yaml 2>/dev/null | yq eval ".items[] | select(.metadata.ownerReferences[]?.uid == \"$original_isvc_uid\") | .metadata.name" -)
        if [ -n "$role_name" ] && [ "$role_name" != "null" ]; then
            log_info "Deleting Role: $role_name"
            oc delete role -n "$NAMESPACE" "$role_name" --ignore-not-found=true
        fi
        
        # Delete RoleBinding
        local rolebinding_name=$(oc get rolebindings -n "$NAMESPACE" -o yaml 2>/dev/null | yq eval ".items[] | select(.metadata.ownerReferences[]?.uid == \"$original_isvc_uid\") | .metadata.name" -)
        if [ -n "$rolebinding_name" ] && [ "$rolebinding_name" != "null" ]; then
            log_info "Deleting RoleBinding: $rolebinding_name"
            oc delete rolebinding -n "$NAMESPACE" "$rolebinding_name" --ignore-not-found=true
        fi
        
        # Delete Secrets associated with the service account
        if [ -n "$sa_name" ] && [ "$sa_name" != "null" ]; then
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

    # Always create resource directories (we'll decide later whether to keep them)
    RESOURCE_DIR="$NAME"
    ORIGINAL_DIR="$RESOURCE_DIR/original"
    
    if [ "$USE_ORIGINAL_NAMES" == "true" ]; then
        RAW_DIR="$RESOURCE_DIR/raw-original-names"
        log_step "Creating temporary resource directories (original names mode)..."
        mkdir -p "$ORIGINAL_DIR" "$RAW_DIR"
    else
        RAW_DIR="$RESOURCE_DIR/raw"
        log_step "Creating temporary resource directories (-raw suffix mode)..."
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
    
    oc get servingruntimes -n $NAMESPACE $SERVINGRUNTIME_NAME -o yaml > "$SERVINGRUNTIME_FILE"
    if [ $? -ne 0 ] || [ ! -s "$SERVINGRUNTIME_FILE" ]; then
        log_error "Failed to export ServingRuntime or file is empty"
        exit 1
    fi
    
    if [ "$USE_ORIGINAL_NAMES" == "true" ]; then
        # Use original names - no suffix changes
        yq eval 'del(.metadata.finalizers, .metadata.resourceVersion, .metadata.uid, .status)' "$SERVINGRUNTIME_FILE" > "$SERVINGRUNTIME_RAW_FILE"
    else
        # Use -raw suffix
        yq eval '
        del(.metadata.finalizers, .metadata.resourceVersion, .metadata.uid, .status) |
        .metadata.name += "-raw" |
        .metadata.annotations."openshift.io/display-name" = (.metadata.annotations."openshift.io/display-name" // "") + "-raw"
        ' "$SERVINGRUNTIME_FILE" > "$SERVINGRUNTIME_RAW_FILE"
    fi
    
    if [ $? -ne 0 ] || [ ! -s "$SERVINGRUNTIME_RAW_FILE" ]; then
        log_error "Failed to process serving runtime $SERVINGRUNTIME_NAME"
        exit 1
    fi
    
    # Step 4: Apply the transformed ServingRuntime
    log_step "Step 4: Applying $SERVINGRUNTIME_RAW_FILE..."
    oc apply -f "$SERVINGRUNTIME_RAW_FILE" -n "$NAMESPACE"
    if [ $? -ne 0 ]; then
        log_error "Failed to apply $SERVINGRUNTIME_RAW_FILE"
        exit 1
    fi
    
    # Helper function to find resources by ownerReference
    find_owned_resource() {
        local resource_type=$1
        local owner_uid=$2
        local namespace=$3
        local output_file=$4
        
        # Send log messages to stderr to avoid interfering with return value
        log_step "Looking for $resource_type with ownerReference..." >&2
        
        local resource_yaml=$(oc get $resource_type -n $namespace -o yaml 2>/dev/null)
        if [ -n "$resource_yaml" ]; then
            echo "$resource_yaml" | yq eval ".items[] | select(.metadata.ownerReferences[]?.uid == \"$owner_uid\")" - > "$output_file" 2>/dev/null
            
            if [ -s "$output_file" ]; then
                local found_name=$(yq eval '.metadata.name' "$output_file" 2>/dev/null)
                if [ -n "$found_name" ] && [ "$found_name" != "null" ]; then
                    log_info "Found $resource_type: $found_name (owned by InferenceService)" >&2
                    echo "$found_name"  # This goes to stdout for capture
                    return 0
                fi
            fi
        fi
        
        log_warn "No $resource_type found with ownerReference" >&2
        rm -f "$output_file" 2>/dev/null
        return 1
    }
    
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
        
        # Find resources using ownerReferences
        if FOUND_SA_NAME=$(find_owned_resource "serviceaccounts" "$ORIGINAL_ISVC_UID" "$NAMESPACE" "$SA_FILE"); then
            SERVICE_ACCOUNT_EXISTS="true"
            SA_NAME="$FOUND_SA_NAME"
        fi
        
        if FOUND_ROLE_NAME=$(find_owned_resource "roles" "$ORIGINAL_ISVC_UID" "$NAMESPACE" "$ROLE_FILE"); then
            ROLE_EXISTS="true"
            ROLE_NAME="$FOUND_ROLE_NAME"
        fi
        
        if FOUND_ROLEBINDING_NAME=$(find_owned_resource "rolebindings" "$ORIGINAL_ISVC_UID" "$NAMESPACE" "$ROLEBINDING_FILE"); then
            ROLE_BINDING_EXISTS="true"
            ROLEBINDING_NAME="$FOUND_ROLEBINDING_NAME"
        fi
        
        if [ "$SERVICE_ACCOUNT_EXISTS" == "true" ]; then
            log_step "Looking for Secret with service account annotation for $SA_NAME..."
            SECRET_JSON=$(oc get secrets -n $NAMESPACE -o json 2>/dev/null | jq --arg sa_name "$SA_NAME" '.items[] | select(.metadata.annotations."kubernetes.io/service-account.name" == $sa_name)' 2>/dev/null)
            if [ -n "$SECRET_JSON" ] && [ "$SECRET_JSON" != "null" ]; then
                echo "$SECRET_JSON" | jq -s '{"apiVersion": "v1", "kind": "List", "items": .}' > "$SECRET_FILE" 2>/dev/null
                if [ -s "$SECRET_FILE" ]; then
                    SECRET_EXISTS="true"
                    FOUND_SECRET_NAME=$(echo "$SECRET_JSON" | jq -r '.metadata.name')
                    log_info "Found Secret: $FOUND_SECRET_NAME (for service account $SA_NAME)"
                else
                    log_warn "Secret found but failed to process"
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
    fi
    
    # Final Step: Apply the transformed InferenceService first
    log_step "Step 6: Applying $ISVC_RAW_FILE..."
    oc apply -f "$ISVC_RAW_FILE" -n "$NAMESPACE"
    
    if [ $? -ne 0 ]; then
        log_error "Failed to apply $ISVC_RAW_FILE"
        exit 1
    fi
    
    # Get the UID of the newly created InferenceService for ownerReferences
    ISVC_RAW_UID=$(oc get isvc -n $NAMESPACE $NAME_RAW -o yaml | yq eval '.metadata.uid' -)
    
    if [ "$SERVICE_ACCOUNT_EXISTS" == "true" ]; then
        log_step "Processing ServiceAccount..."
        if ! yq eval "
            del(.metadata.finalizers, .metadata.resourceVersion, .metadata.uid, .status) |
            .metadata.name = \"$NAME_RAW-sa\" |
            .metadata.ownerReferences[0].name = \"$NAME_RAW\" |
            .metadata.ownerReferences[0].uid = \"$ISVC_RAW_UID\"
        " "$SA_FILE" > "$SA_RAW_FILE" 2>/dev/null; then
            log_error "Failed to process ServiceAccount YAML"
            exit 1
        fi
        
        if [ ! -s "$SA_RAW_FILE" ]; then
            log_error "Processed ServiceAccount file is empty"
            exit 1
        fi
        
        oc apply -f "$SA_RAW_FILE" -n "$NAMESPACE"
        if [ $? -ne 0 ]; then
            log_error "Failed to apply ServiceAccount"
            exit 1
        fi
        
    fi
    
    if [ "$ROLE_EXISTS" == "true" ]; then
        log_step "Processing Role..."
        if ! yq eval "
            del(.metadata.finalizers, .metadata.resourceVersion, .metadata.uid, .status) |
            .metadata.name = \"$NAME_RAW-view-role\" |
            .metadata.ownerReferences[0].name = \"$NAME_RAW\" |
            .metadata.ownerReferences[0].uid = \"$ISVC_RAW_UID\" |
            .rules[0].resourceNames[0] = \"$NAME_RAW\"
        " "$ROLE_FILE" > "$ROLE_RAW_FILE" 2>/dev/null; then
            log_error "Failed to process Role YAML"
            exit 1
        fi
        
        if [ ! -s "$ROLE_RAW_FILE" ]; then
            log_error "Processed Role file is empty"
            exit 1
        fi
        
        oc apply -f "$ROLE_RAW_FILE" -n "$NAMESPACE"
        if [ $? -ne 0 ]; then
            log_error "Failed to apply Role"
            exit 1
        fi
        
    fi
    
    if [ "$ROLE_BINDING_EXISTS" == "true" ]; then
        log_step "Processing RoleBinding..."
        if ! yq eval "
            del(.metadata.finalizers, .metadata.resourceVersion, .metadata.uid, .status) |
            .metadata.name = \"$NAME_RAW-view\" |
            .subjects[0].name = \"$NAME_RAW-sa\" |
            .roleRef.name = \"$NAME_RAW-view-role\" |
            .metadata.ownerReferences[0].name = \"$NAME_RAW\" |
            .metadata.ownerReferences[0].uid = \"$ISVC_RAW_UID\"
        " "$ROLEBINDING_FILE" > "$ROLEBINDING_RAW_FILE" 2>/dev/null; then
            log_error "Failed to process RoleBinding YAML"
            exit 1
        fi
        
        if [ ! -s "$ROLEBINDING_RAW_FILE" ]; then
            log_error "Processed RoleBinding file is empty"
            exit 1
        fi
        
        oc apply -f "$ROLEBINDING_RAW_FILE" -n "$NAMESPACE"
        if [ $? -ne 0 ]; then
            log_error "Failed to apply RoleBinding"
            exit 1
        fi
        
    fi
    
    if [ "$SECRET_EXISTS" == "true" ]; then
        log_step "Processing Secret..."
        
        # Check if there are actually items in the secret list
        SECRET_COUNT=$(yq eval '.items | length' "$SECRET_FILE" 2>/dev/null || echo "0")
        if [ "$SECRET_COUNT" -eq 0 ]; then
            log_warn "No secrets found in exported list"
            log_warn "Skipping secret creation..."
        else
            # Create a new service account token secret
            DISPLAY_NAME=$(yq eval '.items[0].metadata.annotations."openshift.io/display-name" // "default-name"' "$SECRET_FILE" 2>/dev/null || echo "default-name")
            
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
            
            # Create the secret manifest
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
            
            if [ ! -s "$SECRET_RAW_FILE" ]; then
                log_error "Failed to create secret YAML file"
                exit 1
            fi
            
            oc apply -f "$SECRET_RAW_FILE" -n "$NAMESPACE"
            if [ $? -ne 0 ]; then
                log_error "Failed to apply Secret"
                exit 1
            fi
            
        fi
    fi
    
    # Resources have been applied above
    log_step "Step 7: Auth resources applied (if they existed)..."
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
    if [ "$DRY_RUN" == "true" ]; then
        echo -e "${YELLOW}Next steps (dry-run mode):${NC}"
        echo "  1. Review the generated files in the respective directories"
        echo "  2. Apply the raw resources manually: oc apply -f <inference-service-name>/raw/ -n $NAMESPACE"
        echo "  3. Verify the deployment: oc get isvc -n $NAMESPACE <NAME_RAW>"
        echo "  4. Test the endpoint: oc get isvc -n $NAMESPACE <NAME_RAW> -o jsonpath='{.status.url}'"
        echo "  5. Monitor the deployment: oc get pods -n $NAMESPACE -l serving.kserve.io/inferenceservice=<NAME_RAW>"
    else
        echo -e "${YELLOW}Next steps:${NC}"
        echo "  1. Verify the raw deployment: oc get isvc -n $NAMESPACE <NAME_RAW>"
        echo "  2. Test the endpoint: oc get isvc -n $NAMESPACE <NAME_RAW> -o jsonpath='{.status.url}'"
        echo "  3. Monitor the deployment: oc get pods -n $NAMESPACE -l serving.kserve.io/inferenceservice=<NAME_RAW>"
    fi
}


# Run the main function with all arguments
main "$@"

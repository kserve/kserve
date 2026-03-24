#!/bin/bash

# Script to disable hardware profile annotations in inferenceservice-config
# This adds opendatahub.io/managed=false annotation and adds hardware profile
# annotations to serviceAnnotationDisallowedList

set -e

SCRIPT_NAME="$(basename "$0")"
NAMESPACE=""
DRY_RUN="false"
CONFIGMAP_NAME="inferenceservice-config"

ANNOTATIONS_TO_ADD=(
    "opendatahub.io/hardware-profile-name"
    "opendatahub.io/hardware-profile-namespace"
)

CONFIGMAP_JSON=""
MANAGED_ANNOTATION_EXISTS="false"

show_help() {
    cat << EOF
Usage: $SCRIPT_NAME -n <namespace> [OPTIONS]

Modifies the inferenceservice-config ConfigMap to:
  1. Add annotation 'opendatahub.io/managed=false'
  2. Add hardware profile annotations to serviceAnnotationDisallowedList

OPTIONS:
    -n, --namespace NAMESPACE   Application namespace (required)
    --dry-run                   Show what would be changed without applying
    -h, --help                  Show this help message

EXAMPLES:
    $SCRIPT_NAME -n my-app
    $SCRIPT_NAME --namespace my-app --dry-run
EOF
    exit 0
}

parse_arguments() {
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
                    echo "Error: Option $1 requires a value" >&2
                    exit 1
                fi
                ;;
            --dry-run)
                DRY_RUN="true"
                shift
                ;;
            -h|--help)
                show_help
                ;;
            *)
                echo "Error: Unknown option: $1" >&2
                exit 1
                ;;
        esac
    done
}

check_prerequisites() {
    if ! command -v oc &> /dev/null; then
        echo "Error: oc (OpenShift CLI) is required" >&2
        exit 1
    fi

    if ! command -v jq &> /dev/null; then
        echo "Error: jq is required" >&2
        exit 1
    fi

    if ! oc whoami &> /dev/null; then
        echo "Error: Not logged into OpenShift cluster" >&2
        exit 1
    fi
}

validate_arguments() {
    if [ -z "$NAMESPACE" ]; then
        echo "Error: Namespace is required. Use -n or --namespace" >&2
        exit 1
    fi
}

validate_namespace() {
    if ! oc get namespace "$NAMESPACE" &> /dev/null; then
        echo "Error: Namespace '$NAMESPACE' does not exist" >&2
        exit 1
    fi
}

get_configmap() {
    if ! CONFIGMAP_JSON=$(oc get configmap "$CONFIGMAP_NAME" -n "$NAMESPACE" -o json 2>&1); then
        echo "Error: Failed to retrieve ConfigMap '$CONFIGMAP_NAME' from namespace '$NAMESPACE'" >&2
        exit 1
    fi
}

check_managed_annotation() {
    local current_value
    current_value=$(echo "$CONFIGMAP_JSON" | jq -r '.metadata.annotations["opendatahub.io/managed"] // "not-set"')

    if [ "$current_value" == "false" ]; then
        echo "Annotation 'opendatahub.io/managed=false' already exists"
        MANAGED_ANNOTATION_EXISTS="true"
    else
        echo "Annotation 'opendatahub.io/managed' is '$current_value', will set to 'false'"
        MANAGED_ANNOTATION_EXISTS="false"
    fi
}

add_managed_annotation() {
    if [ "$MANAGED_ANNOTATION_EXISTS" == "true" ]; then
        return 0
    fi

    if [ "$DRY_RUN" == "true" ]; then
        echo "[DRY-RUN] Would add annotation: opendatahub.io/managed=false"
        return 0
    fi

    if ! oc annotate configmap "$CONFIGMAP_NAME" -n "$NAMESPACE" "opendatahub.io/managed=false" --overwrite; then
        echo "Error: Failed to add managed annotation" >&2
        exit 1
    fi

    echo "Added annotation: opendatahub.io/managed=false"
}

update_disallowed_list() {
    local inference_service_json
    inference_service_json=$(echo "$CONFIGMAP_JSON" | jq -r '.data.inferenceService // "{}"')

    local parsed_json
    if ! parsed_json=$(echo "$inference_service_json" | jq '.' 2>&1); then
        echo "Error: Failed to parse data.inferenceService as JSON" >&2
        exit 1
    fi

    local current_list
    current_list=$(echo "$parsed_json" | jq '.serviceAnnotationDisallowedList // []')

    local annotations_to_add=()

    for annotation in "${ANNOTATIONS_TO_ADD[@]}"; do
        if echo "$current_list" | jq -e --arg ann "$annotation" 'index($ann) != null' > /dev/null 2>&1; then
            echo "Annotation '$annotation' already in disallowed list"
        else
            annotations_to_add+=("$annotation")
        fi
    done

    if [ ${#annotations_to_add[@]} -eq 0 ]; then
        echo "No annotations need to be added"
        return 0
    fi

    local new_list="$current_list"
    for annotation in "${annotations_to_add[@]}"; do
        new_list=$(echo "$new_list" | jq --arg ann "$annotation" '. + [$ann]')
    done

    local updated_json
    updated_json=$(echo "$parsed_json" | jq --argjson list "$new_list" '.serviceAnnotationDisallowedList = $list')

    local updated_json_string
    updated_json_string=$(echo "$updated_json" | jq '.')

    if [ "$DRY_RUN" == "true" ]; then
        echo "[DRY-RUN] Would add to disallowed list:"
        for annotation in "${annotations_to_add[@]}"; do
            echo "  - $annotation"
        done
        return 0
    fi

    local patch_json
    patch_json=$(jq -n --arg is "$updated_json_string" '{"data": {"inferenceService": $is}}')

    if ! oc patch configmap "$CONFIGMAP_NAME" -n "$NAMESPACE" --type=merge -p "$patch_json"; then
        echo "Error: Failed to patch ConfigMap" >&2
        exit 1
    fi

    echo "Added to disallowed list:"
    for annotation in "${annotations_to_add[@]}"; do
        echo "  - $annotation"
    done
}

restart_controller() {
    if [ "$DRY_RUN" == "true" ]; then
        echo "[DRY-RUN] Would restart kserve-controller-manager deployment"
        return 0
    fi

    echo "Restarting kserve-controller-manager deployment..."
    if ! oc rollout restart deployment kserve-controller-manager -n "$NAMESPACE"; then
        echo "Error: Failed to restart kserve-controller-manager deployment" >&2
        exit 1
    fi

    echo "Waiting for rollout to complete..."
    if ! oc rollout status deployment kserve-controller-manager -n "$NAMESPACE" --timeout=120s; then
        echo "Error: Rollout did not complete in time" >&2
        exit 1
    fi

    echo "Controller restarted successfully"
}

main() {
    parse_arguments "$@"
    validate_arguments
    check_prerequisites
    validate_namespace

    if [ "$DRY_RUN" == "true" ]; then
        echo "DRY-RUN MODE"
    fi

    get_configmap
    check_managed_annotation
    add_managed_annotation

    if [ "$DRY_RUN" != "true" ] && [ "$MANAGED_ANNOTATION_EXISTS" != "true" ]; then
        get_configmap
    fi

    update_disallowed_list

    restart_controller

    if [ "$DRY_RUN" == "true" ]; then
        echo "Dry run completed"
    else
        echo "Done"
    fi
}

main "$@"

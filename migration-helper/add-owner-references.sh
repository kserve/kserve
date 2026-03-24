#!/bin/bash

# Helper script to add ownerReferences to auth resources after applying them
# This is optional and only needed if you want automatic garbage collection

set -e

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

show_help() {
    cat << EOF
Add OwnerReferences Helper Script

DESCRIPTION:
    Adds ownerReferences to ServiceAccount, Role, and RoleBinding resources
    to enable automatic garbage collection when the InferenceService is deleted.
    
    This script should be run AFTER applying the converted resources.

USAGE:
    $0 -n <namespace> -i <inferenceservice-name>

OPTIONS:
    -n, --namespace NAMESPACE       Namespace containing the InferenceService
    -i, --isvc ISVC_NAME           InferenceService name
    -h, --help                      Show this help message

EXAMPLES:
    # Add ownerReferences for InferenceService 'my-model' in namespace 'models'
    $0 -n models -i my-model
    
    # Add ownerReferences for InferenceService 'advanced' in namespace 'testing'
    $0 -n testing -i advanced

EOF
    exit 0
}

# Parse arguments
NAMESPACE=""
ISVC_NAME=""

while [[ $# -gt 0 ]]; do
    case $1 in
        -n|--namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        -i|--isvc)
            ISVC_NAME="$2"
            shift 2
            ;;
        -h|--help)
            show_help
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Validate arguments
if [ -z "$NAMESPACE" ]; then
    echo -e "${RED}Error: Namespace is required${NC}"
    echo "Use --help for usage information"
    exit 1
fi

if [ -z "$ISVC_NAME" ]; then
    echo -e "${RED}Error: InferenceService name is required${NC}"
    echo "Use --help for usage information"
    exit 1
fi

echo -e "${CYAN}Adding ownerReferences for InferenceService: $ISVC_NAME in namespace: $NAMESPACE${NC}"
echo ""

# Get the InferenceService UID
ISVC_UID=$(oc get isvc "$ISVC_NAME" -n "$NAMESPACE" -o jsonpath='{.metadata.uid}' 2>/dev/null)
if [ -z "$ISVC_UID" ] || [ "$ISVC_UID" == "null" ]; then
    echo -e "${RED}Error: InferenceService '$ISVC_NAME' not found in namespace '$NAMESPACE'${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Found InferenceService with UID: $ISVC_UID${NC}"
echo ""

# Patch ServiceAccount
SA_NAME="${ISVC_NAME}-sa"
if oc get sa "$SA_NAME" -n "$NAMESPACE" &>/dev/null; then
    echo -e "Patching ServiceAccount: ${CYAN}$SA_NAME${NC}"
    oc patch sa "$SA_NAME" -n "$NAMESPACE" --type='json' -p="[
        {
            \"op\": \"add\",
            \"path\": \"/metadata/ownerReferences\",
            \"value\": [{
                \"apiVersion\": \"serving.kserve.io/v1beta1\",
                \"kind\": \"InferenceService\",
                \"name\": \"$ISVC_NAME\",
                \"uid\": \"$ISVC_UID\",
                \"blockOwnerDeletion\": false
            }]
        }
    ]" 2>/dev/null && echo -e "${GREEN}✓ ServiceAccount patched${NC}" || echo -e "${YELLOW}⚠ ServiceAccount might already have ownerReferences${NC}"
else
    echo -e "${YELLOW}⚠ ServiceAccount '$SA_NAME' not found${NC}"
fi
echo ""

# Patch Role
ROLE_NAME="${ISVC_NAME}-view-role"
if oc get role "$ROLE_NAME" -n "$NAMESPACE" &>/dev/null; then
    echo -e "Patching Role: ${CYAN}$ROLE_NAME${NC}"
    oc patch role "$ROLE_NAME" -n "$NAMESPACE" --type='json' -p="[
        {
            \"op\": \"add\",
            \"path\": \"/metadata/ownerReferences\",
            \"value\": [{
                \"apiVersion\": \"serving.kserve.io/v1beta1\",
                \"kind\": \"InferenceService\",
                \"name\": \"$ISVC_NAME\",
                \"uid\": \"$ISVC_UID\",
                \"blockOwnerDeletion\": false
            }]
        }
    ]" 2>/dev/null && echo -e "${GREEN}✓ Role patched${NC}" || echo -e "${YELLOW}⚠ Role might already have ownerReferences${NC}"
else
    echo -e "${YELLOW}⚠ Role '$ROLE_NAME' not found${NC}"
fi
echo ""

# Patch RoleBinding
ROLEBINDING_NAME="${ISVC_NAME}-view"
if oc get rolebinding "$ROLEBINDING_NAME" -n "$NAMESPACE" &>/dev/null; then
    echo -e "Patching RoleBinding: ${CYAN}$ROLEBINDING_NAME${NC}"
    oc patch rolebinding "$ROLEBINDING_NAME" -n "$NAMESPACE" --type='json' -p="[
        {
            \"op\": \"add\",
            \"path\": \"/metadata/ownerReferences\",
            \"value\": [{
                \"apiVersion\": \"serving.kserve.io/v1beta1\",
                \"kind\": \"InferenceService\",
                \"name\": \"$ISVC_NAME\",
                \"uid\": \"$ISVC_UID\",
                \"blockOwnerDeletion\": false
            }]
        }
    ]" 2>/dev/null && echo -e "${GREEN}✓ RoleBinding patched${NC}" || echo -e "${YELLOW}⚠ RoleBinding might already have ownerReferences${NC}"
else
    echo -e "${YELLOW}⚠ RoleBinding '$ROLEBINDING_NAME' not found${NC}"
fi
echo ""

echo -e "${GREEN}✓ Completed adding ownerReferences${NC}"
echo ""
echo -e "${YELLOW}Note: Auth resources will now be automatically deleted when the InferenceService is deleted${NC}"


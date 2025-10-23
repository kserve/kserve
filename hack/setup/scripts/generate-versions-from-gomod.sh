#!/bin/bash

# Copyright 2025 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Generate kserve-deps.env from go.mod
# This script only updates the auto-generated section between # START and # END
# Manual versions above # START are preserved

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"

source "${SCRIPT_DIR}/../common.sh"

REPO_ROOT="$(find_repo_root "${SCRIPT_DIR}")"
GO_MOD="${REPO_ROOT}/go.mod"
OUTPUT_FILE="${REPO_ROOT}/kserve-deps.env"

echo "Extracting versions from go.mod..."

# Extract version from go.mod with error handling
# Usage: extract_version <package_path> <remove_v_prefix>
# remove_v_prefix: "true" to remove leading 'v', "false" to keep it
extract_version() {
    local package="$1"
    local remove_v="${2:-false}"
    local version

    version=$(grep -E "^\s+${package} " "$GO_MOD" | awk '{print $2}' | head -1)

    if [ -z "$version" ]; then
        echo "Error: Failed to extract version for ${package}" >&2
        echo "Please check if the package exists in go.mod" >&2
        exit 1
    fi

    if [ "$remove_v" = "true" ]; then
        # Use bash parameter expansion (portable across BSD and GNU)
        echo "${version#v}"
    else
        echo "$version"
    fi
}

# Extract versions from go.mod
KEDA_VERSION=$(extract_version "github.com/kedacore/keda/v2" true)
ISTIO_VERSION=$(extract_version "istio.io/api" true)
GATEWAY_API_VERSION=$(extract_version "sigs.k8s.io/gateway-api" false)
LWS_VERSION=$(extract_version "sigs.k8s.io/lws" false)
GIE_VERSION=$(extract_version "sigs.k8s.io/gateway-api-inference-extension" false)
OTEL_VERSION=$(extract_version "github.com/open-telemetry/opentelemetry-operator" true)
KNATIVE_SERVING_VERSION=$(extract_version "knative.dev/serving" false)

# Create temp file with new auto-generated content
TEMP_FILE=$(mktemp)
cat > "$TEMP_FILE" <<EOF
# START
# Serverless dependencies
ISTIO_VERSION=${ISTIO_VERSION}
KNATIVE_SERVING_VERSION=${KNATIVE_SERVING_VERSION}

# KEDA dependencies
KEDA_VERSION=${KEDA_VERSION}
OPENTELEMETRY_OPERATOR_VERSION=${OTEL_VERSION}

# LLMISvc dependencies
LWS_VERSION=${LWS_VERSION}
GATEWAY_API_VERSION=${GATEWAY_API_VERSION}
GIE_VERSION=${GIE_VERSION}
# END
EOF

# Replace content between # START and # END (inclusive)
# Use portable sed that works on both BSD (Mac) and GNU (Linux)
if sed --version 2>&1 | grep -q GNU; then
    # GNU sed (Linux)
    sed -i '/# START/,/# END/{
        /# START/r '"$TEMP_FILE"'
        d
    }' "$OUTPUT_FILE"
else
    # BSD sed (Mac) - requires backup extension
    sed -i.bak '/# START/,/# END/{
        /# START/r '"$TEMP_FILE"'
        d
    }' "$OUTPUT_FILE"
    rm -f "${OUTPUT_FILE}.bak"
fi

rm "$TEMP_FILE"

echo "✅ Updated auto-generated section in $OUTPUT_FILE"
echo ""
echo "Updated from go.mod:"
echo "  ISTIO_VERSION=${ISTIO_VERSION}"
echo "  KEDA_VERSION=${KEDA_VERSION}"
echo "  GATEWAY_API_VERSION=${GATEWAY_API_VERSION}"
echo "  GIE_VERSION=${GIE_VERSION}"
echo "  LWS_VERSION=${LWS_VERSION}"
echo "  OPENTELEMETRY_OPERATOR_VERSION=${OTEL_VERSION}"
echo "  KNATIVE_SERVING_VERSION=${KNATIVE_SERVING_VERSION}"

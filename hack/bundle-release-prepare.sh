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

# Prepares the OLM bundle kustomize files for a release by replacing all
# :latest image tags with versioned tags (e.g. v0.17.0).
#
# Usage:
#   hack/bundle-release-prepare.sh <version>
#
# Example:
#   hack/bundle-release-prepare.sh 0.17.0
#
# After running this script, run `make bundle` to generate the bundle.

set -o errexit
set -o nounset
set -o pipefail

VERSION=${1:-}
if [ -z "${VERSION}" ]; then
    echo "Usage: $0 <version>" >&2
    echo "Example: $0 0.17.0" >&2
    exit 1
fi

TAG="v${VERSION}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
YQ="${YQ:-yq}"

echo "Preparing OLM bundle for version ${TAG}..."

# 1. Update controller image tags in the manifests kustomization.
echo "  Updating controller image tags in config/manifests/kustomization.yaml"
${YQ} -i "(.images[] | select(.name == \"kserve/kserve-controller\")).newTag = \"${TAG}\"" \
    "${REPO_ROOT}/config/manifests/kustomization.yaml"
${YQ} -i "(.images[] | select(.name == \"kserve/kserve-localmodel-controller\")).newTag = \"${TAG}\"" \
    "${REPO_ROOT}/config/manifests/kustomization.yaml"
${YQ} -i "(.images[] | select(.name == \"kserve/llmisvc-controller\")).newTag = \"${TAG}\"" \
    "${REPO_ROOT}/config/manifests/kustomization.yaml"

# 2. Update serving runtime image tags in the runtimes kustomization.
echo "  Updating serving runtime image tags in config/runtimes/kustomization.yaml"
for img in kserve-sklearnserver kserve-xgbserver kserve-pmmlserver kserve-paddleserver \
           kserve-lgbserver kserve-predictiveserver huggingfaceserver; do
    ${YQ} -i "(.images[] | select(.name == \"${img}\")).newTag = \"${TAG}\"" \
        "${REPO_ROOT}/config/runtimes/kustomization.yaml"
done
${YQ} -i "(.images[] | select(.name == \"huggingfaceserver-gpu\")).newTag = \"${TAG}-gpu\"" \
    "${REPO_ROOT}/config/runtimes/kustomization.yaml"

# 3. Update image tags in the inferenceservice configmap.
echo "  Updating image tags in config/configmap/inferenceservice.yaml"
sed -i.bak \
    -e "s|kserve/storage-initializer:latest|kserve/storage-initializer:${TAG}|g" \
    -e "s|kserve/agent:latest|kserve/agent:${TAG}|g" \
    -e "s|kserve/router:latest|kserve/router:${TAG}|g" \
    "${REPO_ROOT}/config/configmap/inferenceservice.yaml"
rm -f "${REPO_ROOT}/config/configmap/inferenceservice.yaml.bak"

# 4. Update image tags in the CSV base (containerImage annotation and alm-examples).
echo "  Updating image tags in config/manifests/bases/kserve.clusterserviceversion.yaml"
CSV_BASE="${REPO_ROOT}/config/manifests/bases/kserve.clusterserviceversion.yaml"
sed -i.bak \
    -e "s|kserve/kserve-controller:latest|kserve/kserve-controller:${TAG}|g" \
    -e "s|kserve/storage-initializer:latest|kserve/storage-initializer:${TAG}|g" \
    -e "s|kserve/sklearnserver:latest|kserve/sklearnserver:${TAG}|g" \
    "${CSV_BASE}"
rm -f "${CSV_BASE}.bak"

# 5. Update BUNDLE_VERSION in the Makefile.
echo "  Updating BUNDLE_VERSION in Makefile"
sed -i.bak "s|^BUNDLE_VERSION ?= .*|BUNDLE_VERSION ?= ${VERSION}|" "${REPO_ROOT}/Makefile"
rm -f "${REPO_ROOT}/Makefile.bak"

echo ""
echo "Done. All image tags updated to ${TAG}."
echo "Next steps:"
echo "  1. Run 'make bundle' to generate the bundle"
echo "  2. Verify with 'grep -rn :latest bundle/manifests/'"
echo "  3. Commit the changes"
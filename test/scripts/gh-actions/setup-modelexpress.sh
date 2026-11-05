#!/bin/bash

# Copyright 2026 The KServe Authors.
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

# Installs a ModelExpress server with the Kubernetes metadata backend into the
# current cluster and prints the address LLMInferenceServices should use.
#
# Env: MODELEXPRESS_VERSION  release tag of ai-dynamo/modelexpress (default: 0.6.0)
#      MODELEXPRESS_NS       namespace for the server (default: modelexpress)

set -o errexit
set -o nounset
set -o pipefail

MODELEXPRESS_VERSION="${MODELEXPRESS_VERSION:-0.6.0}"
MODELEXPRESS_NS="${MODELEXPRESS_NS:-modelexpress}"
RELEASE=modelexpress

CHART_DIR="$(mktemp -d)"
trap 'rm -rf "${CHART_DIR}"' EXIT

git -c advice.detachedHead=false clone --quiet --depth 1 --branch "v${MODELEXPRESS_VERSION}" --filter=blob:none --sparse \
  https://github.com/ai-dynamo/modelexpress.git "${CHART_DIR}" >&2
git -C "${CHART_DIR}" sparse-checkout set helm >&2

helm upgrade --install "${RELEASE}" "${CHART_DIR}/helm" \
  --namespace "${MODELEXPRESS_NS}" --create-namespace \
  --set image.tag="${MODELEXPRESS_VERSION}" \
  --set serviceAccount.rbac.enabled=true \
  --set env.MX_METADATA_BACKEND=kubernetes \
  --set securityContext.runAsUser=1000 \
  --set podSecurityContext.fsGroup=1000 \
  --wait --timeout 10m >&2

echo "${RELEASE}.${MODELEXPRESS_NS}.svc:8001"

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

# Turns on LLMInferenceService feature gates in the "llmisvc" key of the
# inferenceservice-config ConfigMap, keeping every other setting in that key.
#
# Usage: enable-llmisvc-feature-gates.sh <gate> [<gate> ...]
#   e.g. enable-llmisvc-feature-gates.sh disaggregatedSet

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]:-$0}")" &>/dev/null && pwd 2>/dev/null)"
source "${SCRIPT_DIR}/../../../hack/setup/common.sh"

if [[ $# -eq 0 ]]; then
  echo "Usage: $0 <gate> [<gate> ...]" >&2
  exit 1
fi

updates=()
for gate in "$@"; do
  updates+=("llmisvc.featureGates.${gate}=true")
done

update_isvc_config "${updates[@]}"

echo "llmisvc config in ${KSERVE_NAMESPACE}/inferenceservice-config:"
kubectl get configmap inferenceservice-config -n "${KSERVE_NAMESPACE}" -o jsonpath='{.data.llmisvc}'
echo

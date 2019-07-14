#!/usr/bin/env bash

# Copyright 2018 The Knative Authors
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

set -o errexit
set +x


: ${PROJECT_ID:="knative-environments"}
readonly PROJECT_ID
readonly K8S_CLUSTER_NAME=${1:?"First argument must be the kubernetes cluster name."}
readonly K8S_CLUSTER_REGION
readonly K8S_CLUSTER_ZONE
readonly SET_CLUSTER_CIDR=${SET_CLUSTER_CIDR:-true}

# Patch configmap `config-network` in `knative-serving` namespace.
function patch_network_config_gke() {
  local ip_ranges=""
  if [[ "$SET_CLUSTER_CIDR" == "true" ]]; then
    echo "Getting clusterIpv4Cidr from cluster ${K8S_CLUSTER_NAME}..."
    local cluster_cidr=$(gcloud container clusters describe "${K8S_CLUSTER_NAME}" \
      --project="${PROJECT_ID}" ${K8S_CLUSTER_REGION:+--region="${K8S_CLUSTER_REGION}"} \
      ${K8S_CLUSTER_ZONE:+--zone="${K8S_CLUSTER_ZONE}"} --format="value(clusterIpv4Cidr)")
    ip_ranges="${cluster_cidr},"
  fi
  echo "Getting servicesIpv4Cidr from cluster ${K8S_CLUSTER_NAME}..."
  local svc_cidr=$(gcloud container clusters describe "${K8S_CLUSTER_NAME}" \
    --project="${PROJECT_ID}" ${K8S_CLUSTER_REGION:+--region="${K8S_CLUSTER_REGION}"} \
    ${K8S_CLUSTER_ZONE:+--zone="${K8S_CLUSTER_ZONE}"} --format="value(servicesIpv4Cidr)")
  ip_ranges+="$svc_cidr"

  # Since kubectl doesn't really have "dry-run" mode, we get the file
  # temporarily and patch using "--local" strategy, for display purposes.
  readonly tmp=$(mktemp)
  kubectl get configmap config-network -n knative-serving -oyaml > $tmp

  echo "The following configuration will be applied:"
  readonly property="istio.sidecar.includeOutboundIPRanges"
  readonly patch="{\"data\":{\"${property}\":\"${ip_ranges}\"}}"
  readonly tmpl="{{index .data \"${property}\"}}"

  echo -n "  ${property}: "
  echo "$(kubectl patch --local -f ${tmp} -p ${patch} -o go-template="${tmpl}")"
  echo -e "\n"

  # Cleanup the temp file in any case.
  rm $tmp

  read -p "Do you want to apply the changes (y/N)? " -n 1 -r
  echo
  if [[ "$REPLY" =~ ^[Yy]$ ]]; then
    kubectl patch configmap config-network -n knative-serving -p "${patch}"
  else
    echo "No changes applied"
  fi
}

patch_network_config_gke

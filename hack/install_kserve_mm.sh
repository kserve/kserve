#!/bin/bash

# Copyright 2022 IBM Corporation
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
# limitations under the License.#

# Install KServe and ModelMesh into specified Kubernetes namespaces. KServe controller is
# deployed in "kserve" namespace, ModelMesh controller is in "modelmesh-serving" namespace,
# and ModelMesh is enabled for the optional user namespaces in the -u argument or the
# current namespace when -u is not used. All namespaces will be created if not exist.
# Expect cluster-admin authority and Kube cluster access to be configured, and git to be
# installed prior to running.
# This script can run anywhere and the repo branch variables can be changed to install
# different versions of KServe and ModelMesh.

set -e
############################################################
# Help                                                     #
############################################################
Help()
{
   # Display Help
   echo "Quick install script to deploy KServe and ModelMesh"
   echo
   echo "Syntax: [-u user_namespaces]"
   echo "options:"
   echo "  -u: user namespaces, such as \"ns_a ns_b\", to enable for ModelMesh"
   echo
}

export CTLR_NS="kserve"
export USER_NS=$(kubectl config  get-contexts $(kubectl config current-context) |tail -1|awk '{ print $5 }')
export user_ns_array=()
export C_DIR="$PWD"
export KSERVE_BRANCH="release-0.11"
export MMS_BRANCH="release-0.11"

while (($# > 0)); do
  case "$1" in
  -h | --help)
    Help
    exit
    ;;
  -u | --user_namespaces)
    shift
    user_ns_array=($1)
    ;;
  -*)
    echo "Unknown option: '${1}'"
    exit 10
    ;;
  esac
  shift
done

git clone -b $KSERVE_BRANCH --depth 1 --single-branch https://github.com/kserve/kserve.git
git clone -b $MMS_BRANCH --depth 1 --single-branch https://github.com/kserve/modelmesh-serving.git

cd "${C_DIR}/kserve"
./hack/quick_install.sh

kubectl create ns ${CTLR_NS} || true
cd "${C_DIR}/modelmesh-serving"
./scripts/install.sh -n ${CTLR_NS} --quickstart

if [[ ! -z $user_ns_array ]]; then
  for USER_NS in "${user_ns_array[@]}"; do
    kubectl create ns ${USER_NS} || true
    echo "Enabling ModelMesh for namespace: ${USER_NS}..."
    ./scripts/setup_user_namespaces.sh -u ${USER_NS} --create-storage-secret --deploy-serving-runtimes
  done
else
  echo "Enabling ModelMesh for namespace: ${USER_NS}..."
  ./scripts/setup_user_namespaces.sh -u ${USER_NS} --create-storage-secret --deploy-serving-runtimes
fi

cd "${C_DIR}"
rm -rf kserve
rm -rf modelmesh-serving

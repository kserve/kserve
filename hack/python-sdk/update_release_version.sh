#!/usr/bin/env bash

# Copyright 2023 The KServe Contributors
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

# This script updates the release version of python toml files.
# Usage: ./hack/python-sdk/update_release_version.sh [VERSION]

set -o errexit
set -o nounset
set -o pipefail

VERSION=$1
dirs=(
  "python/aiffairness"
  "python/aixexplainer"
  "python/alibiexplainer"
  "python/artexplainer"
  "python/custom_model"
  "python/custom_transformer"
  "python/lgbserver"
  "python/paddleserver"
  "python/pmmlserver"
  "python/sklearnserver"
  "python/xgbserver"
)

hack/python-sdk/update_python_release_version.py "${VERSION}"

# update lock files of packages which depends on kserve sdk
for dir in "${dirs[@]}"; do
  pushd "${dir}"
  poetry lock --no-update
  popd
done

#!/bin/bash

# Copyright 2023 The KServe Authors.
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

# This script will check if the lock file is up to date with pyproject.toml file.

set -o errexit
set -o nounset
set -o pipefail

YELLOW='\033[0;33m'
NC='\033[0m' # No color

cd ./python
packages=()

# Read the output of find into an array
while IFS= read -r -d '' folder; do
    packages+=("$folder")
done < <(find . -maxdepth 1 -type d -print0)

for folder in "${packages[@]}"
do
    echo "moving into folder ${folder}"
    if [[ ! -f "${folder}/pyproject.toml" ]]; then
        echo -e "${YELLOW}skipping folder ${folder}${NC}"
        continue
    fi
    pushd "${folder}" >> /dev/null
        poetry lock --check
    popd >> /dev/null
done

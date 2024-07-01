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

# This script will install poetry for GH Actions environment.

set -o errexit
set -o nounset
set -o pipefail

export POETRY_VERSION=1.7.1
echo "Installing Poetry $POETRY_VERSION ..."
pip install poetry==$POETRY_VERSION
poetry config virtualenvs.create true
poetry config virtualenvs.in-project true
poetry config installer.parallel true

echo "Installing Poetry Version Plugin"
pip install -e python/plugin/poetry-version-plugin
poetry self show plugins

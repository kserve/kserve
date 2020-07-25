#!/bin/bash

# Copyright 2019 The Kubeflow Authors.
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

# This shell script is used to run unit tests for python SDK.

set -o errexit
set -o nounset
set -o pipefail

echo "Installing requirement packages ..."
python3 -m pip install --upgrade pip
pip3 install --upgrade pytest
pip install --upgrade pytest-tornasync
pip3 install -r python/kfserving/requirements.txt

echo "Executing KFServing SDK testing ..."
pushd python/kfserving/test >/dev/null
  pytest --ignore=test_set_creds.py
popd

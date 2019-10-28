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

# Upgrade to python 3.6 to avoid errors in testing.
apt-get update -yqq
apt-get install -yqq --no-install-recommends software-properties-common
add-apt-repository -y ppa:jonathonf/python-3.6
apt-get update -yqq
apt-get install -yqq --no-install-recommends  python3.6 python3-pip
update-alternatives --install /usr/bin/python3 python3 /usr/bin/python3.5 1
update-alternatives --install /usr/bin/python3 python3 /usr/bin/python3.6 2

# Install requirement pacakges.
python3 -m pip install --upgrade pip
pip3 install --upgrade pytest
pip install --upgrade pytest-tornasync
pip3 install -r python/kfserving/requirements.txt

# Run KFServing SDK unit tests

for library in kfserving xgbserver sklearnserver pytorchserver alibiexplainer; do
  pushd python/$library >/dev/null
    pytest --ignore=test_set_creds.py
  popd
done

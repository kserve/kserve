#!/bin/bash

# Copyright 2020 The Kubeflow Authors.
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

echo "Upgrading to python 3.6 for testing ..."
apt-get update -yqq
apt-get install -y build-essential checkinstall >/dev/null
apt-get install -y libreadline-gplv2-dev libncursesw5-dev libssl-dev libsqlite3-dev tk-dev libgdbm-dev libc6-dev libbz2-dev >/dev/null
wget https://www.python.org/ftp/python/3.6.9/Python-3.6.9.tar.xz >/dev/null
tar xvf Python-3.6.9.tar.xz >/dev/null
pushd Python-3.6.9  >/dev/null
  ./configure >/dev/null
  make altinstall >/dev/null
popd

update-alternatives --install /usr/bin/python3 python3 /usr/bin/python3.5 1
update-alternatives --install /usr/bin/python3 python3 /usr/local/bin/python3.6 2
# Work around the issue https://github.com/pypa/pip/issues/4924
mv /usr/bin/lsb_release /usr/bin/lsb_release.bak

echo "Installing requirement pacakges ..."
python3 -m pip install --upgrade pip
pip3 install --upgrade pylint

python -m kubeflow.testing.test_py_lint --artifacts_dir=$1 --src_dir=$2

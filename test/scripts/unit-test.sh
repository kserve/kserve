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

# This shell script is used to build an image from our argo workflow
set -o nounset
set -o pipefail

# install kubebuilder before run test
arch=amd64
curl -L -O https://storage.googleapis.com/kubebuilder-release/kubebuilder_master_linux_${arch}.tar.gz
tar -zxvf kubebuilder_master_linux_${arch}.tar.gz
mv kubebuilder_master_linux_${arch} kubebuilder && mv kubebuilder /usr/local/
export PATH=$PATH:/usr/local/kubebuilder/bin:${GOPATH}/bin
GO_DIR=${GOPATH}/src/github.com/${REPO_OWNER}/${REPO_NAME}
mkdir -p ${GO_DIR}
cp -r ./* ${GO_DIR}
cd ${GO_DIR}
# Run unit and integration tests
n=0
until [ $n -ge 3 ]
do
  make test
  status=$?
  if [ $status -eq 0 ]; then
     echo "unit test run successfully"
     break
  fi
  n=$[$n+1]
  sleep 5
done
if [ $status -ne 0 ]; then
   echo "tried 3 times, marking unit test failed"
   exit 1
fi

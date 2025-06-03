#!/usr/bin/env bash
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This is a helper script to generate custom tls certificates.
set -eu

MY_PATH=$(dirname "$0")
PROJECT_ROOT=$MY_PATH/../../../../
TLS_DIR=$PROJECT_ROOT/test/scripts/openshift-ci/tls

if [ ! -x "$(command -v openssl)" ]; then
  echo "openssl not found"
  exit 1
fi

echo "Generating Custom CA cert and secret"
if ! [ -d $TLS_DIR/certs/custom ]; then
    mkdir -p $TLS_DIR/certs/custom
fi
openssl req -x509 -sha256 -nodes -days 365 -newkey rsa:4096 -subj "/O=Red Hat/CN=root" -keyout ${TLS_DIR}/certs/custom/root.key -out ${TLS_DIR}/certs/custom/root.crt 
openssl req -nodes --newkey rsa:4096 -subj "/CN=custom/O=Red Hat" --keyout ${TLS_DIR}/certs/custom/custom.key -out ${TLS_DIR}/certs/custom/custom.csr -config ${TLS_DIR}/openssl-san.config
openssl x509 -req -in ${TLS_DIR}/certs/custom/custom.csr -CA ${TLS_DIR}/certs/custom/root.crt -CAkey ${TLS_DIR}/certs/custom/root.key -CAcreateserial -out ${TLS_DIR}/certs/custom/custom.crt -days 365 -sha256 -extfile ${TLS_DIR}/openssl-san.config -extensions v3_req

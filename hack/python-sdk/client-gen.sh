#!/usr/bin/env bash

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

# Note: Need to merge /python/kfserving/README.md and some documents manually after the script execution.

set -o errexit
set -o nounset

SWAGGER_JAR_URL="http://search.maven.org/maven2/io/swagger/swagger-codegen-cli/2.4.6/swagger-codegen-cli-2.4.6.jar"
SWAGGER_CODEGEN_JAR="hack/python-sdk/swagger-codegen-cli.jar"
SWAGGER_CODEGEN_CONF="hack/python-sdk/swagger_config.json"
SWAGGER_CODEGEN_FILE="pkg/apis/serving/v1alpha2/swagger.json"
SDK_OUTPUT_PATH="python/kfserving"

echo "Downloading the swagger-codegen JAR package ..."
if [ ! -f ${SWAGGER_CODEGEN_JAR} ]
then
    wget -O ${SWAGGER_CODEGEN_JAR} ${SWAGGER_JAR_URL}
fi

echo "Generating Python SDK for KFServing ..."
java -jar ${SWAGGER_CODEGEN_JAR} generate -i ${SWAGGER_CODEGEN_FILE} -l python -o ${SDK_OUTPUT_PATH} -c ${SWAGGER_CODEGEN_CONF}

# revert following files since they are diveraged from generated ones
git checkout python/kfserving/kfserving/rest.py
git checkout python/kfserving/README.md
git checkout python/kfserving/kfserving/__init__.py
git checkout python/kfserving/test/__init__.py
git checkout python/kfserving/setup.py
git checkout python/kfserving/requirements.txt

hack/boilerplate.sh
echo "KFServing Python SDK is generated successfully to folder ${SDK_OUTPUT_PATH}/."

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

set -o errexit
set -o nounset

SWAGGER_JAR_URL="http://central.maven.org/maven2/io/swagger/swagger-codegen-cli/2.4.6/swagger-codegen-cli-2.4.6.jar"
SWAGGER_CODEGEN_JAR="hack/sdk-gen/swagger-codegen-cli.jar"
SWAGGER_CODEGEN_CONF="hack/sdk-gen/swagger_config.json"
SWAGGER_CODEGEN_FILE="pkg/apis/serving/v1alpha1/swagger.json"
SDK_OUTPUT_PATH="python/kfserving"

echo "Downloading the swagger-codegen JAR package ..."
wget -O ${SWAGGER_CODEGEN_JAR} ${SWAGGER_JAR_URL}

echo "Generating Python SDK for KFServing ..."
java -jar ${SWAGGER_CODEGEN_JAR} generate -i ${SWAGGER_CODEGEN_FILE} -l python -o ${SDK_OUTPUT_PATH} -c ${SWAGGER_CODEGEN_CONF}

echo "KFServing Python SDK is generated successfully to folder ${SDK_OUTPUT_PATH}/."

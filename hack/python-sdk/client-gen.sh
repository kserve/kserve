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

OPENAPI_GENERATOR_VERSION="4.3.1"
SWAGGER_JAR_URL="https://repo1.maven.org/maven2/org/openapitools/openapi-generator-cli/${OPENAPI_GENERATOR_VERSION}/openapi-generator-cli-${OPENAPI_GENERATOR_VERSION}.jar"
SWAGGER_CODEGEN_JAR="hack/python-sdk/openapi-generator-cli-${OPENAPI_GENERATOR_VERSION}.jar"
SWAGGER_CODEGEN_CONF="hack/python-sdk/swagger_config.json"
SWAGGER_CODEGEN_FILE="pkg/openapi/swagger.json"
SDK_OUTPUT_PATH="python/kserve"

echo "Downloading the swagger-codegen JAR package ..."
if [ ! -f ${SWAGGER_CODEGEN_JAR} ]
then
    wget -O ${SWAGGER_CODEGEN_JAR} ${SWAGGER_JAR_URL}
fi

echo "Generating Python SDK for KServe ..."
java -jar ${SWAGGER_CODEGEN_JAR} generate -i ${SWAGGER_CODEGEN_FILE} -g python -o ${SDK_OUTPUT_PATH} -c ${SWAGGER_CODEGEN_CONF}

# Update kubernetes docs link.
K8S_IMPORT_LIST=$(cat hack/python-sdk/swagger_config.json|grep "V1" | awk -F"\"" '{print $2}')
K8S_DOC_LINK="https://github.com/kubernetes-client/python/blob/master/kubernetes/docs"
for item in $K8S_IMPORT_LIST; do
    sed -i'.bak' -e "s@($item.md)@($K8S_DOC_LINK/$item.md)@g" python/kserve/docs/*
    rm -rf python/kserve/docs/*.bak
done

hack/boilerplate.sh
echo "KServe Python SDK is generated successfully to folder ${SDK_OUTPUT_PATH}/."

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
# Update together with the version. Maven Central publishes only a sha1 for 4.3.1; this is
# the sha256 of the artifact whose sha1 matches it.
SWAGGER_JAR_SHA256="f438cd16bc1db28d3363e314cefb59384f252361db9cb1a04a322e7eb5b331c1"
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
# Checked on every run, not only after a download: CI restores the JAR from the Actions cache.
if command -v sha256sum >/dev/null 2>&1; then
    echo "${SWAGGER_JAR_SHA256}  ${SWAGGER_CODEGEN_JAR}" | sha256sum -c -
else
    echo "${SWAGGER_JAR_SHA256}  ${SWAGGER_CODEGEN_JAR}" | shasum -a 256 -c -
fi

echo "Generating Python SDK for KServe ..."
java -jar ${SWAGGER_CODEGEN_JAR} generate -i ${SWAGGER_CODEGEN_FILE} -g python -o ${SDK_OUTPUT_PATH} -c ${SWAGGER_CODEGEN_CONF}

# Preserve the positional constructor contract that predates the LLM TLS fields.
# New fields are keyword-only so generated clients cannot silently rebind existing
# positional arguments when this model grows.
INGRESS_CONFIG_MODEL="${SDK_OUTPUT_PATH}/kserve/models/v1beta1_ingress_config.py"
INGRESS_TLS_KWONLY=", *, llm_inference_service_tls_cipher_suites=None, llm_inference_service_tls_min_version=None):"
sed -i'.bak' -e '/def __init__(.*, local_vars_configuration=None):/{
s/, llm_inference_service_tls_cipher_suites=None,/,/
s/, llm_inference_service_tls_min_version=None,/,/
s/, local_vars_configuration=None):/, local_vars_configuration=None'"${INGRESS_TLS_KWONLY}"'/
}' "${INGRESS_CONFIG_MODEL}"
rm -f "${INGRESS_CONFIG_MODEL}.bak"
grep -qF "local_vars_configuration=None${INGRESS_TLS_KWONLY}" "${INGRESS_CONFIG_MODEL}" || {
    echo "client-gen: keyword-only patch did not apply to ${INGRESS_CONFIG_MODEL}" >&2
    exit 1
}

# Fix openapi-generator 4.3.1 bug: model references with dots in swagger definition
# names (e.g. "v1alpha2.LLMInferenceService") are emitted as broken Python expressions
# like "kserve.models.v1alpha2/llm_inference_service.v1alpha2.LLMInferenceService("
# instead of "kserve.models.v1alpha2_llm_inference_service.V1alpha2LLMInferenceService(".
echo "Fixing malformed model references in generated test files ..."
find ${SDK_OUTPUT_PATH}/test -name '*.py' -exec \
    sed -i'.bak' -E 's|kserve\.models\.(v[0-9a-z]+)/([a-z_]+)\.\1\.([A-Za-z]+)\(|kserve.models.\1_\2.\u\1\3(|g' {} + && \
    find ${SDK_OUTPUT_PATH}/test -name '*.py.bak' -delete

# Update kubernetes docs link.
K8S_IMPORT_LIST=$(cat hack/python-sdk/swagger_config.json|grep "V1" | awk -F"\"" '{print $2}')
K8S_DOC_LINK="https://github.com/kubernetes-client/python/blob/master/kubernetes/docs"
for item in $K8S_IMPORT_LIST; do
    sed -i'.bak' -e "s@($item.md)@($K8S_DOC_LINK/$item.md)@g" python/kserve/docs/*
    rm -rf python/kserve/docs/*.bak
done

if [ -n "${COPYRIGHT_YEARS_CACHE:-}" ]; then
    hack/boilerplate.sh --year-cache "${COPYRIGHT_YEARS_CACHE}"
else
    hack/boilerplate.sh
fi
echo "KServe Python SDK is generated successfully to folder ${SDK_OUTPUT_PATH}/."

#!/usr/bin/env bash

SDK_GEN_PATH="hack/sdk-gen"
SDK_OUTPUT_PATH="sdk"
SWAGGER_JAR_URL="http://central.maven.org/maven2/io/swagger/swagger-codegen-cli/2.4.6/swagger-codegen-cli-2.4.6.jar"
SWAGGER_CODEGEN_JAR="${SDK_GEN_PATH}/swagger-codegen-cli.jar"
SWAGGER_CODEGEN_CONF="${SDK_GEN_PATH}/swagger_config.json"
SWAGGER_CODEGEN_FILE="pkg/apis/serving/v1alpha1/swagger.json"

echo "Downloading the swagger-codegen JAR package ..."
wget -O ${SWAGGER_CODEGEN_JAR} ${SWAGGER_JAR_URL} >> /dev/null 2>&1
if [ $? -ne 0 ]; then
    echo "ERROR: Failed to download swagger-codege jar pacakge."
    exit 1
fi

echo "Generating Python SDK for KFServing ..."
if which java >/dev/null 2>&1; then
    java -jar ${SWAGGER_CODEGEN_JAR} generate -i ${SWAGGER_CODEGEN_FILE} -l python -o ${SDK_OUTPUT_PATH} -c ${SWAGGER_CODEGEN_CONF}
    if [ $? -ne 0 ]; then
        echo "ERROR: Failed to generate KFServing Python SDK."
        exit 1
    fi
else
    echo "ERROR: No java command found. Intall java and try again."
    exit 1
fi

echo "KFServing Python SDK is generated successfully to folder ${SDK_OUTPUT_PATH}/."

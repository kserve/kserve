#!/bin/bash
# Auto-generated from kserve-images.env - DO NOT EDIT MANUALLY

# E2E environment variables
export DOCKER_IMAGES_PATH="${DOCKER_IMAGES_PATH:-/tmp/docker-images}"

# Image URL to use all building/pushing image targets
export TAG="${TAG:-latest}"
export CONTROLLER_IMG="${CONTROLLER_IMG:-kserve-controller}"
export AGENT_IMG="${AGENT_IMG:-agent}"
export ROUTER_IMG="${ROUTER_IMG:-router}"
export SKLEARN_IMG="${SKLEARN_IMG:-sklearnserver}"
export XGB_IMG="${XGB_IMG:-xgbserver}"
export LGB_IMG="${LGB_IMG:-lgbserver}"
export PREDICTIVE_IMG="${PREDICTIVE_IMG:-predictiveserver}"
export PMML_IMG="${PMML_IMG:-pmmlserver}"
export PADDLE_IMG="${PADDLE_IMG:-paddleserver}"
export CUSTOM_MODEL_IMG="${CUSTOM_MODEL_IMG:-custom-model}"
export CUSTOM_MODEL_GRPC_IMG="${CUSTOM_MODEL_GRPC_IMG:-custom-model-grpc}"
export CUSTOM_MODEL_GRPC_IMG_TAG="${CUSTOM_MODEL_GRPC_IMG_TAG:-${KO_DOCKER_REPO}/${CUSTOM_MODEL_GRPC_IMG}:${TAG}}"
export IMAGE_TRANSFORMER_IMG="${IMAGE_TRANSFORMER_IMG:-image-transformer}"
export IMAGE_TRANSFORMER_IMG_TAG="${IMAGE_TRANSFORMER_IMG_TAG:-${KO_DOCKER_REPO}/${IMAGE_TRANSFORMER_IMG}:${TAG}}"
export CUSTOM_TRANSFORMER_GRPC_IMG="${CUSTOM_TRANSFORMER_GRPC_IMG:-custom-image-transformer-grpc}"
export HUGGINGFACE_IMG="${HUGGINGFACE_IMG:-huggingfaceserver}"
export HUGGINGFACE_SERVER_CPU_IMG="${HUGGINGFACE_SERVER_CPU_IMG:-huggingfaceserver-cpu}"
export AIF_IMG="${AIF_IMG:-aiffairness}"
export ART_IMG="${ART_IMG:-art-explainer}"
export STORAGE_INIT_IMG="${STORAGE_INIT_IMG:-storage-initializer}"
export QPEXT_IMG="${QPEXT_IMG:-qpext}"
export SUCCESS_200_ISVC_IMG="${SUCCESS_200_ISVC_IMG:-success-200-isvc}"
export ERROR_404_ISVC_IMG="${ERROR_404_ISVC_IMG:-error-404-isvc}"
export LLMISVC_CONTROLLER_IMG="${LLMISVC_CONTROLLER_IMG:-llmisvc-controller}"
export LOCALMODEL_CONTROLLER_IMG="${LOCALMODEL_CONTROLLER_IMG:-kserve-localmodel-controller}"
export LOCALMODEL_AGENT_IMG="${LOCALMODEL_AGENT_IMG:-kserve-localmodelnode-agent}"

# CI mode: export all variables to GITHUB_ENV
if [[ "${1:-}" == "--ci" ]]; then
  for var in DOCKER_IMAGES_PATH TAG CONTROLLER_IMG AGENT_IMG ROUTER_IMG SKLEARN_IMG XGB_IMG LGB_IMG PREDICTIVE_IMG PMML_IMG PADDLE_IMG CUSTOM_MODEL_IMG CUSTOM_MODEL_GRPC_IMG CUSTOM_MODEL_GRPC_IMG_TAG IMAGE_TRANSFORMER_IMG IMAGE_TRANSFORMER_IMG_TAG CUSTOM_TRANSFORMER_GRPC_IMG HUGGINGFACE_IMG HUGGINGFACE_SERVER_CPU_IMG AIF_IMG ART_IMG STORAGE_INIT_IMG QPEXT_IMG SUCCESS_200_ISVC_IMG ERROR_404_ISVC_IMG LLMISVC_CONTROLLER_IMG LOCALMODEL_CONTROLLER_IMG LOCALMODEL_AGENT_IMG; do
    echo "${var}=${!var}" >> $GITHUB_ENV
  done
  echo "✅ Exported KServe image variables to GITHUB_ENV"
fi

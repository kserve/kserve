#!/bin/bash
# Usage: image_patch_dev.sh [OVERLAY]
set -u
set -e
OVERLAY=$1
IMG=$(ko resolve -f config/manager/manager.yaml | grep 'image:' | awk '{print $2}')
if [ -z ${IMG} ]; then exit; fi
cat > config/overlays/${OVERLAY}/manager_image_patch.yaml << EOF
apiVersion: apps/v1
kind: StatefulSet 
metadata:
  name: kfserving-controller-manager
  namespace: kfserving-system
spec:
  template:
    spec:
      containers:
        - name: manager
          command:
          image: ${IMG}
EOF

LOGGER_IMG=$(ko resolve -f config/overlays/development/configmap/ko_resolve_logger| grep 'image:' | awk '{print $2}')
if [ -z ${LOGGER_IMG} ]; then exit; fi

BATCHER_IMG=$(ko resolve -f config/overlays/development/configmap/ko_resolve_batcher| grep 'image:' | awk '{print $2}')
if [ -z ${BATCHER_IMG} ]; then exit; fi

AGENT_IMG=$(ko resolve -f config/overlays/development/configmap/ko_resolve_agent| grep 'image:' | awk '{print $2}')
if [ -z ${AGENT_IMG} ]; then exit; fi

cat > config/overlays/${OVERLAY}/configmap/inferenceservice_patch.yaml << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: inferenceservice-config
  namespace: kfserving-system
data:
  logger: |-
    {
        "image" : "${LOGGER_IMG}",
        "memoryRequest": "100Mi",
        "memoryLimit": "1Gi",
        "cpuRequest": "100m",
        "cpuLimit": "1"
    }
  batcher: |-
    {
        "image" : "${BATCHER_IMG}",
        "memoryRequest": "100Mi",
        "memoryLimit": "1Gi",
        "cpuRequest": "100m",
        "cpuLimit": "1"
    }
  agent: |-
    {
        "image" : "${AGENT_IMG}",
        "memoryRequest": "100Mi",
        "memoryLimit": "1Gi",
        "cpuRequest": "100m",
        "cpuLimit": "1"
    }
  predictors: |-
    {
        "tensorflow": {
            "image": "tensorflow/serving",
            "defaultImageVersion": "1.14.0",
            "defaultGpuImageVersion": "1.14.0-gpu",
            "supportedFrameworks": [
              "tensorflow"
            ],
            "multiModelServer": "true"
        },
        "onnx": {
            "image": "mcr.microsoft.com/onnxruntime/server",
            "defaultImageVersion": "v1.0.0",
            "supportedFrameworks": [
              "onnx"
            ],
            "multiModelServer": "false"
        },
        "sklearn": {
          "v1": {
            "image": "yuzhui/sklearnserver",
            "defaultImageVersion": "fixstorage3",
            "supportedFrameworks": [
              "sklearn"
            ],
            "multiModelServer": "false"
          },
          "v2": {
            "image": "docker.io/seldonio/mlserver",
            "defaultImageVersion": "0.1.2",
            "supportedFrameworks": [
              "sklearn"
            ],
            "multiModelServer": "false"
          }
        },
        "xgboost": {
            "image": "gcr.io/kfserving/xgbserver",
            "defaultImageVersion": "v0.4.1",
            "supportedFrameworks": [
              "xgboost"
            ],
            "multiModelServer": "false"
        },
        "pytorch": {
            "image": "gcr.io/kfserving/pytorchserver",
            "defaultImageVersion": "v0.4.1",
            "defaultGpuImageVersion": "v0.4.1-gpu",
            "supportedFrameworks": [
              "pytorch"
            ],
            "multiModelServer": "false"
        },
        "triton": {
            "image": "nvcr.io/nvidia/tritonserver",
            "defaultImageVersion": "20.08-py3",
            "supportedFrameworks": [
              "tensorrt",
              "tensorflow",
              "onnx",
              "pytorch",
              "caffe2"
            ],
            "multiModelServer": "true"
        }
    }

EOF

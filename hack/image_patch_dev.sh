#!/bin/bash
# Usage: image_patch_dev.sh [OVERLAY]
set -u
set -e
OVERLAY=$1
IMG=$(ko resolve -f config/manager/manager.yaml | grep 'image:' | awk '{print $2}')
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
EOF

BATCHER_IMG=$(ko resolve -f config/overlays/development/configmap/ko_resolve_batcher| grep 'image:' | awk '{print $2}')
if [ -z ${BATCHER_IMG} ]; then exit; fi
cat > config/overlays/${OVERLAY}/configmap/inferenceservice_patch.yaml << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: inferenceservice-config
  namespace: kfserving-system
data:
  batcher: |-
    {
        "image" : "${BATCHER_IMG}",
        "memoryRequest": "100Mi",
        "memoryLimit": "1Gi",
        "cpuRequest": "100m",
        "cpuLimit": "1"
    }
EOF

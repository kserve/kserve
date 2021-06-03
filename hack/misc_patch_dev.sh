#!/bin/bash
set -u
CONFIG_NAME=$1
IMG=$2
if [ -z ${IMG} -o -z ${CONFIG_NAME} ]; then exit; fi
cat > config/overlays/dev-image-config/inferenceservice_patch.yaml << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: inferenceservice-config
  namespace: kfserving-system
data:
    ${CONFIG_NAME}: |-
      {
        "image" : "${IMG}",
        "memoryRequest": "100Mi",
        "memoryLimit": "1Gi",
        "cpuRequest": "100m",
        "cpuLimit": "1"
      }
EOF

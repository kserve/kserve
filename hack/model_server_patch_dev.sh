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
  namespace: kserve
data:
  predictors: |-
    {
      "${CONFIG_NAME}": {
          "image" : "${IMG}",
          "defaultImageVersion": "latest",
          "allowedImageVersions": [
               "latest"
            ]
      }
    }
EOF

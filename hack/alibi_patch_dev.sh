#!/bin/bash
# Usage: image_patch_dev.sh [OVERLAY]
set -u
IMG=$1
if [ -z ${IMG} ]; then exit; fi
cat > config/overlays/dev-image-config/inferenceservice_patch.yaml << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: inferenceservice-config
  namespace: kserve
data:
  explainers: |-
    {
        "alibi": {
          "image" : "${IMG}",
          "defaultImageVersion": "latest",
          "allowedImageVersions": [
               "latest"
            ]
        }
    }
EOF

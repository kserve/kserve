#!/bin/bash
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
    storageInitializer: |-
      {
        "image" : "${IMG}",
        "memoryRequest": "100Mi",
        "memoryLimit": "1Gi",
        "cpuRequest": "100m",
        "cpuLimit": "1"
      }
EOF

#!/bin/bash
# Usage: image_patch_dev.sh [OVERLAY]

OVERLAY=$1
IMG=$(ko resolve -f config/default/manager/manager.yaml | grep 'image:' | awk '{print $2}')
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

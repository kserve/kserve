#!/bin/bash
IMG=$(ko resolve -f config/default/manager/manager.yaml | grep 'image:' | awk '{print $2}')
if [ -z ${IMG} ]; then exit; fi
cat > config/overlays/development/manager_image_patch.yaml << EOF 
apiVersion: apps/v1
kind: StatefulSet 
metadata:
  name: controller-manager
spec:
  template:
    spec:
      containers:
        - name: manager
          command:
          image: ${IMG}
EOF

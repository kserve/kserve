#!/bin/bash
IMG=$(ko resolve -f config/manager/manager.yaml | grep 'image:' | awk '{print $2}')
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

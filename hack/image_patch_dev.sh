#!/bin/bash
# Usage: image_patch_dev.sh [OVERLAY]
set -u
set -e
set -o pipefail

OVERLAY=$1
IMG=$(ko resolve -f config/manager/manager.yaml | grep 'image:' | head -1 | awk '{print $2}')
if [ -z ${IMG} ]; then exit; fi
cat > config/overlays/${OVERLAY}/manager_image_patch.yaml << EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kserve-controller-manager
  namespace: kserve
spec:
  template:
    spec:
      containers:
        - name: manager
          command:
            - /ko-app/manager
          image: ${IMG}
EOF

IMG=$(ko resolve -f config/localmodels/manager.yaml | grep 'image:' | head -1 | awk '{print $2}')
if [ -z ${IMG} ]; then exit; fi
cat > config/overlays/${OVERLAY}/localmodel_image_patch.yaml << EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kserve-localmodel-controller-manager
  namespace: kserve
spec:
  template:
    spec:
      containers:
        - name: manager
          command:
            - /ko-app/localmodel
          image: ${IMG}
EOF

IMG=$(ko resolve -f config/localmodelnodes/manager.yaml | grep 'image:' | head -1 | awk '{print $2}')
if [ -z ${IMG} ]; then exit; fi
cat > config/overlays/${OVERLAY}/localmodelnode_image_patch.yaml << EOF
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: kserve-localmodelnode-agent
  namespace: kserve
spec:
  template:
    spec:
      containers:
        - name: manager
          command:
            - /ko-app/localmodelnode
          image: ${IMG}
EOF

AGENT_IMG=$(ko resolve -f config/overlays/development/configmap/ko_resolve_agent| grep 'image:' | awk '{print $2}')
ROUTER_IMG=$(ko resolve -f config/overlays/development/configmap/ko_resolve_router| grep 'image:' | awk '{print $2}')

if [ -z ${AGENT_IMG} ]; then exit; fi

cat > config/overlays/${OVERLAY}/configmap/inferenceservice_patch.yaml << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: inferenceservice-config
  namespace: kserve
data:
  logger: |-
    {
        "image" : "${AGENT_IMG}",
        "memoryRequest": "100Mi",
        "memoryLimit": "100Mi",
        "cpuRequest": "100m",
        "cpuLimit": "100m"
    }
  batcher: |-
    {
        "image" : "${AGENT_IMG}",
        "memoryRequest": "100Mi",
        "memoryLimit": "100Mi",
        "cpuRequest": "100m",
        "cpuLimit": "100m"
    }
  agent: |-
    {
        "image" : "${AGENT_IMG}",
        "memoryRequest": "100Mi",
        "memoryLimit": "500Mi",
        "cpuRequest": "100m",
        "cpuLimit": "100m"
    }
  router: |-
    {
        "image" : "${ROUTER_IMG}",
        "memoryRequest": "100Mi",
        "memoryLimit": "500Mi",
        "cpuRequest": "100m",
        "cpuLimit": "100m"
    }
  metricsAggregator: |-
    {
        "enableMetricAggregation": "false",
        "enablePrometheusScraping" : "false"
    }
EOF

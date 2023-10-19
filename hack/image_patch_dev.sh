#!/bin/bash
# Usage: image_patch_dev.sh [OVERLAY]
set -u
set -e
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
cat >> config/overlays/${OVERLAY}/configmap/inferenceservice_patch.yaml << EOF
  ingress: |-
    {
        "ingressGateway" : "knative-serving/knative-ingress-gateway",
        "ingressService" : "istio-ingressgateway.istio-system.svc.cluster.local",
        "localGateway" : "knative-serving/knative-local-gateway",
        "localGatewayService" : "knative-local-gateway.istio-system.svc.cluster.local",
        "ingressDomain"  : "svc.cluster.local",
        "ingressClassName" : "istio",
        "domainTemplate": "{{ .Name }}-{{ .Namespace }}.{{ .IngressDomain }}",
        "urlScheme": "http",
        "disableIstioVirtualHost": false
    }
  deploy: |-
    {
      "defaultDeploymentMode": "RawDeployment"
    }
EOF

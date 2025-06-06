#!/bin/bash

# This script sets up the Envoy AI Gateway for GitHub Actions.

set -euo pipefail

ENVOY_AIGW_VERSION="v0.1.5"

# Install Envoy AI Gateway
helm upgrade -i aieg oci://docker.io/envoyproxy/ai-gateway-helm \
    --version $ENVOY_AIGW_VERSION \
    --namespace envoy-ai-gateway-system \
    --create-namespace

kubectl wait --timeout=2m -n envoy-ai-gateway-system deployment/ai-gateway-controller --for=condition=Available

# Configure Envoy Gateway
# Note: Envoy Gateway is already installed by the dependency setup action.
kubectl apply -f https://raw.githubusercontent.com/envoyproxy/ai-gateway/refs/tags/v0.1.5/manifests/envoy-gateway-config/redis.yaml
kubectl apply -f https://raw.githubusercontent.com/envoyproxy/ai-gateway/refs/tags/v0.1.5/manifests/envoy-gateway-config/config.yaml
kubectl apply -f https://raw.githubusercontent.com/envoyproxy/ai-gateway/refs/tags/v0.1.5/manifests/envoy-gateway-config/rbac.yaml

kubectl rollout restart -n envoy-gateway-system deployment/envoy-gateway

kubectl wait --timeout=2m -n envoy-gateway-system deployment/envoy-gateway --for=condition=Available

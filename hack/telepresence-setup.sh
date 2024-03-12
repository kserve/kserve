#!/bin/bash

# Script for setting up telepresence to run the KServe controller
# locally, fully connected to the cluster (it works also for the
# incoming direction when used as a webhook)
#
# Usage:
#        # Install everything. This command is idempotent and can
#        # be called multiple times
#        ./telepresence-setup.sh
#
#        # Remove everything from telepresence
#        ./telepresence-setup.sh uninstall
#
# More information for using telepresence for developing:
#
# * https://codefresh.io/blog/telepresence-2-local-development/
# * https://ttt.io/kubernetes-admission-controller

set -o pipefail
set +e

if [[ "$1" == "uninstall" ]]; then
  telepresence helm uninstall
  kubectl delete ns ambassador
  telepresence quit -s
  [ -d "${TMPDIR}/k8s-webhook-server" ] && rm -rf "${TMPDIR}/k8s-webhook-server"
  exit 0
fi

if ! type telepresence >/dev/null 2>&1; then
  echo "Telepresence is not installed."
  echo "Please install it, e.g. with \"brew install telepresence\" for Mac"
  echo "or download it from https://www.getambassador.io/docs/telepresence/latest/install for all OS"
  exit 1
fi

# Check if cluster is already setup for telepresence and install it if not
if ! kubectl get -n ambassador deploy traffic-manager >/dev/null 2>&1; then
  echo "* Installing Telepresence in cluster"
  set -e
  telepresence helm install
  set +e
else
  echo "* Telepresence already installed in cluster"
fi

# Connect to the cluster
set -e
echo "* Connecting to cluster (local root password might be required)"
telepresence connect
set +e

# Intercept the kserve controller manager
if ! telepresence status --output json | jq -e '.user_daemon.intercepts[]? | select(.name == "kserve-controller-manager-kserve")' > /dev/null; then
  echo "* Intercept kserve-webhook-server-service"
  set -e
  telepresence intercept kserve-controller-manager --service=kserve-webhook-server-service --port 9443 -n kserve --mount=false
  set +e
else
  echo "* Webhook service already intercepted"
fi

# Copy of certs and ca from secret so that controller manager can run locally
# Get secret in JSON format
set -e
secret_json=$(kubectl get secret kserve-webhook-server-cert -n kserve -o json)
target_dir="${TMPDIR}/k8s-webhook-server/serving-certs"
echo "* Extracting Webhook certs to $target_dir"
mkdir -p $target_dir
for key in $(echo "$secret_json" | jq -r '.data | keys[]'); do
  # Decode value
  value=$(echo "$secret_json" | jq -r ".data[\"$key\"]" | base64 -d)
  echo "$value" > "$target_dir/$key"
done

cat <<EOF
===============================
Telepresence up and ready.
You can run and debug cmd/manager/main.go locally now. 
EOF

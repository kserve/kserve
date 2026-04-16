#!/bin/bash
# Patches the confidentialImage field in the inferenceservice-config ConfigMap.
# Usage: hack/patch-confidential-image.sh <image>
# Example: hack/patch-confidential-image.sh <registry>/<user>/storage-initializer-confidential:latest

set -o nounset
set -o errexit
set -o pipefail

NAMESPACE="${KSERVE_NAMESPACE:-kserve}"
IMAGE="${1:?Usage: $0 <image>}"

kubectl get configmap inferenceservice-config -n "${NAMESPACE}" -o json | \
  python3 -c "
import json, sys
cm = json.load(sys.stdin)
si = json.loads(cm['data']['storageInitializer'])
si['confidentialImage'] = sys.argv[1]
cm['data']['storageInitializer'] = json.dumps(si)
json.dump(cm, sys.stdout)
" "${IMAGE}" | kubectl apply -f -

echo "Updated confidentialImage to ${IMAGE} in namespace ${NAMESPACE}"

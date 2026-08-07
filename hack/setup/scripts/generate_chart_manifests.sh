SCRIPT_DIR="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"

source "${SCRIPT_DIR}/../common.sh"

# KServe and Common
kustomize build ${REPO_ROOT}/config/default > ${REPO_ROOT}/charts/kserve-resources/files/kserve/resources.yaml
kustomize build ${REPO_ROOT}/config/certmanager > ${REPO_ROOT}/charts/kserve-resources/files/common/certmanager.yaml
kustomize build ${REPO_ROOT}/config/configmap > ${REPO_ROOT}/charts/kserve-resources/files/common/configmap.yaml

# LLMISVC Configs and Runtimes
kustomize build ${REPO_ROOT}/config/llmisvcconfig > ${REPO_ROOT}/charts/kserve-runtime-configs/files/llmisvcconfigs/resources.yaml
kustomize build ${REPO_ROOT}/config/runtimes > ${REPO_ROOT}/charts/kserve-runtime-configs/files/runtimes/resources.yaml

# LLMISVC and Common
kustomize build ${REPO_ROOT}/config/llmisvc > ${REPO_ROOT}/charts/kserve-llmisvc-resources/files/llmisvc/resources.yaml
kustomize build ${REPO_ROOT}/config/certmanager > ${REPO_ROOT}/charts/kserve-llmisvc-resources/files/common/certmanager.yaml
kustomize build ${REPO_ROOT}/config/configmap > ${REPO_ROOT}/charts/kserve-llmisvc-resources/files/common/configmap.yaml

# StorageContainer resources for Helm charts
echo "Building storagecontainer resources..."
kustomize build ${REPO_ROOT}/config/storagecontainers > ${REPO_ROOT}/charts/kserve-resources/files/common/storagecontainer.yaml
kustomize build ${REPO_ROOT}/config/storagecontainers > ${REPO_ROOT}/charts/kserve-llmisvc-resources/files/common/storagecontainer.yaml
echo "✅ Built storagecontainer resources"

# LocalModel and Common
kustomize build ${REPO_ROOT}/config/localmodels > ${REPO_ROOT}/charts/kserve-localmodel-resources/files/resources.yaml

# Generate values.yaml from common sections
echo "Generating values.yaml files from common sections..."

# kserve-resources values.yaml
yq eval-all '. as $item ireduce ({}; . * $item)' \
  ${REPO_ROOT}/charts/_common/common-sections.yaml \
  ${REPO_ROOT}/charts/_common/kserve-resources-specific.yaml \
  > ${REPO_ROOT}/charts/kserve-resources/values.yaml

# kserve-llmisvc-resources values.yaml
yq eval-all '. as $item ireduce ({}; . * $item)' \
  ${REPO_ROOT}/charts/_common/common-sections.yaml \
  ${REPO_ROOT}/charts/_common/kserve-llmisvc-resources-specific.yaml \
  > ${REPO_ROOT}/charts/kserve-llmisvc-resources/values.yaml

# kserve-localmodel-resources values.yaml
yq eval-all '. as $item ireduce ({}; . * $item)' \
  ${REPO_ROOT}/charts/_common/common-sections.yaml \
  ${REPO_ROOT}/charts/_common/kserve-localmodel-resources-specific.yaml \
  > ${REPO_ROOT}/charts/kserve-localmodel-resources/values.yaml

echo "✅ Generated values.yaml files"

# Sync common patch files to charts that need them
echo "Syncing common patch files..."
mkdir -p ${REPO_ROOT}/charts/kserve-resources/files/common
mkdir -p ${REPO_ROOT}/charts/kserve-llmisvc-resources/files/common

cp ${REPO_ROOT}/charts/_common/common-patches/*.yaml ${REPO_ROOT}/charts/kserve-resources/files/common/
cp ${REPO_ROOT}/charts/_common/common-patches/*.yaml ${REPO_ROOT}/charts/kserve-llmisvc-resources/files/common/

echo "✅ Synced common patch files"

#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

echo "Initialising feature repo ..."
feast init driver_feature_repo

pushd driver_feature_repo/feature_repo >> /dev/null
echo "Customizing the feature_store.yaml to use redis"
cat <<EOF >feature_store.yaml
project: driver_feature_repo
registry: data/registry.db
provider: local
online_store:
    type: redis
    connection_string: "redis-service.default.svc.cluster.local:6379"
entity_key_serialization_version: 2
EOF
echo "Running feast apply ..."
feast apply
echo "Materialising feature store ..."
feast materialize-incremental "$(date +%Y-%m-%d)"
popd

echo "copying feature store to the volume mount ..."
cp -r driver_feature_repo /mnt/driver_feature_repo

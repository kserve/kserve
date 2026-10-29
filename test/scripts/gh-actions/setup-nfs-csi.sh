#!/bin/bash

# Copyright 2026 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Installs an in-cluster NFS server and the csi-driver-nfs provisioner, then
# creates a ReadWriteMany (RWX) StorageClass named "nfs-csi". This gives the
# modelcache e2e suite real cross-node RWX storage so the shared-PVC import path
# (LocalModelNamespaceCache.spec.pvcRef) can be exercised end to end: one import
# Job writes a single copy that serving replicas on different nodes mount
# read-only.
#
# Usage: setup-nfs-csi.sh

set -o errexit
set -o nounset
set -o pipefail

CSI_DRIVER_NFS_VERSION="${CSI_DRIVER_NFS_VERSION:-v4.9.0}"
NFS_SERVER_IMAGE="${NFS_SERVER_IMAGE:-itsthenetwork/nfs-server-alpine:12}"
NFS_NAMESPACE="nfs-server"
STORAGE_CLASS="nfs-csi"

echo "Installing csi-driver-nfs ${CSI_DRIVER_NFS_VERSION}..."
curl -skSL "https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/${CSI_DRIVER_NFS_VERSION}/deploy/install-driver.sh" \
  | bash -s "${CSI_DRIVER_NFS_VERSION}" --

echo "Waiting for csi-driver-nfs controller and node plugin to be ready..."
kubectl -n kube-system rollout status deployment/csi-nfs-controller --timeout=300s
kubectl -n kube-system rollout status daemonset/csi-nfs-node --timeout=300s

echo "Deploying in-cluster NFS server..."
kubectl create namespace "${NFS_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nfs-server
  namespace: ${NFS_NAMESPACE}
  labels:
    app: nfs-server
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nfs-server
  template:
    metadata:
      labels:
        app: nfs-server
    spec:
      containers:
        - name: nfs-server
          image: ${NFS_SERVER_IMAGE}
          ports:
            - name: nfs
              containerPort: 2049
            - name: mountd
              containerPort: 20048
            - name: rpcbind
              containerPort: 111
          securityContext:
            privileged: true
          env:
            - name: SHARED_DIRECTORY
              value: /exports
          volumeMounts:
            - name: nfs-storage
              mountPath: /exports
      volumes:
        - name: nfs-storage
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: nfs-server
  namespace: ${NFS_NAMESPACE}
spec:
  selector:
    app: nfs-server
  ports:
    - name: nfs
      port: 2049
    - name: mountd
      port: 20048
    - name: rpcbind
      port: 111
EOF

echo "Waiting for NFS server to be ready..."
kubectl -n "${NFS_NAMESPACE}" rollout status deployment/nfs-server --timeout=300s

echo "Creating RWX StorageClass ${STORAGE_CLASS}..."
cat <<EOF | kubectl apply -f -
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ${STORAGE_CLASS}
provisioner: nfs.csi.k8s.io
parameters:
  server: nfs-server.${NFS_NAMESPACE}.svc.cluster.local
  share: /
reclaimPolicy: Delete
volumeBindingMode: Immediate
mountOptions:
  - nfsvers=4.1
EOF

echo "nfs-csi RWX StorageClass is ready."

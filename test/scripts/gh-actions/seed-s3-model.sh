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

# Copies a Hugging Face model into the SeaweedFS example-models bucket that
# setup-kserve.sh deploys, and prints its s3:// URI.
#
# Usage: seed-s3-model.sh <owner/model>
# Env:   STORAGE_INITIALIZER_IMAGE (default: kserve/storage-initializer:latest)

set -o errexit
set -o nounset
set -o pipefail

REPO="${1:?usage: seed-s3-model.sh <owner/model>}"
STORAGE_INITIALIZER_IMAGE="${STORAGE_INITIALIZER_IMAGE:-kserve/storage-initializer:latest}"
JOB="s3-seed-$(echo "${REPO}" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9-' '-' | cut -c1-50 | sed 's/-*$//')"

kubectl -n kserve delete job "${JOB}" --ignore-not-found --wait=true >&2
kubectl -n kserve apply -f - >&2 <<YAML
apiVersion: batch/v1
kind: Job
metadata:
  name: ${JOB}
spec:
  activeDeadlineSeconds: 1800
  backoffLimit: 3
  template:
    spec:
      restartPolicy: Never
      initContainers:
      - name: download-hf-model
        image: ${STORAGE_INITIALIZER_IMAGE}
        args: ["hf://${REPO}", "/shared/${REPO}"]
        env:
        - {name: HF_HUB_DISABLE_PROGRESS_BARS, value: "1"}
        - {name: HF_HUB_DISABLE_XET, value: "1"}
        - {name: HF_HUB_ENABLE_HF_TRANSFER, value: "0"}
        volumeMounts:
        - {name: model-cache, mountPath: /shared}
      containers:
      - name: s3-seed
        image: docker.io/amazon/aws-cli:2.33.11@sha256:88ea087837da1b31a3afeeb4538888244a3087a867ddb1909848643fcc4b324e
        command: ["/bin/sh", "-c", "aws s3 mb s3://example-models || true; aws s3 sync /shared/${REPO} s3://example-models/${REPO}/ --no-verify-ssl"]
        env:
        - name: AWS_ACCESS_KEY_ID
          valueFrom: {secretKeyRef: {name: mlpipeline-s3-artifact, key: accesskey}}
        - name: AWS_SECRET_ACCESS_KEY
          valueFrom: {secretKeyRef: {name: mlpipeline-s3-artifact, key: secretkey}}
        - {name: AWS_DEFAULT_REGION, value: us-east-1}
        - {name: AWS_ENDPOINT_URL_S3, value: "http://s3-service.kserve:8333"}
        volumeMounts:
        - {name: model-cache, mountPath: /shared}
      volumes:
      - name: model-cache
        emptyDir: {}
YAML
kubectl -n kserve wait --for=condition=complete "job/${JOB}" --timeout=30m >&2

echo "s3://example-models/${REPO}"

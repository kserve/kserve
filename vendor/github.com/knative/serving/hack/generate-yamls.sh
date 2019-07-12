#!/usr/bin/env bash

# Copyright 2018 The Knative Authors
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

# This script builds all the YAMLs that Knative serving publishes. It may be
# varied between different branches, of what it does, but the following usage
# must be observed:
#
# generate-yamls.sh  <repo-root-dir> <generated-yaml-list>
#     repo-root-dir         the root directory of the repository.
#     generated-yaml-list   an output file that will contain the list of all
#                           YAML files. The first file listed must be our
#                           manifest that contains all images to be tagged.

# Different versions of our scripts should be able to call this script with
# such assumption so that the test/publishing/tagging steps can evolve
# differently than how the YAMLs are built.

# The following environment variables affect the behavior of this script:
# * `$KO_FLAGS` Any extra flags that will be passed to ko.
# * `$YAML_OUTPUT_DIR` Where to put the generated YAML files, otherwise a
#   random temporary directory will be created. **All existing YAML files in
#   this directory will be deleted.**
# * `$KO_DOCKER_REPO` If not set, use ko.local as the registry.

set -o errexit
set -o pipefail

readonly YAML_REPO_ROOT=${1:?"First argument must be the repo root dir"}
readonly YAML_LIST_FILE=${2:?"Second argument must be the output file"}

# Location of istio YAMLs
readonly ISTIO_CRD_YAML=${YAML_REPO_ROOT}/third_party/istio-1.1.2/istio-crds.yaml
readonly ISTIO_YAML=${YAML_REPO_ROOT}/third_party/istio-1.1.2/istio.yaml
readonly ISTIO_LEAN_YAML=${YAML_REPO_ROOT}/third_party/istio-1.1.2/istio-lean.yaml
readonly ISTIO_KNATIVE_EXTRAS_YAML=${YAML_REPO_ROOT}/third_party/istio-1.1.2/istio-knative-extras.yaml

# Set output directory
if [[ -z "${YAML_OUTPUT_DIR:-}" ]]; then
  readonly YAML_OUTPUT_DIR="$(mktemp -d)"
fi
rm -fr ${YAML_OUTPUT_DIR}/*.yaml

# Generated Knative component YAML files
readonly SERVING_YAML=${YAML_OUTPUT_DIR}/serving.yaml
readonly MONITORING_YAML=${YAML_OUTPUT_DIR}/monitoring.yaml
readonly MONITORING_METRIC_PROMETHEUS_YAML=${YAML_OUTPUT_DIR}/monitoring-metrics-prometheus.yaml
readonly MONITORING_TRACE_ZIPKIN_YAML=${YAML_OUTPUT_DIR}/monitoring-tracing-zipkin.yaml
readonly MONITORING_TRACE_ZIPKIN_IN_MEM_YAML=${YAML_OUTPUT_DIR}/monitoring-tracing-zipkin-in-mem.yaml
readonly MONITORING_TRACE_JAEGER_YAML=${YAML_OUTPUT_DIR}/monitoring-tracing-jaeger.yaml
readonly MONITORING_TRACE_JAEGER_IN_MEM_YAML=${YAML_OUTPUT_DIR}/monitoring-tracing-jaeger-in-mem.yaml
readonly MONITORING_LOG_ELASTICSEARCH_YAML=${YAML_OUTPUT_DIR}/monitoring-logs-elasticsearch.yaml

# Flags for all ko commands
readonly KO_YAML_FLAGS="-P ${KO_FLAGS}"

if [[ -n "${TAG}" ]]; then
  LABEL_YAML_CMD=(sed -e "s|serving.knative.dev/release: devel|serving.knative.dev/release: \"${TAG}\"|")
else
  LABEL_YAML_CMD=(cat)
fi

: ${KO_DOCKER_REPO:="ko.local"}
export KO_DOCKER_REPO

cd "${YAML_REPO_ROOT}"

echo "Building Knative Serving"
ko resolve ${KO_YAML_FLAGS} -f config/ | "${LABEL_YAML_CMD[@]}" > "${SERVING_YAML}"

echo "Building Monitoring & Logging"
# Use ko to concatenate them all together.
ko resolve ${KO_YAML_FLAGS} -R -f config/monitoring/100-namespace.yaml \
    -f third_party/config/monitoring/logging/elasticsearch \
    -f config/monitoring/logging/elasticsearch \
    -f third_party/config/monitoring/metrics/prometheus \
    -f config/monitoring/metrics/prometheus \
    -f config/monitoring/tracing/zipkin | "${LABEL_YAML_CMD[@]}" > "${MONITORING_YAML}"

# Metrics via Prometheus & Grafana
ko resolve ${KO_YAML_FLAGS} -R -f config/monitoring/100-namespace.yaml \
    -f third_party/config/monitoring/metrics/prometheus \
    -f config/monitoring/metrics/prometheus | "${LABEL_YAML_CMD[@]}" > "${MONITORING_METRIC_PROMETHEUS_YAML}"

# Logs via ElasticSearch, Fluentd & Kibana
ko resolve ${KO_YAML_FLAGS} -R -f config/monitoring/100-namespace.yaml \
    -f third_party/config/monitoring/logging/elasticsearch \
    -f config/monitoring/logging/elasticsearch | "${LABEL_YAML_CMD[@]}" > "${MONITORING_LOG_ELASTICSEARCH_YAML}"

# Traces via Zipkin when ElasticSearch is installed
ko resolve ${KO_YAML_FLAGS} -R -f config/monitoring/tracing/zipkin | "${LABEL_YAML_CMD[@]}" > "${MONITORING_TRACE_ZIPKIN_YAML}"

# Traces via Zipkin in Memory when ElasticSearch is not installed
ko resolve ${KO_YAML_FLAGS} -R -f config/monitoring/tracing/zipkin-in-mem | "${LABEL_YAML_CMD[@]}" > "${MONITORING_TRACE_ZIPKIN_IN_MEM_YAML}"

# Traces via Jaeger when ElasticSearch is installed
ko resolve ${KO_YAML_FLAGS} -R -f config/monitoring/tracing/jaeger -f config/monitoring/tracing/jaeger/elasticsearch | "${LABEL_YAML_CMD[@]}" > "${MONITORING_TRACE_JAEGER_YAML}"

# Traces via Jaeger in Memory when ElasticSearch is not installed
ko resolve ${KO_YAML_FLAGS} -R -f config/monitoring/tracing/jaeger -f config/monitoring/tracing/jaeger/memory | "${LABEL_YAML_CMD[@]}" > "${MONITORING_TRACE_JAEGER_IN_MEM_YAML}"

echo "All manifests generated"

# List generated YAML files, with serving.yaml first.

ls -1 ${SERVING_YAML} > ${YAML_LIST_FILE}
ls -1 ${YAML_OUTPUT_DIR}/*.yaml | grep -v ${SERVING_YAML} >> ${YAML_LIST_FILE}
ls -1 ${ISTIO_CRD_YAML} ${ISTIO_YAML} ${ISTIO_LEAN_YAML} ${ISTIO_KNATIVE_EXTRAS_YAML} >> ${YAML_LIST_FILE}

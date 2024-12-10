#!/bin/bash

# Copyright 2023 The KServe Authors.
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

set -o errexit
set -o pipefail

usage() {
  cat <<EOF
Generate certificate suitable for use with an KFServing webhook service.
This script uses openssl to generate self-signed CA certificate that is
suitable for use with KFServing webhook services. See
https://kubernetes.io/docs/concepts/cluster-administration/certificates/#distributing-self-signed-ca-certificate
for detailed explanation and additional instructions.
The server key/cert CA cert are stored in a k8s secret.

usage: ${0} [OPTIONS]
The following flags are optional.
       --service                Service name of webhook. Default: kserve-webhook-server-service
       --namespace              Namespace where webhook service and secret reside. Default: kserve
       --secret                 Secret name for CA certificate and server certificate/key pair. Default: kserve-webhook-server-cert
       --mutatingWebhookName    Name for the mutating webhook config. This can be specified multiple times. Default: inferenceservice.serving.kserve.io
       --validatingWebhookName  Name for the validating webhook config. This can be specified multiple times. Default: All validating webhooks
       --webhookDeployment      Deployment name of the webhook controller. Default: kserve-controller-manager
EOF
  exit 1
}
mutatingWebhookNames=()
validatingWebhookNames=()
while [[ $# -gt 0 ]]; do
  case ${1} in
  --service)
    service="$2"
    shift
    ;;
  --secret)
    secret="$2"
    shift
    ;;
  --namespace)
    namespace="$2"
    shift
    ;;
  --mutatingWebhookName)
    mutatingWebhookNames+=("$2")
    shift
    ;;
  --validatingWebhookName)
    validatingWebhookNames+=("$2")
    shift
    ;;
  --webhookDeployment)
    webhookDeployment="$2"
    shift
    ;;
  *)
    usage
    ;;
  esac
  shift
done
#TODO check backward compatibility
[ -z "${secret}" ] && secret=kserve-webhook-server-cert
[ -z "${namespace}" ] && namespace=kserve
[ -z "${webhookDeployment}" ] && webhookDeployment=kserve-controller-manager
[ "${#validatingWebhookNames[@]}" -eq 0 ] && validatingWebhookNames=("inferenceservice.serving.kserve.io" "inferencegraph.serving.kserve.io" "servingruntime.serving.kserve.io" "clusterservingruntime.serving.kserve.io" "trainedmodel.serving.kserve.io" "localmodelcache.serving.kserve.io")
[ "${#mutatingWebhookNames[@]}" -eq 0 ] && mutatingWebhookNames=("inferenceservice.serving.kserve.io")
[ -z "${service}" ] && service=kserve-webhook-server-service
webhookDeploymentName=${webhookDeployment}
mutatingWebhookConfigNames=("${mutatingWebhookNames[@]}")
validatingWebhookConfigNames=("${validatingWebhookNames[@]}")
echo service: "${service}"
echo namespace: "${namespace}"
echo secret: "${secret}"
echo webhookDeploymentName: "${webhookDeploymentName}"
echo mutatingWebhookConfigNames: "${mutatingWebhookConfigNames[@]}"
echo validatingWebhookConfigNames: "${validatingWebhookConfigNames[@]}"
if [ ! -x "$(command -v openssl)" ]; then
  echo "openssl not found"
  exit 1
fi
tmpdir=$(mktemp -d)
echo "creating certs in tmpdir ${tmpdir} "
cat <<EOF >>"${tmpdir}"/csr.conf
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[ v3_req ]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names
[alt_names]
DNS.1 = ${service}
DNS.2 = ${service}.${namespace}
DNS.3 = ${service}.${namespace}.svc
DNS.4 = ${service}.${namespace}.svc.cluster
DNS.5 = ${service}.${namespace}.svc.cluster.local

EOF
# Create CA and Server key/certificate
openssl genrsa -out "${tmpdir}"/ca.key 2048
openssl req -x509 -newkey rsa:2048 -key "${tmpdir}"/ca.key -out "${tmpdir}"/ca.crt -days 365 -nodes -subj "/CN=${service}.${namespace}.svc"

openssl genrsa -out "${tmpdir}"/server.key 2048
openssl req -new -key "${tmpdir}"/server.key -subj "/CN=${service}.${namespace}.svc" -out ${tmpdir}/server.csr -config ${tmpdir}/csr.conf

# Self sign
openssl x509 -extensions v3_req -req -days 365 -in "${tmpdir}"/server.csr -CA "${tmpdir}"/ca.crt -CAkey "${tmpdir}"/ca.key -CAcreateserial -out "${tmpdir}"/server.crt -extfile "${tmpdir}"/csr.conf
# create the secret with server cert/key
kubectl create secret generic ${secret} \
  --from-file=tls.key="${tmpdir}"/server.key \
  --from-file=tls.crt="${tmpdir}"/server.crt \
  --dry-run=client -o yaml |
  kubectl -n ${namespace} apply -f -
# Webhook pod needs to be restarted so that the service reload the secret
# http://github.com/kubeflow/kubeflow/issues/3227
webhookPod=$(kubectl get pods -n ${namespace} | grep ${webhookDeploymentName} | awk '{print $1;}')
# ignore error if webhook pod does not exist
kubectl delete pod "${webhookPod}" -n "${namespace}" 2>/dev/null || true
echo "webhook ${webhookPod} is restarted to utilize the new secret"

echo "CA Certificate:"
cat "${tmpdir}"/ca.crt

# -a means base64 encode
caBundle=$(cat "${tmpdir}"/ca.crt | openssl enc -a -A)
echo "Encoded CA:"
echo -e "${caBundle} \n"

# check if jq is installed
if [ ! -x "$(command -v jq)" ]; then
  echo "jq not found"
  exit 1
fi
for webhookConfigName in "${mutatingWebhookConfigNames[@]}"; do
  echo "Patching mutating webhook ${webhookConfigName}"
  # Patch CA Certificate to mutatingWebhook
  mutatingWebhookCount=$(kubectl get mutatingwebhookconfiguration "${webhookConfigName}" -ojson | jq -r '.webhooks' | jq length)
  # build patchstring based on webhook counts
  mutatingPatchString='['
  for i in $(seq 0 $(($mutatingWebhookCount - 1))); do
    mutatingPatchString=$mutatingPatchString'{"op": "replace", "path": "/webhooks/'$i'/clientConfig/caBundle", "value":"{{CA_BUNDLE}}"}, '
  done
  # strip ', '
  mutatingPatchString=${mutatingPatchString%, }']'
  mutatingPatchString=$(echo ${mutatingPatchString} | sed "s|{{CA_BUNDLE}}|${caBundle}|g")

  echo "patching ca bundle for mutating webhook configuration(${webhookConfigName})..."
  kubectl patch mutatingwebhookconfiguration "${webhookConfigName}" \
    --type='json' -p="${mutatingPatchString}"
done

for webhookConfigName in "${validatingWebhookConfigNames[@]}"; do
  echo "Patching validating webhook ${webhookConfigName}"
  # Patch CA Certificate to validatingWebhook
  validatingWebhookCount=$(kubectl get validatingwebhookconfiguration "${webhookConfigName}" -ojson | jq -r '.webhooks' | jq length)
  validatingPatchString='['
  for i in $(seq 0 $(($validatingWebhookCount - 1))); do
    validatingPatchString=$validatingPatchString'{"op": "replace", "path": "/webhooks/'$i'/clientConfig/caBundle", "value":"{{CA_BUNDLE}}"}, '
  done
  validatingPatchString=${validatingPatchString%, }']'
  validatingPatchString=$(echo ${validatingPatchString} | sed "s|{{CA_BUNDLE}}|${caBundle}|g")

  echo "patching ca bundle for validating webhook configuration(${webhookConfigName})..."
  kubectl patch validatingwebhookconfiguration "${webhookConfigName}" \
    --type='json' -p="${validatingPatchString}"
done

# The conversion webhook has been removed: https://github.com/kserve/kserve/pull/3700/files. 
# However, this is kept as a reference for potential future use of the conversion webhook.

# echo "patching ca bundler for conversion webhook configuration.."
# conversionPatchString='[{"op": "replace", "path": "/spec/conversion/webhook/clientConfig/caBundle", "value":"{{CA_BUNDLE}}"}]'
# conversionPatchString=$(echo ${conversionPatchString} | sed "s|{{CA_BUNDLE}}|${caBundle}|g")
# kubectl patch CustomResourceDefinition inferenceservices.serving.kserve.io \
#   --type='json' -p="${conversionPatchString}"

#!/bin/bash

set -e

usage() {
    cat <<EOF
Generate certificate suitable for use with an KFServing webhook service.
This script uses openssl to generate self-signed CA certificate that is
suitable for use with KFServing webhook services. See
https://kubernetes.io/docs/concepts/cluster-administration/certificates/#distributing-self-signed-ca-certificate
for detailed explantion and additional instructions.
The server key/cert CA cert are stored in a k8s secret.

usage: ${0} [OPTIONS]
The following flags are optional.
       --service           Service name of webhook. Default: kfserving-webhook-server-service
       --namespace         Namespace where webhook service and secret reside. Default: kfserving-system
       --secret            Secret name for CA certificate and server certificate/key pair. Default: kfserving-webhook-server-cert
       --webhookName       Name for the mutating and validating webhook config. Default: inferenceservice.serving.kubeflow.org
       --webhookDeployment Statefulset name of the webhook controller. Default: kfserving-controller-manager
EOF
    exit 1
}

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
        --webhookName)
            webhookName="$2"
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
[ -z ${secret} ] && secret=kfserving-webhook-server-cert
[ -z ${namespace} ] && namespace=kfserving-system
[ -z ${webhookDeployment} ] && webhookDeployment=kfserving-controller-manager
[ -z ${webhookName} ] && webhookName=inferenceservice.serving.kubeflow.org
[ -z ${service} ] && service=kfserving-webhook-server-service
webhookDeploymentName=${webhookDeployment}-0
webhookConfigName=${webhookName}
echo service: ${service}
echo namespace: ${namespace}
echo secret: ${secret}
echo webhookDeploymentName: ${webhookDeploymentName}
echo webhookConfigName: ${webhookConfigName}
if [ ! -x "$(command -v openssl)" ]; then
    echo "openssl not found"
    exit 1
fi
tmpdir=$(mktemp -d)
echo "creating certs in tmpdir ${tmpdir} "
cat <<EOF >> ${tmpdir}/csr.conf
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
openssl genrsa -out ${tmpdir}/ca.key 2048
openssl req -x509 -newkey rsa:2048 -key ${tmpdir}/ca.key -out ${tmpdir}/ca.crt -days 365 -nodes -subj "/CN=${service}.${namespace}.svc"

openssl genrsa -out ${tmpdir}/server.key 2048
openssl req -new -key ${tmpdir}/server.key -subj "/CN=${service}.${namespace}.svc" -out ${tmpdir}/server.csr -config ${tmpdir}/csr.conf

# Self sign
openssl x509 -req -days 365 -in ${tmpdir}/server.csr -CA ${tmpdir}/ca.crt -CAkey ${tmpdir}/ca.key -CAcreateserial -out ${tmpdir}/server.crt -extfile ${tmpdir}/csr.conf
# create the secret with server cert/key
kubectl create secret generic ${secret} \
        --from-file=tls.key=${tmpdir}/server.key \
        --from-file=tls.crt=${tmpdir}/server.crt \
        --dry-run -o yaml |
    kubectl -n ${namespace} apply -f -
# Webhook pod needs to be restarted so that the service reload the secret
# http://github.com/kueflow/kubeflow/issues/3227
webhookPod=$(kubectl get pods -n ${namespace} |grep ${webhookDeploymentName} |awk '{print $1;}')
# ignore error if webhook pod does not exist
kubectl delete pod ${webhookPod} -n ${namespace} 2>/dev/null || true
echo "webhook ${webhookPod} is restarted to utilize the new secret"

echo "CA Certificate:"
cat ${tmpdir}/ca.crt

# -a means base64 encode
caBundle=$(cat ${tmpdir}/ca.crt | openssl enc -a -A)
echo "Encoded CA:"
echo -e "${caBundle} \n"

# Patch CA Certificate to webhooks
mutatingPatchString='[{"op": "replace", "path": "/webhooks/0/clientConfig/caBundle", "value":"{{CA_BUNDLE}}"}, {"op": "replace", "path": "/webhooks/1/clientConfig/caBundle", "value":"{{CA_BUNDLE}}"}]'
mutatingPatchString=$(echo ${mutatingPatchString} | sed "s|{{CA_BUNDLE}}|${caBundle}|g")
validatingPatchString='[{"op": "replace", "path": "/webhooks/0/clientConfig/caBundle", "value":"{{CA_BUNDLE}}"}]'
validatingPatchString=$(echo ${validatingPatchString} | sed "s|{{CA_BUNDLE}}|${caBundle}|g")

echo "patching ca bundle for mutating webhook configuration..."
kubectl patch mutatingwebhookconfiguration ${webhookConfigName} \
    --type='json' -p="${mutatingPatchString}"

echo "patching ca bundle for validating webhook configuration..."
kubectl patch validatingwebhookconfiguration ${webhookConfigName} \
    --type='json' -p="${validatingPatchString}"
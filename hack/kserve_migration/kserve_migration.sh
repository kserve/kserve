#!/bin/bash
# Usage: kserve_migration.sh

set -o errexit
set -o errtrace

export KSERVE_VERSION=v0.7.0-rc0
export CONFIG_DIR="config"
export ISVC_CONFIG_DIR="${CONFIG_DIR}/isvc"
export KSVC_CONFIG_DIR="${CONFIG_DIR}/ksvc"

CLEAN_KFSERVING="true"
KFSERVING_NAMESPACE=${KFSERVING_NAMESPACE:-kfserving-system}

# custom logger
log() {
    level=$1
    msg=$2
    ts=$(date -u +'%F %T')
    echo "${ts} [${level}] $msg"
}

# Validates whether controller manager and models web app service running 
# on this machine for the given namespace or not.
isControllerRunning() {
    namespace=$1
    prefix="kserve"
    if [ "${namespace}" == "${KFSERVING_NAMESPACE}" ]; then
        prefix="kfserving"
    fi
    svc_names=$(kubectl get svc -n $namespace -o jsonpath='{.items[*].metadata.name}')
    for svc_name in "${prefix}-controller-manager-metrics-service" \
                    "${prefix}-controller-manager-service"; do
        if [ ! -z "${svc_names##*$svc_name*}" ]; then
            log ERROR "${prefix} controller services are not installed completely."
            exit 1;
        fi
    done
    po_names=$(kubectl get po -n $namespace -o jsonpath='{.items[*].metadata.name}')
    for po_name in "${prefix}-controller-manager"; do
        if [ ! -z "${po_names##*$po_name*}" ]; then
            log ERROR "${prefix} controller services are not installed completely."
            exit 1;
        fi
    done
}

# Checks user preference on cleaning kfserving controller
if [ "${REMOVE_KFSERVING}" == "false" ]; then
    CLEAN_KFSERVING="false"
fi

# Checks whether the kfserving is running or not
log INFO "checking whether kfserving is running or not"
isControllerRunning "${KFSERVING_NAMESPACE}"

# Checks whether the kserve is running or not
log INFO "checking whether kserve is running or not"
isControllerRunning kserve

# # Deploy kserve
# log INFO "deploying kserve"
# cd ..
# KSERVE_CONFIG=kserve.yaml
# for i in 1 2 3 4 5 ; do kubectl apply -f install/${KSERVE_VERSION}/${KSERVE_CONFIG} && break || sleep 15; done
# kubectl wait --for=condition=ready --timeout=120s po --all -n kserve
# isControllerRunning kserve
# cd hack
# log INFO "kserve deployment completed"

# Get inference services config
log INFO "getting inference services config"
inference_services=$(kubectl get inferenceservice.serving.kubeflow.org -A -o jsonpath='{.items[*].metadata.namespace},{.items[*].metadata.name}')
declare -a isvc_names
declare -a isvc_ns
declare -A kfserving_isvc_status
if [ ! -z "$inference_services" ]; then
    mkdir -p ${ISVC_CONFIG_DIR}
    IFS=','; isvc_split=($inference_services); unset IFS;
    isvc_ns=(${isvc_split[0]})
    isvc_names=(${isvc_split[1]})
fi
isvc_count=${#isvc_names[@]}
for (( i=0; i<${isvc_count}; i++ ));
do
    kubectl get inferenceservice.serving.kubeflow.org ${isvc_names[$i]} -n ${isvc_ns[$i]} -o yaml > "${ISVC_CONFIG_DIR}/${isvc_names[$i]}.yaml"
    kfserving_isvc_status[${isvc_names[$i]}]=$(kubectl get inferenceservice.serving.kubeflow.org ${isvc_names[$i]} -n ${isvc_ns[$i]} -o json | jq --raw-output '.status.conditions | map(select(.type == "Ready"))[0].status')
done

# Get knative services names
log INFO "getting knative services"
knative_services=$(kubectl get ksvc -A -o jsonpath='{.items[*].metadata.namespace},{.items[*].metadata.name}')
declare -a ksvc_names;
declare -a ksvc_ns;
if [ ! -z "$knative_services" ]; then
    mkdir -p ${KSVC_CONFIG_DIR}
    IFS=','; ksvc_split=(${knative_services}); unset IFS;
    ksvc_ns=(${ksvc_split[0]})
    ksvc_names=(${ksvc_split[1]})
fi
ksvc_count=${#ksvc_names[@]}

(
    # Stop kfserving controller
    log INFO "stopping kfserving controller"
    kubectl scale --replicas=0 statefulset.apps kfserving-controller-manager -n "${KFSERVING_NAMESPACE}"
    sleep 30

    trap 'kubectl scale --replicas=1 statefulset.apps kfserving-controller-manager -n ${KFSERVING_NAMESPACE}' ERR

    # Deploy inference services on kserve
    log INFO "deploying inference services on kserve"
    for (( i=0; i<${isvc_count}; i++ ));
    do
        yq d -i "${ISVC_CONFIG_DIR}/${isvc_names[$i]}.yaml" 'metadata.annotations[kubectl.kubernetes.io/last-applied-configuration]'
        yq d -i "${ISVC_CONFIG_DIR}/${isvc_names[$i]}.yaml" 'metadata.creationTimestamp'
        yq d -i "${ISVC_CONFIG_DIR}/${isvc_names[$i]}.yaml" 'metadata.finalizers'
        yq d -i "${ISVC_CONFIG_DIR}/${isvc_names[$i]}.yaml" 'metadata.generation'
        yq d -i "${ISVC_CONFIG_DIR}/${isvc_names[$i]}.yaml" 'metadata.resourceVersion'
        yq d -i "${ISVC_CONFIG_DIR}/${isvc_names[$i]}.yaml" 'metadata.uid'
        yq d -i "${ISVC_CONFIG_DIR}/${isvc_names[$i]}.yaml" 'metadata.managedFields'
        yq d -i "${ISVC_CONFIG_DIR}/${isvc_names[$i]}.yaml" 'status'
        sed -i -- 's/kubeflow.org/kserve.io/g' ${ISVC_CONFIG_DIR}/${isvc_names[$i]}.yaml
        kubectl apply -f "${ISVC_CONFIG_DIR}/${isvc_names[$i]}.yaml"
    done
)
sleep 300

# Remove owner references from knative services
log INFO "removing owner references from knative services"
declare -A ksvc_isvc_map
for (( i=0; i<${ksvc_count}; i++ ));
do
    ksvc_api_version=$(kubectl get ksvc ${ksvc_names[$i]} -n ${ksvc_ns[$i]} -o json | jq --raw-output '.metadata.ownerReferences[0].apiVersion')
    if [ "$ksvc_api_version" == "serving.kubeflow.org/v1beta1" ]; then
        ksvc_isvc_map[${ksvc_names[$i]}]=$(kubectl get ksvc ${ksvc_names[$i]} -n ${ksvc_ns[$i]} -o json | jq --raw-output '.metadata.ownerReferences[0].name')
        kubectl patch ksvc ${ksvc_names[$i]} -n ${ksvc_ns[$i]} --type json -p='[{"op": "remove", "path": "/metadata/ownerReferences"}]'
    fi
done

# Remove owner references from virtual services
log INFO "removing owner references from virtual services"
for (( i=0; i<${isvc_count}; i++ ));
do
    vsvc_api_version=$(kubectl get virtualservices ${isvc_names[$i]} -n ${isvc_ns[$i]} -o json | jq --raw-output '.metadata.ownerReferences[0].apiVersion')
    if [ "$vsvc_api_version" == "serving.kubeflow.org/v1beta1" ]; then
        kubectl patch virtualservices ${isvc_names[$i]} -n ${isvc_ns[$i]} --type json -p='[{"op": "remove", "path": "/metadata/ownerReferences"}]'
    fi
done
sleep 5

# Extract inference service uids
log INFO "extracting inference service uids"
declare -A infr_uid_map
for (( i=0; i<${isvc_count}; i++ ));
do
    infr_uid_map[${isvc_names[$i]}]=$(kubectl get inferenceservice.serving.kserve.io ${isvc_names[$i]} -n ${isvc_ns[$i]} -o json | jq --raw-output '.metadata.uid')
done

# Update knative services with new owner reference
log INFO "updating knative services with new owner reference"
for (( i=0; i<${ksvc_count}; i++ ));
do
    owner_ref_count=$(kubectl get ksvc ${ksvc_names[$i]} -n ${ksvc_ns[$i]} -o json | jq --raw-output '.metadata.ownerReferences | length')
    if [ $owner_ref_count -eq 0 ]; then
        isvc_name=${ksvc_isvc_map[${ksvc_names[$i]}]}
        isvc_uid=${infr_uid_map[${isvc_name}]}
        kubectl patch ksvc ${ksvc_names[$i]} -n ${ksvc_ns[$i]} --type='json' -p='[{"op": "add", "path": "/metadata/ownerReferences", "value": [{"apiVersion": "serving.kserve.io/v1beta1","blockOwnerDeletion": true,"controller": true,"kind": "InferenceService","name": "'${isvc_name}'","uid": "'${isvc_uid}'"}] }]'
    fi
done

# Update virtual services with new owner reference
log INFO "updating virtual services with new owner reference"
for (( i=0; i<${isvc_count}; i++ ));
do
    owner_ref_count=$(kubectl get virtualservices ${isvc_names[$i]} -n ${isvc_ns[$i]} -o json | jq --raw-output '.metadata.ownerReferences | length')
    if [ $owner_ref_count -eq 0 ]; then
        isvc_uid=${infr_uid_map[${isvc_names[$i]}]}
        kubectl patch virtualservices ${isvc_names[$i]} -n ${isvc_ns[$i]} --type='json' -p='[{"op": "add", "path": "/metadata/ownerReferences", "value": [{"apiVersion": "serving.kserve.io/v1beta1","blockOwnerDeletion": true,"controller": true,"kind": "InferenceService","name": "'${isvc_names[$i]}'","uid": "'${isvc_uid}'"}] }]'
    fi
done
sleep 5

# Verify that all inference services are migrated and ready
log INFO "verifying inference services are migrated and ready"
for (( i=0; i<${isvc_count}; i++ ));
do
    if [ "${kfserving_isvc_status[${isvc_names[$i]}]}" == "True" ]; then
        (
            trap 'log ERROR "inference service ${isvc_names[$i]} did not migrate properly. migration job exits with code 1."' ERR
            kubectl wait --for=condition=ready --timeout=10s inferenceservice.serving.kserve.io/${isvc_names[$i]} -n ${isvc_ns[$i]}
        )
    fi
done

# Start kfserving controller for clean up
log INFO "starting kfserving controller for clean up"
kubectl scale --replicas=1 statefulset.apps kfserving-controller-manager -n "${KFSERVING_NAMESPACE}"

# Delete inference services running on kfserving
log INFO "deleting inference services on kfserving"
for (( i=0; i<${isvc_count}; i++ ));
do
    kubectl delete inferenceservice.serving.kubeflow.org ${isvc_names[$i]} -n ${isvc_ns[$i]}
done

# Clean up kfserving
if [ "${CLEAN_KFSERVING}" == "true" ]; then
    log INFO "deleting kfserving namespace"

    if [ "${KFSERVING_NAMESPACE}" != "kubeflow" ]; then
      kubectl delete ns "${KFSERVING_NAMESPACE}"
    fi

    log INFO "deleting kfserving cluster role and cluster role binding"
    kubectl delete ClusterRoleBinding kfserving-manager-rolebinding
    kubectl delete ClusterRoleBinding kfserving-models-web-app-binding
    kubectl delete ClusterRoleBinding kfserving-proxy-rolebinding

    log INFO "deleting kfserving webhook configuration and crd"
    kubectl delete CustomResourceDefinition inferenceservices.serving.kubeflow.org
    kubectl delete CustomResourceDefinition trainedmodels.serving.kubeflow.org

    kubectl delete MutatingWebhookConfiguration inferenceservice.serving.kubeflow.org
    kubectl delete ValidatingWebhookConfiguration inferenceservice.serving.kubeflow.org
    kubectl delete ValidatingWebhookConfiguration trainedmodel.serving.kubeflow.org
fi

rm -rf ${CONFIG_DIR}

log INFO "kserve migration completed successfully"
exit 0;

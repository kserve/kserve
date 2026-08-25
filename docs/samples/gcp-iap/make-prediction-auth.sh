# This script explains how to make a http request to an inference service hosted behind a GCP IAP.

# Prerequisites:
# set following parameters:
#   - ${PROJECT} - the gcp project that owns the Identity Aware Proxy https://console.cloud.google.com/security/iap
#   - ${NAMESPACE} - the k8s namespace the inference service has been deployed to
#   - ${KUBEFLOW_NAME} - the full Kubeflow cluster name
#   - ${INGRESS_DNS} - the external dns of the loadbalancer service / gateway
#   - ${IAP_CLIENT_ID} - the Outh 2.0 client id used by the IAP. See https://console.cloud.google.com/apis/credentials 
#   - ${DESKTOP_CLIENT_ID} - DESKTOP_CLIENT_ID in https://cloud.google.com/iap/docs/authentication-howto#setting_up_the_client_id
#   - ${DESKTOP_CLIENT_SECRET} - DESKTOP_CLIENT_SECRET in https://cloud.google.com/iap/docs/authentication-howto#setting_up_the_client_id
#   - ${USER_EMAIL} - The user account which has access to Kubeflow cluster in the target namespace.
#   - ${SERVICE_URL} - the external 'path based route' defined and exposed by virtual-service.yaml


# Set the environment
INFERENCE_SERVICE=sklearn-iap
INPUT_PATH=./iris-input.json
PROJECT='<project>'
NAMESPACE='<namespace>'
KUBEFLOW_NAME='<kubeflow-cluster-name>'
INGRESS_DNS=${KUBEFLOW_NAME}.endpoints.${PROJECT}.cloud.goog
IAP_CLIENT_ID='<project-id>-<random-32-hash>.apps.googleusercontent.com'
DESKTOP_CLIENT_ID='<project-id>-<random-32-hash>.apps.googleusercontent.com'
DESKTOP_CLIENT_SECRET='<hash-code>'
USER_EMAIL='<user>@<mail>.com'
SERVICE_URL=https://${INGRESS_DNS}/kfserving/${NAMESPACE}/${INFERENCE_SERVICE}:predict


python iap_request_auth.py --url=${SERVICE_URL} \
--iap_client_id=${IAP_CLIENT_ID} \
--desktop_client_id=${DESKTOP_CLIENT_ID} \
--desktop_client_secret=${DESKTOP_CLIENT_SECRET} \
--user_account=${USER_EMAIL} \
--input=./iris-input.json

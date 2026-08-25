# This script explains how to make a http request to an inference service hosted behind a GCP IAP.  It handles obtaining and using a JWT.

# Pre-requisites:
# 1. download service account key
#   - gcloud iam service-accounts keys create key.json --iam-account <service-account>@<project>.iam.gserviceaccount.com
#     Service account name can be custom-user@<project>.iam.gserviceaccount.com
#     https://www.kubeflow.org/docs/external-add-ons/serving/tfserving_new/#sending-prediction-request-through-ingress-and-iap
# 2. download iap_request.py (this uses key.json to obtain a JWT and invokes inference service with 'Authorization: Bearer <JWT> header')
# 3. set following parameters:
#   - ${PROJECT} - the gcp project that owns the Identity Aware Proxy https://console.cloud.google.com/security/iap
#   - ${NAMESPACE} - the k8s namespace the inference service has been deployed to
#   - ${KUBEFLOW_NAME} - the full Kubeflow cluster name
#   - ${INGRESS_DNS} - the external dns of the loadbalancer service / gateway
#   - ${IAP_CLIENT_ID} - the Outh 2.0 client id used by the IAP. See https://console.cloud.google.com/apis/credentials 
#   - ${SERVICE_URL} - the external 'path based route' defined and exposed by virtual-service.yaml
export GOOGLE_APPLICATION_CREDENTIALS=key.json

# Set the environment
INFERENCE_SERVICE=sklearn-iap
INPUT_PATH=@./iris-input.json
PROJECT='<project>'
NAMESPACE='<namespace>'
KUBEFLOW_NAME='<kubeflow-cluster-name>'
INGRESS_DNS=${KUBEFLOW_NAME}.endpoints.${PROJECT}.cloud.goog
IAP_CLIENT_ID='<project-id>-<random-32-hash>.apps.googleusercontent.com'
SERVICE_URL=https://${INGRESS_DNS}/kfserving/${NAMESPACE}/${INFERENCE_SERVICE}:predict

# Print out the command that can be used to execute a http prediction request
echo python iap_request.py $SERVICE_URL ${IAP_CLIENT_ID} --input=${INPUT_PATH}

# Uncomment next line to execute
python iap_request.py $SERVICE_URL ${IAP_CLIENT_ID} --input=${INPUT_PATH}

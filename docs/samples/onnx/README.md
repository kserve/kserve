
# Predict on a KFService using ONNX
## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#deploy-kfserving).
2. Your cluster's Istio Ingress gateway must be network accessible.
3. Your cluster's Istio Egresss gateway must [allow Google Cloud Storage](https://knative.dev/docs/serving/outbound-network-access/)

## Create the KFService
Apply the CRD
```
kubectl apply -f onnx.yaml 
```

Expected Output
```
$ kfservice.serving.kubeflow.org/style-sample configured
```

## Run a sample inference
1. Setup env vars
```
export SERVICE_URL=$(kubectl get kfservice ${MODEL_NAME} -o jsonpath='{.status.url}')
```
2. Verify the service is healthy
```
curl ${SERVICE_URL}
```
3. Install dependencies
```
pip install jupyter
pip install numpy
pip install pillow
pip install protobuf
pip install requests
```
4. Run the [sample notebook](mosaic-onnx.ipynb) in jupyter
```
jupyter notebook
```

## Uploading your own model
Upload your model as model.onnx to S3, GCS or an Azure Blob
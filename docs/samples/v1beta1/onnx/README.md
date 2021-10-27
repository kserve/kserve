
# Deploy InferenceService with ONNX model
## Setup
1. Your ~/.kube/config should point to a cluster with [KServe installed](https://github.com/kserve/kserve#installation).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).

## Create the InferenceService
Apply the CRD
```
kubectl apply -f onnx.yaml 
```

Expected Output
```
$ inferenceservice.serving.kserve.io/style-sample configured
```

## Run a sample inference

1. Setup env vars
The first step is to [determine the ingress IP and ports](https://kserve.github.io/website/get_started/first_isvc/#3-determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```
export MODEL_NAME=style-sample
export SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}' | cut -d "/" -f 3)
```
2. Verify the service is healthy
```
SERVICE_URL=http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME
curl ${SERVICE_URL}
```
3. Install dependencies
```
pip install -r requirements.txt
```
4. Run the [sample notebook](mosaic-onnx.ipynb) in jupyter
```
jupyter notebook
```

## Uploading your own model
The sample model for the example in this readme is already uploaded and available for use. However if you would like to modify the example to use your own ONNX model, all you need to do is to upload your model as `model.onnx` to S3, GCS or an Azure Blob.

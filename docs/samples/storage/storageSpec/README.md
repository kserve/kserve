# Predict on a InferenceService with StorageSpec
## Setup
1. Your `~/.kube/config` should point to a cluster with [KServe installed](https://github.com/kserve/kserve#installation).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).
3. Your cluster's Istio Egresss gateway must [allow accessing S3 Storage](https://knative.dev/docs/serving/outbound-network-access/)

## Create Common Storage Secret
Create a common secret with your [storage credential](https://console.aws.amazon.com/iam/home#/users), `KServe` reads the secret key to inject
the necessary storage volume on storage initializer or model agent to download the models from the destination storage.
```yaml
apiVersion: v1
stringData:
  localMinIO: |
    {
      "type": "s3",
      "access_key_id": "minio",
      "secret_access_key": "minio123",
      "endpoint_url": "http://minio-service.kubeflow:9000",
      "bucket": "mlpipeline",
      "region": "us-south",
      "anonymous": "False"
    }
kind: Secret
metadata:
  name: storage-config
type: Opaque
```

Apply the secret and service account
```bash
kubectl apply -f common_secret.yaml
```

Then, download the [sklearn model.joblib](https://console.cloud.google.com/storage/browser/kfserving-examples/models/sklearn/1.0/model) and store the model at the path `sklearn/model.joblib` inside the a new bucket called `example-models`.

Note: if you are running kserve with istio sidecars enabled, there can be a race condition between the istio proxy being ready and the agent pulling models.
This will result in a `tcp dial connection refused` error when the agent tries to download from s3.
To resolve it, istio allows the blocking of other containers in a pod until the proxy container is ready.
You can enabled this by setting `proxy.holdApplicationUntilProxyStarts: true` in `istio-sidecar-injector` configmap,
`proxy.holdApplicationUntilProxyStarts` flag was introduced in Istio 1.7 as an experimental feature and is turned off by default.

## Create the InferenceService
Create the InferenceService with the storage spec and select the credential entry.
```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: example-sklearn-isvc
spec:
  predictor:
    sklearn:
      storage:
        key: localMinIO # Credential key for the destination storage in the common secret
        path: sklearn # Model path inside the bucket
        # schemaPath: null # Optional schema files for payload schema
        parameters: # Parameters to override the default values inside the common secret.
          bucket: example-models
```

```bash
kubectl apply -f sklearn_storagespec.yaml
```

Expected Output
```
$ inferenceservice.serving.kserve.io/example-sklearn-isvc created
```

## Run a prediction
The first step is to [determine the ingress IP and ports](../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```bash
MODEL_NAME=example-sklearn-isvc
INPUT_PATH=@./iris-input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict -d $INPUT_PATH
```
Expected Output
```
*   Trying 169.45.97.44:80...
* Connected to 169.45.97.44 (169.45.97.44) port 80 (#0)
> POST /v1/models/example-sklearn-isvc:predict HTTP/1.1
> Host: example-sklearn-isvc.default.example.com
> User-Agent: curl/7.71.1
> Accept: */*
> Content-Length: 76
> Content-Type: application/x-www-form-urlencoded
>
* upload completely sent off: 76 out of 76 bytes
* Mark bundle as not supporting multiuse
< HTTP/1.1 200 OK
< content-length: 23
< content-type: application/json; charset=UTF-8
< date: Thu, 04 Nov 2021 23:17:45 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 12
<
* Connection #0 to host 169.45.97.44 left intact
{"predictions": [1, 1]}
```

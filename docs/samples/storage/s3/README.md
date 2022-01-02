# Predict on a InferenceService with saved model on S3
## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).
3. Your cluster's Istio Egress gateway must [allow accessing S3 Storage](https://knative.dev/docs/serving/outbound-network-access/)

## Create S3 Secret and attach to Service Account
Create a secret with your [S3 user credential](https://console.aws.amazon.com/iam/home#/users), `KFServing` reads the secret annotations to inject 
the S3 environment variables on storage initializer or model agent to download the models from S3 storage. 
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mysecret
  annotations:
     serving.kserve.io/s3-endpoint: s3.amazonaws.com # replace with your s3 endpoint e.g minio-service.kubeflow:9000 
     serving.kserve.io/s3-usehttps: "1" # by default 1, if testing with minio you can set to 0
     serving.kserve.io/s3-region: "us-east-2"
     serving.kserve.io/s3-useanoncredential: "false" # omitting this is the same as false, if true will ignore provided credential and use anonymous credentials
type: Opaque
stringData: # use `stringData` for raw credential string or `data` for base64 encoded string
  AWS_ACCESS_KEY_ID: XXXX
  AWS_SECRET_ACCESS_KEY: XXXXXXXX
```

The next step is to attach the created secret to the service account's secret list.
By default `KFServing` uses `default` service account, you can create your own service account and overwrite on `InferenceService` CRD.

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: sa
secrets:
- name: mysecret
```

Apply the secret and service account
```bash
kubectl apply -f s3_secret.yaml
```

Note: if you are running kfserving with istio sidecars enabled, there can be a race condition between the istio proxy being ready and the agent pulling models. 
This will result in a `tcp dial connection refused` error when the agent tries to download from s3.
To resolve it, istio allows the blocking of other containers in a pod until the proxy container is ready. 
You can enabled this by setting `proxy.holdApplicationUntilProxyStarts: true` in `istio-sidecar-injector` configmap,
`proxy.holdApplicationUntilProxyStarts` flag was introduced in Istio 1.7 as an experimental feature and is turned off by default.

## Create the InferenceService
Create the InferenceService with the s3 `storageUri` and the service account with s3 credential attached.
```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "mnist-s3"
spec:
  predictor:
    serviceAccountName: sa
    tensorflow:
      storageUri: "s3://kfserving-examples/mnist"
```

```bash
kubectl apply -f tensorflow_s3.yaml
```

Expected Output
```
$ inferenceservice.serving.kserve.io/mnist-s3 created
```

## Run a prediction
The first step is to [determine the ingress IP and ports](../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```bash
MODEL_NAME=mnist-s3
INPUT_PATH=@./input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice mnist-s3 -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict -d $INPUT_PATH
```
Expected Output
```
Note: Unnecessary use of -X or --request, POST is already inferred.
*   Trying 35.237.217.209...
* TCP_NODELAY set
* Connected to mnist-s3.default.35.237.217.209.xip.io (35.237.217.209) port 80 (#0)
> POST /v1/models/mnist-s3:predict HTTP/1.1
> Host: mnist-s3.default.35.237.217.209.xip.io
> User-Agent: curl/7.55.1
> Accept: */*
> Content-Length: 2052
> Content-Type: application/x-www-form-urlencoded
> Expect: 100-continue
>
< HTTP/1.1 100 Continue
* We are completely uploaded and fine
< HTTP/1.1 200 OK
< content-length: 251
< content-type: application/json
< date: Sun, 04 Apr 2021 20:06:27 GMT
< x-envoy-upstream-service-time: 5
< server: istio-envoy
<
{
    "predictions": [
        {
            "predictions": [0.327352405, 2.00153053e-07, 0.0113353515, 0.203903764, 3.62863029e-05, 0.416683704, 0.000281196437, 8.36911859e-05, 0.0403052084, 1.82206513e-05],
            "classes": 5
        }
    ]
}* Connection #0 to host mnist-s3.default.35.237.217.209.xip.io left intact
```

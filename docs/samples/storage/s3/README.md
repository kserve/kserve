    
# Predict on a InferenceService with saved model on S3
## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).
3. Your cluster's Istio Egresss gateway must [allow accessing S3 Storage](https://knative.dev/docs/serving/outbound-network-access/)
4. The example uses the Kubeflow's Minio setup if you have [Kubeflow](https://www.kubeflow.org/docs/started/getting-started/) installed,
you can also setup your own [Minio server](https://docs.min.io/docs/deploy-minio-on-kubernetes.html) or use other S3 compatible cloud storage.


## Train TF mnist model and save on S3
Follow Kubeflow's [TF mnist example](https://github.com/kubeflow/examples/tree/master/mnist#using-s3) to train a TF mnist model and save on S3,
change following S3 access settings, `modelDir` and `exportDir` as needed. If you already have a mnist model saved on S3 you can skip this step.
```bash
export S3_USE_HTTPS=0 #set to 0 for default minio installs
export S3_ENDPOINT=minio-service.kubeflow:9000
export AWS_ENDPOINT_URL=http://${S3_ENDPOINT}

kustomize edit add configmap mnist-map-training --from-literal=S3_ENDPOINT=${S3_ENDPOINT}
kustomize edit add configmap mnist-map-training --from-literal=AWS_ENDPOINT_URL=${AWS_ENDPOINT_URL}
kustomize edit add configmap mnist-map-training --from-literal=S3_USE_HTTPS=${S3_USE_HTTPS}

kustomize edit add configmap mnist-map-training --from-literal=modelDir=s3://mnist/v1
kustomize edit add configmap mnist-map-training --from-literal=exportDir=s3://mnist/v1/export
```

## Create S3 Secret and attach to Service Account
If you already have a S3 secret created from last step you can skip this step, since `KFServing` is relying on secret annotations to setup proper
S3 environment variables you may still need to add following annotations to your secret to overwrite S3 endpoint or other S3 options.
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mysecret
  annotations:
     serving.kubeflow.org/s3-endpoint: minio-service.kubeflow:9000 # replace with your s3 endpoint
     serving.kubeflow.org/s3-usehttps: "0" # by default 1, for testing with minio you need to set to 0
type: Opaque
stringData:
# Before KFServing 0.3
  awsAccessKeyID: XXXX
  awsSecretAccessKey: XXXXXXXX
# KFServing 0.4
  AWS_ACCESS_KEY_ID: XXXX
  AWS_SECRET_ACCESS_KEY: XXXXXXXX
```

`KFServing` gets the secrets from your service account, you need to add the above created or existing secret to your service account's secret list.
By default `KFServing` uses `default` service account, user can use own service account and overwrite on `InferenceService` CRD.

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

## Create the InferenceService
Apply the CRD
```bash
kubectl apply -f tensorflow_s3.yaml
```

Expected Output
```
$ inferenceservice.serving.kubeflow.org/mnist-s3 created
```

## Run a prediction
The first step is to [determine the ingress IP and ports](../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```bash
MODEL_NAME=mnist-s3
INPUT_PATH=@./input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice mnist-s3 -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict -d $INPUT_PATH
```
Expected Output
```
*   Trying 34.73.93.217...
* TCP_NODELAY set
* Connected to 34.73.93.217 (34.73.93.217) port 80 (#0)
> POST /v1/models/mnist-s3:predict HTTP/1.1
> Host: mnist-s3.default.svc.cluster.local
> User-Agent: curl/7.54.0
> Accept: */*
> Content-Length: 2052
> Content-Type: application/x-www-form-urlencoded
> Expect: 100-continue
>
< HTTP/1.1 100 Continue
* We are completely uploaded and fine

< HTTP/1.1 200 OK
< content-length: 218
< content-type: application/json
< date: Thu, 23 May 2019 01:33:08 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 20536
<
{
    "predictions": [
        {
            "classes": 2,
            "predictions": [0.28852, 3.02198e-06, 0.484786, 0.123249, 0.000372552, 0.0635331, 0.00168883, 0.00327147, 0.0344911, 8.54185e-05]
        }
    ]
* Connection #0 to host 34.73.93.217 left intact
```

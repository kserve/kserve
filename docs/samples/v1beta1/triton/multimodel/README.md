# Multi Model Serving with Triton(Alpha)
There are growing use cases of developing per-user or per-category ML models instead of cohort model. For example a news classification service trains custom model based on each news category,
a recommendation model trains on each user's usage history to personalize the recommendation. 

While you get the benefit of better inference accuracy by building models for each use case, 
the cost of deploying models increase significantly because you may train anywhere from hundreds to thousands of custom models, and it becomes difficult to manage so many models on production.
These challenges become more pronounced when you don’t access all models at the same time but still need them to be available at all times.
KFServing multi model serving design addresses these issues and gives a scalable yet cost-effective solution to deploy multiple models.

You must set Triton's multiModelServer flag in `inferenceservice.yaml` to true to enable multi-model serving for Triton.

## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing 0.5 installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).
3. Skip [tag resolution](https://knative.dev/docs/serving/tag-resolution/) for `nvcr.io` which requires auth to resolve triton inference server image digest
```bash
kubectl patch cm config-deployment --patch '{"data":{"registriesSkippingTagResolving":"nvcr.io"}}' -n knative-serving
```
4. Increase progress deadline since pulling triton image and big bert model may longer than default timeout for 120s, this setting requires knative 0.15.0+
```bash
kubectl patch cm config-deployment --patch '{"data":{"progressDeadline": "600s"}}' -n knative-serving
```
5. Setup Minio for storing the models as currently model agent sidecar only supports S3 protocol.
```bash
# Setup Minio
kubectl apply -f minio.yaml
kubectl apply -f s3_secret.yaml
kubectl port-forward $(kubectl get pod --selector="app=minio" --output jsonpath='{.items[0].metadata.name}') 9000:9000
mc config host add myminio http://127.0.0.1:9000 minio minio123

# Copy cifar10 model to minio
gsutil cp -r gs://kfserving-examples/models/torchscript/cifar10 .
mc cp -r cifar10 myminio/triton/torchscript

# Copy simple string model to minio
gsutil cp -r gs://kfserving-samples/models/triton/simple_string
mc cp -r simple_string myminio/triton
```

## Create the hosting InferenceService
KFServing `Multi Model Serving` design decouples the trained model artifact from hosting service. User can create a hosting `InferenceService` without `StoragegUri`
and then deploy multiple `TrainedModel` onto the assigned `InferenceService`.

```yaml
apiVersion: "serving.kubeflow.org/v1beta1"
kind: "InferenceService"
metadata:
  name: "triton-mms"
spec:
  predictor:
    triton:
      args:
      - --log-verbose=1     
      resources:
        limits:
          cpu: "1"
          memory: 16Gi
        requests:
          cpu: "1"
          memory: 16Gi
```
Note that you create the hosting `InferenceService` with enough memory resource without specifying the `StorageUri` as single model service initially.

```bash
kubectl apply -f multi_model_triton.yaml
```
Check the `InferenceService` status

```bash
 kubectl get inferenceservice triton-mms
NAME   URL                                                    READY   AGE
triton-mms   http://triton-mms.default.35.229.120.99.xip.io   True    8h
```

## Deploy Trained Model
Now you have an `InferenceService` running with 16Gi memory but no model is loaded on the server yet, let's deploy the model onto the server using `TrainedModel` CR.

On `TrainedModel` CR you specify the model framework you trained with and the `storageUri` where the model is getting stored, last you set the `InferenceService` name you want
the model to deploy onto.

### Deploy Cifar10 Torchscript Model
```yaml
apiVersion: "serving.kubeflow.org/v1alpha1"
kind: "TrainedModel"
metadata:
  name: "cifar10"
spec:
  inferenceService: triton-mms
  model:
    framework: pytorch
    storageUri: s3://triton/torchscript/cifar10
``` 

```bash
kubectl apply -f multi_model_triton.yaml
```

Check the model agent sidecar logs, after the `TrainedModel` CR is created the agent downloads the model from specified `StorageUri` to the mounted
model repository and calls the load endpoint of the model server.
```bash
kubectl logs triton-mms-predictor-default-2g8lg-deployment-69c9964bc4-mfg92 agent

{"level":"info","ts":1605449512.0457177,"logger":"Watcher","msg":"Processing event","event":"\"/mnt/configs/..data\": CREATE"}
{"level":"info","ts":1605449512.0464094,"logger":"modelWatcher","msg":"removing model","modelName":"cifar10"}
{"level":"info","ts":1605449512.0464404,"logger":"modelWatcher","msg":"adding model","modelName":"cifar10"}
{"level":"info","ts":1605449512.046505,"logger":"modelProcessor","msg":"worker is started for","model":"cifar10"}
{"level":"info","ts":1605449512.0465908,"logger":"modelProcessor","msg":"unloading model","modelName":"cifar10"}
{"level":"info","ts":1605449512.0487478,"logger":"modelProcessor","msg":"Downloading model","storageUri":"s3://triton/torchscript/cifar10"}
{"level":"info","ts":1605449512.048788,"logger":"Downloader","msg":"Downloading to model dir","modelUri":"s3://triton/torchscript/cifar10","modelDir":"/mnt/models"}
{"level":"info","ts":1605449512.0488636,"logger":"modelAgent","msg":"Download model ","modelName":"cifar10","storageUri":"s3://triton/torchscript/cifar10","modelDir":"/mnt/models"}
{"level":"info","ts":1605449512.0488217,"logger":"modelOnComplete","msg":"completion event for model","modelName":"cifar10","inFlight":1}
{"level":"info","ts":1605449512.6908782,"logger":"modelOnComplete","msg":"completion event for model","modelName":"cifar10","inFlight":0}
```
Check the `Triton Inference Service` log you will see that the model is loaded into the memory
```bash
I1115 14:11:52.060489 1 model_repository_manager.cc:737] loading: cifar10:1
I1115 14:11:52.061524 1 libtorch_backend.cc:217] Creating instance cifar10_0_0_cpu on CPU using model.pt
I1115 14:11:52.690479 1 model_repository_manager.cc:925] successfully loaded 'cifar10' version 1
```

Now you can curl the model endpoint
```bash
MODEL_NAME=cifar10
INPUT_PATH=@./input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservices triton-mms -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v2/models/$MODEL_NAME

{"name":"cifar10","versions":["1"],"platform":"pytorch_libtorch","inputs":[{"name":"INPUT__0","datatype":"FP32","shape":[-1,3,32,32]}],"outputs":[{"name":"OUTPUT__0","datatype":"FP32","shape":[-1,10]}]}
```

### Deploy Simple String Tensorflow Model
Next let's deploy another model to the same `InferenceService`

```yaml
apiVersion: "serving.kubeflow.org/v1alpha1"
kind: "TrainedModel"
metadata:
  name: "simple_string"
spec:
  inferenceService: triton-mms
  model:
    framework: pytorch
    storageUri: s3://triton/simple_string
``` 

Check the `Triton Inference Service` log you will see that the model is also loaded into the memory
```bash
I1115 14:11:52.060489 1 model_repository_manager.cc:737] loading: cifar10:1
I1115 14:11:52.061524 1 libtorch_backend.cc:217] Creating instance cifar10_0_0_cpu on CPU using model.pt
I1115 14:11:52.690479 1 model_repository_manager.cc:925] successfully loaded 'cifar10' version 1
```

Now you can curl the `simple_string` model endpoint
```bash
MODEL_NAME=simple_string
INPUT_PATH=@./input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservices triton-mms -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v2/models/$MODEL_NAME

{"name":"simple_string","versions":["1"],"platform":"tensorflow_graphdef","inputs":[{"name":"INPUT0","datatype":"BYTES","shape":[-1,16]},{"name":"INPUT1","datatype":"BYTES","shape":[-1,16]}],"outputs":[{"name":"OUTPUT0","datatype":"BYTES","shape":[-1,16]},{"name":"OUTPUT1","datatype":"BYTES","shape":[-1,16]}]}
```

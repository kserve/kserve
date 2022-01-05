# Multi Model Serving with Triton(Alpha)
There are growing use cases of developing per-user or per-category ML models instead of cohort model. For example a news classification service trains custom model based on each news category,
a recommendation model trains on each user's usage history to personalize the recommendation. 

While you get the benefit of better inference accuracy by building models for each use case, 
the cost of deploying models increase significantly because you may train anywhere from hundreds to thousands of custom models, and it becomes difficult to manage so many models on production.
These challenges become more pronounced when you don’t access all models at the same time but still need them to be available at all times.
KFServing multi model serving design addresses these issues and gives a scalable yet cost-effective solution to deploy multiple models.

> :warning: You must set Triton's multiModelServer flag in `inferenceservice.yaml` to true to enable multi-model serving for Triton.

## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing 0.6 installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).
3. Skip [tag resolution](https://knative.dev/docs/serving/tag-resolution/) for `nvcr.io` which requires auth to resolve triton inference server image digest
```bash
kubectl patch cm config-deployment --patch '{"data":{"registriesSkippingTagResolving":"nvcr.io"}}' -n knative-serving
```
4. Increase progress deadline since pulling triton image and big bert model may longer than default timeout for 120s, this setting requires knative 0.15.0+
```bash
kubectl patch cm config-deployment --patch '{"data":{"progressDeadline": "600s"}}' -n knative-serving
```

## Create the hosting InferenceService
KFServing `Multi Model Serving` design decouples the trained model artifact from the hosting `InferenceService`. You first create a hosting `InferenceService` without `StorageUri`
and then deploy multiple `TrainedModel` CRs onto the designated `InferenceService`.

```yaml
apiVersion: "serving.kserve.io/v1beta1"
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
          memory: 2Gi
        requests:
          cpu: "1"
          memory: 2Gi
```
Note that you create the hosting `InferenceService` with enough memory resource for hosting multiple models.

```bash
kubectl apply -f multi_model_triton.yaml
```
Check the `InferenceService` status

```bash
kubectl get isvc triton-mms
NAME         URL                                     READY   PREV   LATEST   PREVROLLEDOUTREVISION   LATESTREADYREVISION                  AGE
triton-mms   http://triton-mms.default.example.com   True           100                              triton-mms-predictor-default-00001   23h
```

## Deploy Trained Model
Now you have an `InferenceService` running with 2Gi memory but no model is deployed on `InferenceService` yet, let's deploy the trained models on `InferenceService` 
by applying the `TrainedModel` CRs.

On `TrainedModel` CR you specify the following fields:
- Hosting `InferenceService`: The `InferenceService` you want the trained model to deploy to.
- Model framework: the ML framework you trained with, the trained model validation webhook validates the framework if it is supported by the model server in this case `Triton Inference Server`.
- Estimated model memory resource: the trained model validation webhook validates that the summed memory of all trained models do no exceed the parent `InferenceService` memory limit.  


### Deploy Cifar10 TorchScript Model
```yaml
apiVersion: "serving.kserve.io/v1alpha1"
kind: "TrainedModel"
metadata:
  name: "cifar10"
spec:
  inferenceService: triton-mms
  model:
    framework: pytorch
    storageUri: gs://kfserving-examples/models/torchscript/cifar10
    memory: 1Gi
``` 

```bash
kubectl apply -f trained_model.yaml
```

After `TrainedModel` CR is applied, check the model agent sidecar logs, the model agent downloads the model from specified `StorageUri` to the mounted
model repository and then calls the load endpoint of `Triton Inference Server`.
```bash
kubectl logs triton-mms-predictor-default-2g8lg-deployment-69c9964bc4-mfg92 agent

{"level":"info","ts":1621717148.93849,"logger":"fallback-logger","caller":"agent/watcher.go:173","msg":"adding model cifar10"}
{"level":"info","ts":1621717148.939014,"logger":"fallback-logger","caller":"agent/puller.go:136","msg":"Worker is started for cifar10"}
{"level":"info","ts":1621717148.9393005,"logger":"fallback-logger","caller":"agent/puller.go:144","msg":"Downloading model from gs://kfserving-examples/models/torchscript/cifar10"}
{"level":"info","ts":1621717148.939781,"logger":"fallback-logger","caller":"agent/downloader.go:48","msg":"Downloading gs://kfserving-examples/models/torchscript/cifar10 to model dir /mnt/models"}
{"level":"info","ts":1621717149.4635677,"logger":"fallback-logger","caller":"agent/downloader.go:68","msg":"Creating successFile /mnt/models/cifar10/SUCCESS.71f376a9daa07a04ae1bd52cbe7f3a2c46ceb701350d9dffc73381df5a230923"}
{"level":"info","ts":1621717149.6402793,"logger":"fallback-logger","caller":"agent/puller.go:161","msg":"Successfully loaded model cifar10"}
{"level":"info","ts":1621717149.6404483,"logger":"fallback-logger","caller":"agent/puller.go:129","msg":"completion event for model cifar10, in flight ops 0"}
```

Check the `Triton Inference Service` log you will see that the model is loaded into the memory.
```bash
I0522 20:59:09.469834 1 model_repository_manager.cc:737] loading: cifar10:1
I0522 20:59:09.471278 1 libtorch_backend.cc:217] Creating instance cifar10_0_0_cpu on CPU using model.pt
I0522 20:59:09.638318 1 model_repository_manager.cc:925] successfully loaded 'cifar10' version 1
```

Check the `TrainedModel` CR status
```bash
kubectl get tm cifar10  
NAME      URL                                                             READY   AGE
cifar10   http://triton-mms.default.example.com/v2/models/cifar10/infer   True    3h45m

# to show more detailed status
kubectl get tm cifar10 -oyaml
status:
  address:
    url: http://triton-mms.default.svc.cluster.local/v2/models/cifar10/infer
  conditions:
  - lastTransitionTime: "2021-05-22T20:56:12Z"
    status: "True"
    type: FrameworkSupported
  - lastTransitionTime: "2021-05-22T20:56:12Z"
    status: "True"
    type: InferenceServiceReady
  - lastTransitionTime: "2021-05-22T20:56:12Z"
    status: "True"
    type: IsMMSPredictor
  - lastTransitionTime: "2021-05-22T20:58:56Z"
    status: "True"
    type: MemoryResourceAvailable
  - lastTransitionTime: "2021-05-22T20:58:56Z"
    status: "True"
    type: Ready
  url: http://triton-mms.default.example.com/v2/models/cifar10/infer
```
> :warning: Currently the trained model CR does not reflect download and load/unload status, it has been worked on in
the issue for adding the [probing component](https://github.com/kubeflow/kfserving/issues/1045).

Now you can curl the model metadata endpoint
```bash
MODEL_NAME=cifar10
SERVICE_HOSTNAME=$(kubectl get inferenceservices triton-mms -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v2/models/$MODEL_NAME

{"name":"cifar10","versions":["1"],"platform":"pytorch_libtorch","inputs":[{"name":"INPUT__0","datatype":"FP32","shape":[-1,3,32,32]}],"outputs":[{"name":"OUTPUT__0","datatype":"FP32","shape":[-1,10]}]}
```

### Deploy Simple String Tensorflow Model
Next let's deploy another model to the same `InferenceService`.
> :warning: The TrainModel resource name must be the same as the model name specified in triton model configuration.


```yaml
apiVersion: "serving.kserve.io/v1alpha1"
kind: "TrainedModel"
metadata:
  name: "simple-string"
spec:
  inferenceService: triton-mms
  model:
    framework: tensorflow
    storageUri: gs://kfserving-examples/models/tensorrt/simple_string
    memory: 1Gi
``` 

Check the `Triton Inference Service` log you will see that the `simple-string` model is also loaded into the memory.
```bash
I0523 00:04:19.298966 1 model_repository_manager.cc:737] loading: simple-string:1
I0523 00:04:20.367808 1 tensorflow.cc:1281] Creating instance simple-string on CPU using model.graphdef
I0523 00:04:20.497748 1 model_repository_manager.cc:925] successfully loaded 'simple-string' version 1
```

Check the `TrainedModel` CR status
```bash
kubectl get tm simple-string
NAME            URL                                                                   READY   AGE
simple-string   http://triton-mms.default.example.com/v2/models/simple-string/infer   True    20h

# to show more detailed status
kubectl get tm cifar10 -oyaml
status:
  address:
    url: http://triton-mms.default.svc.cluster.local/v2/models/simple-string/infer
  conditions:
  - lastTransitionTime: "2021-05-23T00:02:42Z"
    status: "True"
    type: FrameworkSupported
  - lastTransitionTime: "2021-05-23T00:02:42Z"
    status: "True"
    type: InferenceServiceReady
  - lastTransitionTime: "2021-05-23T00:02:42Z"
    status: "True"
    type: IsMMSPredictor
  - lastTransitionTime: "2021-05-23T00:02:42Z"
    status: "True"
    type: MemoryResourceAvailable
  - lastTransitionTime: "2021-05-23T00:02:42Z"
    status: "True"
    type: Ready
  url: http://triton-mms.default.example.com/v2/models/simple-string/infer
```

Now you can curl the `simple-string` model metadata endpoint
```bash
MODEL_NAME=simple-string
SERVICE_HOSTNAME=$(kubectl get inferenceservices triton-mms -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v2/models/$MODEL_NAME

{"name":"simple-string","versions":["1"],"platform":"tensorflow_graphdef","inputs":[{"name":"INPUT0","datatype":"BYTES","shape":[-1,16]},{"name":"INPUT1","datatype":"BYTES","shape":[-1,16]}],"outputs":[{"name":"OUTPUT0","datatype":"BYTES","shape":[-1,16]},{"name":"OUTPUT1","datatype":"BYTES","shape":[-1,16]}]}
```

### Run Performance Test
The performance job runs vegeta load testing to the `MultiModelInferenceService` with model `cifar10`.
```bash
kubectl create -f perf.yaml
Requests      [total, rate, throughput]         600, 10.02, 10.01
Duration      [total, attack, wait]             59.912s, 59.9s, 11.755ms
Latencies     [min, mean, 50, 90, 95, 99, max]  5.893ms, 11.262ms, 10.252ms, 16.077ms, 18.804ms, 26.745ms, 39.202ms
Bytes In      [total, mean]                     189000, 315.00
Bytes Out     [total, mean]                     66587400, 110979.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      200:600  
Error Set:
```

### Homogeneous model allocation and Autoscaling
The current MMS implementation uses the homogeneous approach meaning that each `InferenceService` replica holds the same
set of models. Autoscaling is based on the aggregated traffic for this set of models NOT the request volume for individual
model, this set of models is always scaled up together. The downside of this approach is that traffic spike for one model
can result in scaling out entire set of models despite the low request volume for other models hosted on the same `InferenceService`,
this may not be desirable for `InferenceService` that hosts a set of big models. The other approach is to use the heterogeneous
allocation where each replica can host a different set of models, models are scaled up/down individually based on its own
traffic and in this way it ensures better resource utilization.

### Delete Trained Models
To remove the resources, run the command `kubectl delete inferenceservice triton-mms`. 
This will delete the inference service and result in the trained models deleted. To delete individual `TrainedModel` you
can run the command `kubectl delete tm $MODEL_NAME`.

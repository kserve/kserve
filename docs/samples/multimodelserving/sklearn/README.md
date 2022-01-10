# Multi-Model Serving with Sklearn

## Overview

The general overview of multi-model serving:
1. Deploy InferenceService with the framework specified
2. Deploy TrainedModel(s) with the storageUri, framework, and memory
3. A config map will be created and will contain details about each trained model
4. Model Agent loads model from the model config
5. An endpoint is set up and is ready to serve model(s)
6. Deleting a model leads to removing model from config map which causes the model agent to unload the model
7. Deleting the InferenceService causes the TrainedModel(s) to be deleted

> :warning: You must set SKLearn's multiModelServer flag in `inferenceservice.yaml` to true to enable multi-model serving for SKLearn.

## Example
Firstly, you should have kfserving installed. Check [this](https://github.com/kubeflow/kfserving#install-kfserving) out if you have not installed kfserving.

The content below is in the file `inferenceservice.yaml`.

```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "sklearn-iris-example"
spec:
  predictor:
    minReplicas: 1
    sklearn:
      protocolVersion: v1
      name: "sklearn-iris-predictor"
      resources:
        limits:
          cpu: 100m
          memory: 512Mi
        requests:
          cpu: 100m
          memory: 512Mi
```
Run the command `kubectl apply -f inferenceservice.yaml` to create the inference service. Check if the service is properly deployed by running `kubectl get inferenceservice`. The output should be similar to the below.
```yaml
NAME                   URL                                               READY   PREV   LATEST   PREVROLLEDOUTREVISION   LATESTREADYREVISION                            AGE
sklearn-iris-example   http://sklearn-iris-example.default.example.com   True           100                              sklearn-iris-example-predictor-default-kgtql   22s
```

Next, the other file the trained models `trainedmodels.yaml` is shown below.
```yaml
apiVersion: "serving.kserve.io/v1alpha1"
kind: "TrainedModel"
metadata:
  name: "model1-sklearn"
spec:
  inferenceService: "sklearn-iris-example"
  model:
    storageUri: "gs://kfserving-samples/models/sklearn/iris"
    framework: "sklearn"
    memory: "256Mi"
---
apiVersion: "serving.kserve.io/v1alpha1"
kind: "TrainedModel"
metadata:
  name: "model2-sklearn"
spec:
  inferenceService: "sklearn-iris-example"
  model:
    storageUri: "gs://kfserving-samples/models/sklearn/iris"
    framework: "sklearn"
    memory: "256Mi"
```
Run the command `kubectl apply -f trainedmodels.yaml` to create the trained models. Run `kubectl get trainedmodel` to view the resource.

Run `kubectl get po` to get the name of the predictor pod. The name should be similar to sklearn-iris-example-predictor-default-xxxxx-deployment-xxxxx.

Run `kubectl logs <name-of-predictor-pod> -c agent` to check if the models are properly loaded. You should get the same output as below. Wait a few minutes and try again if you do not see "Downloading model".
```yaml
{"level":"info","ts":"2021-01-20T16:24:00.421Z","caller":"agent/puller.go:129","msg":"Downloading model from gs://kfserving-samples/models/sklearn/iris"}
{"level":"info","ts":"2021-01-20T16:24:00.421Z","caller":"agent/downloader.go:47","msg":"Downloading gs://kfserving-samples/models/sklearn/iris to model dir /mnt/models"}
{"level":"info","ts":"2021-01-20T16:24:00.424Z","caller":"agent/puller.go:121","msg":"Worker is started for model1-sklearn"}
{"level":"info","ts":"2021-01-20T16:24:00.424Z","caller":"agent/puller.go:129","msg":"Downloading model from gs://kfserving-samples/models/sklearn/iris"}
{"level":"info","ts":"2021-01-20T16:24:00.424Z","caller":"agent/downloader.go:47","msg":"Downloading gs://kfserving-samples/models/sklearn/iris to model dir /mnt/models"}
{"level":"info","ts":"2021-01-20T16:24:09.255Z","caller":"agent/puller.go:146","msg":"Successfully loaded model model2-sklearn"}
{"level":"info","ts":"2021-01-20T16:24:09.256Z","caller":"agent/puller.go:114","msg":"completion event for model model2-sklearn, in flight ops 0"}
{"level":"info","ts":"2021-01-20T16:24:09.260Z","caller":"agent/puller.go:146","msg":"Successfully loaded model model1-sklearn"}
{"level":"info","ts":"2021-01-20T16:24:09.260Z","caller":"agent/puller.go:114","msg":"completion event for model model1-sklearn, in flight ops 0"}
```

Run the command `kubectl get cm modelconfig-sklearn-iris-example-0 -oyaml` to get the configmap. The output should be similar to the below.
```yaml
apiVersion: v1
data:
   models.json: '[{"modelName":"model1-sklearn","modelSpec":{"storageUri":"gs://kfserving-samples/models/sklearn/iris","framework":"sklearn","memory":"256Mi"}},{"modelName":"model2-sklearn","modelSpec":{"storageUri":"gs://kfserving-samples/models/sklearn/iris","framework":"sklearn","memory":"256Mi"}}]'
kind: ConfigMap
metadata:
   creationTimestamp: "2021-01-20T16:22:52Z"
   name: modelconfig-sklearn-iris-example-0
   namespace: default
   ownerReferences:
      - apiVersion: serving.kserve.io/v1beta1
        blockOwnerDeletion: true
        controller: true
        kind: InferenceService
        name: sklearn-iris-example
        uid: f91d8414-0bfa-4182-af25-5d0c1a7eff4e
   resourceVersion: "1958556"
   selfLink: /api/v1/namespaces/default/configmaps/modelconfig-sklearn-iris-example-0
   uid: 79e68f80-e31a-419b-994b-14a6159d8cc2
```

The models will be ready to serve once they are successfully loaded.

Check to see which case applies to you.

If the EXTERNAL-IP value is set, your environment has an external load balancer that you can use for the ingress gateway. Set them by running: 
````bash
export INGRESS_HOST=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
export INGRESS_PORT=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.spec.ports[?(@.name=="http2")].port}')
export SERVICE_HOSTNAME=$(kubectl get inferenceservice sklearn-iris-example -n default -o jsonpath='{.status.url}' | cut -d "/" -f 3)
````

If the EXTERNAL-IP is none, and you can access the gateway using the service's node port:
```bash
# GKE
export INGRESS_HOST=worker-node-address
# Minikube
export INGRESS_HOST=$(minikube ip)å
# Other environment(On Prem)
export INGRESS_HOST=$(kubectl get po -l istio=ingressgateway -n istio-system -o jsonpath='{.items[0].status.hostIP}')
export INGRESS_PORT=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.spec.ports[?(@.name=="http2")].nodePort}')
```

For KIND/Port Forwarding:
- Run `kubectl port-forward -n istio-system svc/istio-ingressgateway 8080:80`
- In a different window, run:
   ```bash
   export INGRESS_HOST=localhost
   export INGRESS_PORT=8080
   export SERVICE_HOSTNAME=$(kubectl get inferenceservice sklearn-iris-example -n default -o jsonpath='{.status.url}' | cut -d "/" -f 3)
   ```


After setting up the above:
- Go to the root directory of `kfserving`
- Query the two models:
  - Curl from ingress gateway:
     ```bash
     curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/model1-sklearn:predict -d @./docs/samples/v1alpha2/sklearn/iris-input.json
     curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/model2-sklearn:predict -d @./docs/samples/v1alpha2/sklearn/iris-input.json
     ```
  - Curl from local cluster gateway
    ```
    curl -v http://sklearn-iris-example.default/v1/models/model1-sklearn:predict -d @./docs/samples/v1alpha2/sklearn/iris-input.json
    curl -v http://sklearn-iris-example.default/v1/models/model2-sklearn:predict -d @./docs/samples/v1alpha2/sklearn/iris-input.json
     ```

The outputs should be
```yaml
{"predictions": [1, 1]}*
```

To remove the resources, run the command `kubectl delete inferenceservice sklearn-iris-example`. This will delete the inference service and result in the trained models being deleted.

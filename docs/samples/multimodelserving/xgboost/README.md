# Multi-Model Serving with XGBoost

## Overview

The general overview of multi-model serving:
1. Deploy InferenceService with the framework specified
2. Deploy TrainedModel(s) with the storageUri, framework, and memory
3. A config map will be created and will contain details about each trained model
4. Model Agent loads model from the model config
5. An endpoint is set up and is ready to serve model(s)
6. Deleting a model leads to removing model from config map which causes the model agent to unload the model
7. Deleting the InferenceService causes the TrainedModel(s) to be deleted

> :warning: You must set XGBoost's multiModelServer flag in `inferenceservice.yaml` to true to enable multi-model serving for XGBoost.

## Example
Firstly, you should have kfserving installed. Check [this](https://github.com/kubeflow/kfserving#install-kfserving) out if you have not installed kfserving.

The content below is in the file `inferenceservice.yaml`.

```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "xgboost-mms"
spec:
  predictor:
    minReplicas: 1
    xgboost:
      protocolVersion: v1
      #name: "xgboost-mms-predictor"
      resources:
        limits:
          cpu: "1"
          memory: 2Gi
        requests:
          cpu: "1"
          memory: 2Gi
```

Run the command `kubectl create namespace <namespace_name>` to create namespace.

Run the command `kubectl apply -f inferenceservice.yaml -n <namespace_name>` to create the inference service. Check if the service is properly deployed by running `kubectl get inferenceservice -n <namespace_name>`. The output should be similar to the below.


NAME  | URL | READY | PREV | LATEST | PREVROLLEDOUTREVISION | LATESTREADYREVISION | AGE 
--- | --- | --- | --- |--- |--- |--- |--- 
xgboost-mms | http://xgboost-mms.ns-xgboost.example.com | True |  | 100 |  | xgboost-mms-predictor-00001 | 13m 


Next, the other file the trained models `trainedmodels.yaml` is shown below.
```yaml
apiVersion: "serving.kserve.io/v1alpha1"
kind: "TrainedModel"
metadata:
  name: "xgboost-iris"
spec:
  inferenceService: "xgboost-mms"
  model:
    framework: "xgboost"
    storageUri: "gs://kfserving-examples/models/xgboost/iris"
    memory: "1Gi"  
---
apiVersion: "serving.kserve.io/v1alpha1"
kind: "TrainedModel"
metadata:
  name: "xgboost-model2"
spec:
  inferenceService: "xgboost-mms"
  model:
    framework: "xgboost"
    storageUri: "gs://kfserving-examples/models/xgboost/1.5/model"
    memory: "1Gi"
```
Run the command `kubectl apply -f trained_models.yaml -n <namespace_name>` to create the trained models. Run `kubectl get trainedmodel -n <namespace_name>` to view the resource.

Run `kubectl get po -n <namespace_name>` to get the name of the predictor pod. The name should be similar to xgboost-mms-predictor-xxxxx-deployment-xxxxxxxxxx-xxxxx.

Run `kubectl logs <name-of-predictor-pod> -c agent` to check if the models are properly loaded. You should get the same output as below. Wait a few minutes and try again if you do not see "Downloading model".
```yaml
{"severity":"INFO","timestamp":"2023-10-28T14:10:17.062058117Z","caller":"agent/puller.go:151","message":"Downloading model from gs://kfserving-examples/models/xgboost/1.5/model"}
{"severity":"INFO","timestamp":"2023-10-28T14:10:17.062296714Z","caller":"agent/downloader.go:47","message":"Downloading gs://kfserving-examples/models/xgboost/1.5/model to model dir /mnt/models"}
{"severity":"INFO","timestamp":"2023-10-28T14:10:17.062260166Z","caller":"agent/puller.go:143","message":"Worker is started for xgboost-iris"}
{"severity":"INFO","timestamp":"2023-10-28T14:10:17.063509962Z","caller":"agent/puller.go:151","message":"Downloading model from gs://kfserving-examples/models/xgboost/iris"}
{"severity":"INFO","timestamp":"2023-10-28T14:10:17.064881628Z","caller":"agent/downloader.go:47","message":"Downloading gs://kfserving-examples/models/xgboost/iris to model dir /mnt/models"}
{"severity":"INFO","timestamp":"2023-10-28T14:10:18.119968135Z","caller":"agent/downloader.go:67","message":"Creating successFile /mnt/models/xgboost-iris/SUCCESS.dd490f479a3c51035d10ec027165a6d841d38966c130b63d4fb8f813dac83303"}
{"severity":"INFO","timestamp":"2023-10-28T14:10:18.12647361Z","caller":"agent/puller.go:168","message":"Successfully loaded model xgboost-iris"}
{"severity":"INFO","timestamp":"2023-10-28T14:10:18.127099047Z","caller":"agent/puller.go:136","message":"completion event for model xgboost-iris, in flight ops 0"}
{"severity":"INFO","timestamp":"2023-10-28T14:10:18.221908262Z","caller":"agent/downloader.go:67","message":"Creating successFile /mnt/models/xgboost-model2/SUCCESS.a98143f655153e2a53ef4ebdcba6af07e27cfffe569d8f75c3e3c70e4e5bc2d0"}
{"severity":"INFO","timestamp":"2023-10-28T14:10:18.226566378Z","caller":"agent/puller.go:168","message":"Successfully loaded model xgboost-model2"}
{"severity":"INFO","timestamp":"2023-10-28T14:10:18.226610648Z","caller":"agent/puller.go:136","message":"completion event for model xgboost-model2, in flight ops 0"}
```
Run the command `kubectl get cm -n <namespace_name>` to get name of configmap.

Run the command `kubectl get cm <configmap_name> -n <namespace_name> -oyaml` to get the configmap. The output should be similar to the below. In this case it is `kubectl get cm modelconfig-xgboost-mms-0 -n xgboost-ns -oyaml`.
```yaml
apiVersion: v1
data:
  models.json: '[{"modelName":"xgboost-model2","modelSpec":{"storageUri":"gs://kfserving-examples/models/xgboost/1.5/model","framework":"xgboost","memory":"1Gi"}},{"modelName":"xgboost-iris","modelSpec":{"storageUri":"gs://kfserving-examples/models/xgboost/iris","framework":"xgboost","memory":"1Gi"}}]'
kind: ConfigMap
metadata:
  creationTimestamp: "2023-10-28T14:08:56Z"
  name: modelconfig-xgboost-mms-0
  namespace: xgboost-ns
  ownerReferences:
  - apiVersion: serving.kserve.io/v1beta1
    blockOwnerDeletion: true
    controller: true
    kind: InferenceService
    name: xgboost-mms
    uid: ac17eb04-d3c8-427b-b2f3-999827965d28
  resourceVersion: "1415162"
  uid: 91c246be-e576-42cb-80f6-ee8c5d5bbe5a
```

The models will be ready to serve once they are successfully loaded.

Check to see which case applies to you.

If the EXTERNAL-IP value is set, your environment has an external load balancer that you can use for the ingress gateway. Set them by running: 
````bash
export INGRESS_HOST=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')
export INGRESS_PORT=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.spec.ports[?(@.name=="http2")].port}')
export SERVICE_HOSTNAME=$(kubectl get inferenceservice xgboost-mms -n xgboost-ns -o jsonpath='{.status.url}' | cut -d "/" -f 3)
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
   export SERVICE_HOSTNAME=$(kubectl get inferenceservice xgboost-mms -n <namespace_name> -o jsonpath='{.status.url}' | cut -d "/" -f 3)
   ```


After setting up the above:
- Go to the root directory of `kfserving`
- Query the two models:
  - Curl from ingress gateway:
     ```bash
     curl -v -H "Host: ${SERVICE_HOSTNAME}" -H "Content-Type: application/json" http://${INGRESS_HOST}:${INGRESS_PORT}/v2/models/xgboost-iris/infer -d @./docs/samples/v1beta1/xgboost/iris-input.json
     curl -v -H "Host: ${SERVICE_HOSTNAME}" -H "Content-Type: application/json" http://${INGRESS_HOST}:${INGRESS_PORT}/v2/models/xgboost-model2/infer -d @./docs/samples/v1beta1/xgboost/iris-input.json
     ```

  - Curl from local cluster gateway
    ```
    curl -v http://xgboost-mms.xgboost-ns/v2/models/xgboost-iris/infer -d @./docs/samples/v1beta1/xgboost/iris-input.json
    curl -v http://xgboost-mms.xgboost-ns/v2/models/xgboost-model2/infer -d @./docs/samples/v1beta1/xgboost/iris-input.json
     ```

The outputs should be
```yaml
{"model_name":"xgboost-model2","model_version":null,"id":"e5eb7972-0006-4fa3-a3e0-088f101cb98d","parameters":null,"outputs":[{"name":"output-0","shape":[2],"datatype":"FP32","parameters":null,"data":[1.0,1.0]}]}
```

[1.0,1.0] is the output of model

To remove the resources, run the command `kubectl delete inferenceservice xgboost-mms -n <namespace_name>`. This will delete the inference service and result in the trained models being deleted.

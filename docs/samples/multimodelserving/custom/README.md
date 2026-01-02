# Multi-Model Serving with Custom Model Server

## Overview

The general overview of multi-model serving with a custom model server:
1. Deploy InferenceService with a custom container that implements the V2 protocol
2. Deploy TrainedModel(s) with the storageUri, framework, and memory
3. A config map will be created and will contain details about each trained model
4. Model Agent loads model from the model config by calling the model server's load endpoint
5. An endpoint is set up and is ready to serve model(s)
6. Deleting a model leads to removing model from config map which causes the model agent to unload the model
7. Deleting the InferenceService causes the TrainedModel(s) to be deleted

## Requirements for Custom Model Server

For multi-model serving to work with a custom model server, your server must implement the [KServe V2 Protocol](https://github.com/kserve/kserve/tree/master/docs/predict-api/v2), specifically:

1. **Load endpoint**: `POST /v2/repository/models/{model_name}/load`
   - Called by the model agent to load a model into memory
   - The model files will be available at the configured model directory

2. **Unload endpoint**: `POST /v2/repository/models/{model_name}/unload`
   - Called by the model agent to unload a model from memory

3. **Model Ready endpoint**: `GET /v2/models/{model_name}/ready`
   - Used to check if a model is loaded and ready for inference

4. **Inference endpoint**: `POST /v2/models/{model_name}/infer`
   - The main inference endpoint for predictions

## Example

Firstly, you should have KServe installed. Check [this](https://github.com/kserve/kserve#readme) out if you have not installed KServe.

### Create the InferenceService

The content below is in the file `inferenceservice.yaml`.

```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "custom-model-mms"
spec:
  predictor:
    containers:
      - name: kserve-container
        image: ${CUSTOM_MODEL_SERVER_IMAGE}
        resources:
          limits:
            cpu: "1"
            memory: 2Gi
          requests:
            cpu: "1"
            memory: 2Gi
```

Replace `${CUSTOM_MODEL_SERVER_IMAGE}` with your custom model server image that implements the V2 protocol.

Note that you create the hosting `InferenceService` with enough memory resource for hosting multiple models but without a `storageUri` field.

Run the command `kubectl apply -f inferenceservice.yaml` to create the inference service. Check if the service is properly deployed by running `kubectl get inferenceservice`. The output should be similar to the below.

```yaml
NAME               URL                                            READY   PREV   LATEST   PREVROLLEDOUTREVISION   LATESTREADYREVISION                       AGE
custom-model-mms   http://custom-model-mms.default.example.com    True           100                              custom-model-mms-predictor-default-xxxxx  22s
```

### Deploy TrainedModels

Next, deploy the trained models using `trainedmodels.yaml`.

```yaml
apiVersion: "serving.kserve.io/v1alpha1"
kind: "TrainedModel"
metadata:
  name: "model1-custom"
spec:
  inferenceService: "custom-model-mms"
  model:
    storageUri: "${MODEL1_STORAGE_URI}"
    framework: "custom"
    memory: "256Mi"
---
apiVersion: "serving.kserve.io/v1alpha1"
kind: "TrainedModel"
metadata:
  name: "model2-custom"
spec:
  inferenceService: "custom-model-mms"
  model:
    storageUri: "${MODEL2_STORAGE_URI}"
    framework: "custom"
    memory: "256Mi"
```

Replace `${MODEL1_STORAGE_URI}` and `${MODEL2_STORAGE_URI}` with the storage URIs of your models (e.g., `gs://your-bucket/models/model1`, `s3://your-bucket/models/model2`).

Run the command `kubectl apply -f trainedmodels.yaml` to create the trained models. Run `kubectl get trainedmodel` to view the resources.

```bash
kubectl get trainedmodel
NAME            URL                                                               READY   AGE
model1-custom   http://custom-model-mms.default.example.com/v2/models/model1-custom/infer   True    30s
model2-custom   http://custom-model-mms.default.example.com/v2/models/model2-custom/infer   True    30s
```

### Verify Model Loading

Run `kubectl get po` to get the name of the predictor pod. The name should be similar to `custom-model-mms-predictor-default-xxxxx-deployment-xxxxx`.

Run `kubectl logs <name-of-predictor-pod> -c agent` to check if the models are properly loaded. You should see logs similar to:

```
{"level":"info","ts":"2021-01-20T16:24:00.421Z","caller":"agent/puller.go:129","msg":"Downloading model from ${MODEL1_STORAGE_URI}"}
{"level":"info","ts":"2021-01-20T16:24:09.260Z","caller":"agent/puller.go:146","msg":"Successfully loaded model model1-custom"}
{"level":"info","ts":"2021-01-20T16:24:00.421Z","caller":"agent/puller.go:129","msg":"Downloading model from ${MODEL2_STORAGE_URI}"}
{"level":"info","ts":"2021-01-20T16:24:09.260Z","caller":"agent/puller.go:146","msg":"Successfully loaded model model2-custom"}
```

### Query the Models

Check to see which case applies to you for setting up ingress.

If the EXTERNAL-IP value is set, your environment has an external load balancer that you can use for the ingress gateway:
```bash
export INGRESS_HOST=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
export INGRESS_PORT=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.spec.ports[?(@.name=="http2")].port}')
export SERVICE_HOSTNAME=$(kubectl get inferenceservice custom-model-mms -n default -o jsonpath='{.status.url}' | cut -d "/" -f 3)
```

If the EXTERNAL-IP is none, and you can access the gateway using the service's node port:
```bash
# GKE
export INGRESS_HOST=worker-node-address
# Minikube
export INGRESS_HOST=$(minikube ip)
# Other environment (On Prem)
export INGRESS_HOST=$(kubectl get po -l istio=ingressgateway -n istio-system -o jsonpath='{.items[0].status.hostIP}')
export INGRESS_PORT=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.spec.ports[?(@.name=="http2")].nodePort}')
```

For KIND/Port Forwarding:
- Run `kubectl port-forward -n istio-system svc/istio-ingressgateway 8080:80`
- In a different window, run:
  ```bash
  export INGRESS_HOST=localhost
  export INGRESS_PORT=8080
  export SERVICE_HOSTNAME=$(kubectl get inferenceservice custom-model-mms -n default -o jsonpath='{.status.url}' | cut -d "/" -f 3)
  ```

After setting up the above, query the models using the V2 inference protocol:

```bash
# Query model1-custom
curl -v -H "Host: ${SERVICE_HOSTNAME}" \
  http://${INGRESS_HOST}:${INGRESS_PORT}/v2/models/model1-custom/infer \
  -d '{"inputs": [{"name": "input-0", "shape": [1], "datatype": "FP32", "data": [1.0]}]}'

# Query model2-custom
curl -v -H "Host: ${SERVICE_HOSTNAME}" \
  http://${INGRESS_HOST}:${INGRESS_PORT}/v2/models/model2-custom/infer \
  -d '{"inputs": [{"name": "input-0", "shape": [1], "datatype": "FP32", "data": [2.0]}]}'
```

Or from local cluster gateway:
```bash
curl -v http://custom-model-mms.default/v2/models/model1-custom/infer \
  -d '{"inputs": [{"name": "input-0", "shape": [1], "datatype": "FP32", "data": [1.0]}]}'
```

## Cleanup

To remove the resources, run the command:
```bash
kubectl delete inferenceservice custom-model-mms
```

This will delete the inference service and result in the trained models being deleted as well.

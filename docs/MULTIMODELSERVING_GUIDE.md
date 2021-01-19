# Multi Model Serving

### Example

Firstly, you should have kfserving installed. Check [this](https://github.com/kubeflow/kfserving#install-kfserving) out if you have not installed kfserving.

Copy the content below into a file `inferenceservice.yaml`.

```yaml
apiVersion: "serving.kubeflow.org/v1beta1"
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
          memory: 256Mi
        requests:
          cpu: 100m
          memory: 256Mi
```
Run the command `kubectl apply -f inferenceservice.yaml` to create the inference service. Check if the service is properly deployed by running `k get inferenceservice`. The output should be similar to the below.
```yaml
NAME                   URL                                               READY   PREV   LATEST   PREVROLLEDOUTREVISION   LATESTREADYREVISION                            AGE
sklearn-iris-example   http://sklearn-iris-example.default.example.com   True           100                              sklearn-iris-example-predictor-default-kgtql   22s
```

Next, create another file for the trained models `trainedmodels.yaml` and copy the content below.
```yaml
apiVersion: "serving.kubeflow.org/v1alpha1"
kind: "TrainedModel"
metadata:
  name: "model1"
spec:
  inferenceService: "sklearn-iris-example"
  model:
    storageUri: "gs://kfserving-samples/models/sklearn/iris"
    framework: "sklearn"
    memory: "256Mi"
---
apiVersion: "serving.kubeflow.org/v1alpha1"
kind: "TrainedModel"
metadata:
  name: "model2"
spec:
  inferenceService: "sklearn-iris-example"
  model:
    storageUri: "gs://kfserving-samples/models/sklearn/iris"
    framework: "sklearn"
    memory: "256Mi"
```
Run the command `kubectl apply -f trainedmodels.yaml` to create the trained models. Run `k get trainedmodel` to view the resource.

Run `kubectl logs <name-of-predictor-pod> -c agent` to check if the models are properly loaded. You should get the same output as below. Run `k get po` to get the name of the predictor pod. The name should be similar to sklearn-iris-example-predictor-default-xxxxx-deployment-xxxxx.

```yaml
{"level":"info","ts":"2021-01-19T15:04:53.503Z","caller":"agent/puller.go:146","msg":"Successfully loaded model model1"}
{"level":"info","ts":"2021-01-19T15:04:53.503Z","caller":"agent/puller.go:114","msg":"completion event for model model1, in flight ops 0"}
{"level":"info","ts":"2021-01-19T15:04:53.599Z","caller":"agent/puller.go:146","msg":"Successfully loaded model model2"}
{"level":"info","ts":"2021-01-19T15:04:53.599Z","caller":"agent/puller.go:114","msg":"completion event for model model2, in flight ops 0"}
```

Run the command `kubectl get cm modelconfig-sklearn-iris-example-0 -oyaml` to get the configmap. The output should be similar to the below.
```yaml
apiVersion: v1
data:
  models.json: '[{"modelName":"model1","modelSpec":{"storageUri":"gs://kfserving-samples/models/sklearn/iris","framework":"sklearn","memory":"256Mi"}},{"modelName":"model2","modelSpec":{"storageUri":"gs://kfserving-samples/models/sklearn/iris","framework":"sklearn","memory":"256Mi"}}]'
kind: ConfigMap
metadata:
  creationTimestamp: "2021-01-19T15:02:09Z"
  name: modelconfig-sklearn-iris-example-0
  namespace: default
  ownerReferences:
    - apiVersion: serving.kubeflow.org/v1beta1
      blockOwnerDeletion: true
      controller: true
      kind: InferenceService
      name: sklearn-iris-example
      uid: 6ff54022-4287-4000-b96e-97abcc13c8f2
  resourceVersion: "1553737"
  selfLink: /api/v1/namespaces/default/configmaps/modelconfig-sklearn-iris-example-0
  uid: 0756af76-413c-4c86-a071-f2413989fe11
```

The models will be ready to serve once they are successfully loaded.

For KIND/minikube:
1. Run `kubectl port-forward -n istio-system svc/istio-ingressgateway 8080:80`
2. In a different window, run:
    1. `export INGRESS_HOST=localhost`
    2. `export INGRESS_PORT=8080`
   3. `export SERVICE_HOSTNAME=$(kubectl get inferenceservice sklearn-iris-example -n default -o jsonpath='{.status.url}' | cut -d "/" -f 3)`
3. Go to the root directory of `kfserving`
4. Query the two models by running:
    1. `curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/model1:predict -d @./docs/samples/v1alpha2/sklearn/iris-input.json`
    2. `curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/model2:predict -d @./docs/samples/v1alpha2/sklearn/iris-input.json`

The outputs should be
```yaml
{"predictions": [1, 1]}* Closing connection 0
```

To remove the resources, run the command `kubectl delete inferenceservice sklearn-iris-example`. This will delete the inference service and result in the trained models being deleted.
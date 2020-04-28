# Inference Batcher

We add this module to support batching predict for any ML frameworks (TensorFlow, PyTorch, ...) without decreasing the performance.

This batcher module also support customized images.

![Batcher](../../diagrams/batcher.jpg)

* We use webhook to inject the batcher container into the InferenceService. 
* We choose beego web framework to accept user requests.
* We use channels to transfer data between go routines.
* We use HTTP connections between containers. In the future, We may use RPC.
* Users can choose to use the batcher or not by changing the yaml file of InferenceService.
* When the number of instances (For example, the number of pictures) reaches the maxBatchsize or the latency meets the maxLatency, a batching predict will be triggered.
```
apiVersion: "serving.kubeflow.org/v1alpha2"
kind: "InferenceService"
metadata:
  name: "sklearn-iris"
spec:
  default:
    predictor:
      minReplicas: 1
      batcher:
        port: "8082"
        svcHost: "127.0.0.1"
        svcPort: "8080"
        maxBatchsize: "32"
        maxLatency: "1.0"
      sklearn:
        storageUri: "gs://kfserving-samples/models/sklearn/iris"
```
* port: the port of inferenceservice-batcher container.
* svcHost: the host of kfserving-container (This value should always be "127.0.0.1").
* svcPort: the port of kfserving-container.
* maxBatchsize: the max batch size for predict.
* maxLatency: the max latency for predict (In milliseconds).

All of the bellowing fields have default values in the code. You can config them or not as you wish.
* port: "8082".
* svcHost: "127.0.0.1".
* svcPort: "8080".
* maxBatchsize: "32".
* maxLatency: "1.0".

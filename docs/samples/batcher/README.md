# Inference Batcher

We add this module to support batching predict for any ML frameworks (TensorFlow, PyTorch, ...) without decreasing the performance.

This batcher module also support customized images.

![Batcher](../../diagrams/batcher.jpg)

* We use webhook to inject the batcher container into the InferenceService. 
* We choose Beego web framework to accept user requests.
* We use channels to transfer data between go routines.
* We use HTTP connections between containers. In the future, we may use RPC.
* Users can choose to use the batcher or not by changing the yaml file of InferenceService.
* When the number of instances (For example, the number of pictures) reaches the maxBatchSize or the latency meets the maxLatency, a batching predict will be triggered.
```
apiVersion: "serving.kubeflow.org/v1alpha2"
kind: "InferenceService"
metadata:
  name: "pytorch-cifar10"
spec:
  default:
    predictor:
      minReplicas: 1
      batcher:
        maxBatchSize: "32"
        maxLatency: "1.0"
        timeout: "60"
      pytorch:
        storageUri: "gs://kfserving-samples/models/pytorch/cifar10/"
```
* port: the port of inferenceservice-batcher container.
* maxBatchSize: the max batch size for predict.
* maxLatency: the max latency for predict (In milliseconds).
* timeout: timeout of calling predictor service (In seconds).

All of the bellowing fields have default values in the code. You can config them or not as you wish.
* maxBatchSize: "32".
* maxLatency: "1.0".
* timeout: "60".

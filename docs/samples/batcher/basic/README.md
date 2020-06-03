# Inference Batcher Demo

We first create a sklearn predictor with a batcher. The "maxLatency" is set to a big value (5000.0 milliseconds) to make us be able to observe the batching process.

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
        maxBatchSize: "32"
        maxLatency: "5000.0"
        timeout: "60"
      sklearn:
        storageUri: "gs://kfserving-samples/models/sklearn/iris"
```

Let's apply this yaml:

```
kubectl create -f sklearn-batcher.yaml
```

We can now send 2 requests to the sklearn model under 2 ssh terminal tabs. Use `kfserving-ingressgateway` as your `INGRESS_GATEWAY` if you are deploying KFServing as part of Kubeflow install, and not independently.

```
MODEL_NAME=sklearn-iris
INPUT_PATH=@./iris-input.json
INGRESS_GATEWAY=istio-ingressgateway
CLUSTER_IP=$(kubectl -n istio-system get service $INGRESS_GATEWAY -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
SERVICE_HOSTNAME=$(kubectl get inferenceservice sklearn-iris -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://$CLUSTER_IP/v1/models/$MODEL_NAME:predict -d $INPUT_PATH -H "Content-Type: application/json"
```

The request will go to the batcher container first, and then the batcher container will do batching and send the batching request to the predictor container.

Notice: If the interval of sending the two requests is less than "maxLatency", the returned "batchId" will be the same.

Expected Output for each ssh terminal tab.

```
*   Trying 169.63.251.68...
* TCP_NODELAY set
* Connected to 169.63.251.68 (169.63.251.68) port 80 (#0)
> POST /models/sklearn-iris:predict HTTP/1.1
> Host: sklearn-iris.default.svc.cluster.local
> User-Agent: curl/7.60.0
> Accept: */*
> Content-Length: 76
> Content-Type: application/x-www-form-urlencoded
>
* upload completely sent off: 76 out of 76 bytes
< HTTP/1.1 200 OK
< content-length: 23
< content-type: application/json; charset=UTF-8
< date: Mon, 20 May 2019 20:49:02 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 1943
<
* Connection #0 to host 169.63.251.68 left intact
{"predictions": [1, 1], "batchId": "XXXXXXXXXXXXXXXXX"}
```

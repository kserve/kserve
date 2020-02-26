# Predict on a InferenceService using a prebuilt image

## Setup

1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#deploy-kfserving).
2. Your cluster's Istio Ingress gateway must be network accessible.

## Create the InferenceService

Apply the CRD

```
kubectl apply -f custom.yaml
```

Expected Output

```
inferenceservice.serving.kubeflow.org/custom-prebuilt-image
```

## Run a prediction

This example uses the [codait/max-object-detector](https://github.com/IBM/MAX-Object-Detector) image. Since its REST interface is different than the [Tensorflow V1 HTTP API](https://www.tensorflow.org/tfx/serving/api_rest#predict_api) that KFServing expects we will need to bypass the inferenceservice and send our request directly to the predictor. The Max Object Detector api server expects a POST request to the `/model/predict` endpoint that includes an `image` multipart/form-data and an optional `threshold` query string.

```
MODEL_NAME=custom-prebuilt-image
CLUSTER_IP=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
SERVICE_HOSTNAME=$(kubectl get route ${MODEL_NAME}-predictor-default -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -F "image=@dog-human.jpg" http://${CLUSTER_IP}/model/predict -H "Host: ${SERVICE_HOSTNAME}"
```

Expected output

```
*   Trying 169.47.250.204...
* TCP_NODELAY set
* Connected to 169.47.250.204 (169.47.250.204) port 80 (#0)
> POST /model/predict HTTP/1.1
> Host: custom-prebuilt-image-predictor-default.default.example.com
> User-Agent: curl/7.64.1
> Accept: */*
> Content-Length: 125759
> Content-Type: multipart/form-data; boundary=------------------------cfebb5b7485962f9
> Expect: 100-continue
>
< HTTP/1.1 100 Continue
* We are completely uploaded and fine
< HTTP/1.1 200 OK
< content-length: 377
< content-type: application/json
< date: Wed, 26 Feb 2020 18:13:40 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 5312
<
{"status": "ok", "predictions": [{"label_id": "1", "label": "person", "probability": 0.944034993648529, "detection_box": [0.1242099404335022, 0.12507186830043793, 0.8423267006874084, 0.5974075794219971]}, {"label_id": "18", "label": "dog", "probability": 0.8645511865615845, "detection_box": [0.10447660088539124, 0.1779915690422058, 0.8422801494598389, 0.7320017218589783]}]}
* Connection #0 to host 169.47.250.204 left intact
* Closing connection 0
```

If the image you are using follows the Tensorflow V1 HTTP API you do not need to bypass the inferenceservice. You can do this by replacing the above command where we set SERVICE_HOSTNAME with one that uses the inferenceservice.

```
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}' | cut -d "/" -f 3)
```

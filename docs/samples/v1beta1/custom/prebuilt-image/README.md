# Predict on a InferenceService using a prebuilt image

## Setup

1. Your ~/.kube/config should point to a cluster with [KServe installed](https://github.com/kserve/kserve/#installation).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).

## Create the InferenceService

Apply the CRD

```
kubectl apply -f custom.yaml
```

Expected Output

```
inferenceservice.serving.kserve.io/custom-prebuilt-image
```

## Run a prediction
The first step is to [determine the ingress IP and ports](https://kserve.github.io/website/get_started/first_isvc/#3-determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

This example uses the [codait/max-object-detector](https://github.com/IBM/MAX-Object-Detector) image. The Max Object Detector api server expects a POST request to the `/model/predict` endpoint that includes an `image` multipart/form-data and an optional `threshold` query string.

```
MODEL_NAME=custom-prebuilt-image
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -F "image=@dog-human.jpg" http://${INGRESS_HOST}:${INGRESS_PORT}/model/predict -H "Host: ${SERVICE_HOSTNAME}"
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

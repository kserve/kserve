
# Predict on a InferenceService using Tensorflow
## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#deploy-kfserving).
2. Your cluster's Istio Ingress gateway must be network accessible.


## Create the InferenceService
Apply the CRD
```
kubectl apply -f tensorflow.yaml 
```

Expected Output
```
$ inferenceservice.serving.kubeflow.org/flowers-sample configured
```

## Run a prediction

```
MODEL_NAME=flowers-sample
INPUT_PATH=@./input.json
CLUSTER_IP=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" http://$CLUSTER_IP/v1/models/$MODEL_NAME:predict -d $INPUT_PATH
```
Expected Output
```
*   Trying 34.83.190.188...
* TCP_NODELAY set
* Connected to 34.83.190.188 (34.83.190.188) port 80 (#0)
> POST /v1/models/flowers-sample:predict HTTP/1.1
> Host: flowers-sample.default.svc.cluster.local
> User-Agent: curl/7.60.0
> Accept: */*
> Content-Length: 16201
> Content-Type: application/x-www-form-urlencoded
> Expect: 100-continue
> 
< HTTP/1.1 100 Continue
* We are completely uploaded and fine
< HTTP/1.1 200 OK
< content-length: 204
< content-type: application/json
< date: Fri, 10 May 2019 23:22:04 GMT
< server: envoy
< x-envoy-upstream-service-time: 19162
< 
{
    "predictions": [
        {
            "scores": [0.999115, 9.20988e-05, 0.000136786, 0.000337257, 0.000300533, 1.84814e-05],
            "prediction": 0,
            "key": "   1"
        }
    ]
* Connection #0 to host 34.83.190.188 left intact
}%
```

If you stop making requests to the application, you should eventually see that your application scales itself back down to zero. Watch the pod until you see that it is `Terminating`. This should take approximately 90 seconds.

```
kubectl get pods --watch
```

Note: To exit the watch, use `ctrl + c`.

## Canary Rollout

To test a canary rollout, you can use the tensorflow-canary.yaml 

Apply the CRD
```
kubectl apply -f tensorflow-canary.yaml 
```

To verify if your traffic split percenage is applied correctly, you can use the following command:

```
kubectl get inferenceservices
NAME             READY     URL                                  DEFAULT TRAFFIC   CANARY TRAFFIC   AGE
flowers-sample   True      http://flowers-sample.default.example.com   90                10               48s
```

If you are using the [Knative CLI (kn)](https://github.com/knative/client), run the following command

```
kn route list

NAME                               URL                                                           AGE     CONDITIONS   TRAFFIC
flowers-sample-predictor-canary    http://flowers-sample-predictor-canary.default.example.com    2d23h   3 OK / 3     100% -> flowers-sample-predictor-canary-mswmr
flowers-sample-predictor-default   http://flowers-sample-predictor-default.default.example.com   5d20h   3 OK / 3     100% -> flowers-sample-predictor-default-x7zcl

```

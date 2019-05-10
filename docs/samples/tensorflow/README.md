
# Predict on a KFService using Tensorflow
## Setup
1. Your ~/.kube/config should point to a cluster with KFServing installed.
2. Your cluster's Istio Ingressgateway must be network accessible.

## Create the KFService
Apply the CRD
```
kubectl apply -f tensorflow.yaml 
```

Expected Output
```
$ kfservice.serving.kubeflow.org/flowers-sample configured
```

## Run a prediction

```
MODEL_NAME=flowers-sample
INPUT_PATH=@./input.json
CLUSTER_IP=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

curl -v -H "Host: flowers-sample.default.svc.cluster.local" http://$CLUSTER_IP/v1/models/$MODEL_NAME:predict -d $INPUT_PATH
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
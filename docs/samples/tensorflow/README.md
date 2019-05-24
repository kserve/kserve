
# Predict on a KFService using Tensorflow
## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#deploy-kfserving).
2. Your cluster's Istio Ingress gateway must be network accessible.
3. Your cluster's Istio Egresss gateway must [allow Google Cloud Storage](https://knative.dev/docs/serving/outbound-network-access/)

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
## Canary Rollout

To run a canary rollout, you can use the tensorflow-canary.yaml 

Apply the CRD
```
kubectl apply -f tensorflow-canary.yaml 
```


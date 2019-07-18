
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
SERVICE_HOSTNAME=$(kubectl get kfservice ${MODEL_NAME} -o jsonpath='{.status.url}' |sed -e 's/^http:\/\///g' -e 's/^https:\/\///g')

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
kubectl get kfservices
NAME             URL                                  DEFAULT TRAFFIC   CANARY TRAFFIC   AGE
flowers-sample   http://flowers-sample.default.example.com   90                10               48s
```

If you are using the [Knative CLI (knctl)](#knative-cli), run the following command

```
knctl route show --route flowers-sample 
Route 'flowers-sample'

Name             flowers-sample  
Domain           flowers-sample.default.example.com  
Internal Domain  flowers-sample.default.svc.cluster.local  
Age              1m  

Targets

Percent  Revision                      Service  Domain  
90%      flowers-sample-default-4s74r  -        flowers-sample.default.example.com  
10%      flowers-sample-canary-bjdkm   -        flowers-sample.default.example.com  

Conditions

Type                Status  Age  Reason  Message  
AllTrafficAssigned  True    46s  -       -  
IngressReady        True    45s  -       -  
Ready               True    45s  -       -  

Succeeded
```


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
flowers-sample   flowers-sample.default.example.com   90                10               48s
```

If you are using the [Knative CLI (knctl)](#knative-cli), run the following command

```
knctl revision list 
Revisions

Service                 Name                          Tags  Annotations                                                 Conditions  Age  Traffic  
flowers-sample-canary   flowers-sample-canary-6kpt6   -     autoscaling.knative.dev/class: kpa.autoscaling.knative.dev  4 OK / 5    40m  10% -> flowers-sample.default.example.com  
                                                            autoscaling.knative.dev/target: "1"                                            
flowers-sample-default  flowers-sample-default-l9c24  -     autoscaling.knative.dev/class: kpa.autoscaling.knative.dev  4 OK / 5    40m  90% -> flowers-sample.default.example.com  
                                                            autoscaling.knative.dev/target: "1"  
```

### Knative CLI:

[Knative CLI (`knctl`)](https://github.com/cppforlife/knctl) provides simple set of commands to interact with a [Knative installation](https://github.com/knative/docs). You can grab pre-built binaries from the [Releases page](https://github.com/cppforlife/knctl/releases). Once downloaded, you can run the following commands to get it working.

```
# compare checksum output to what's included in the release notes
$ shasum -a 265 ~/Downloads/knctl-*

# move binary to your system’s /usr/local/bin -- might require root password
$ mv ~/Downloads/knctl-* /usr/local/bin/knctl

# make the newly copied file executable -- might require root password
$ chmod +x /usr/local/bin/knctl
```
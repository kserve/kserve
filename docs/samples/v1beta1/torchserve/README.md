# Predict on a InferenceService using Torchserve

## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).

## Create PV and PVC
This document uses amazonEBS PV 

### Create PV

Edit volume id in pv.yaml file

```
kubectl apply -f pv.yaml
```

Expected Output

```
persistentvolume/model-pv-volume created
```

### Create PVC

```
kubectl apply -f pvc.yaml
```

Expected Output

```
persistentvolumeclaim/model-pv-claim created
```

### Create PV Pod

```
kubectl apply -f pvpod.yaml
```

Expected Output

```
pod/model-store-pod created
```

### Copy mar file and config properties to storage

Download Marfile

```
wget https://torchserve.pytorch.org/mar_files/mnist.mar
```

Download input image

```
wget https://raw.githubusercontent.com/pytorch/serve/master/examples/image_classifier/mnist/test_data/0.png
```

Copy Marfile

```
kubectl exec --tty pod/model-store-pod -- mkdir /pv/model-store/
kubectl cp mnist.mar model-store-pod:/pv/model-store/mnist.mar
```

Copy config.properties

```
kubectl exec --tty pod/model-store-pod -- mkdir /pv/config/
kubectl cp config.properties model-store-pod:/pv/config/config.properties
```

### Delete pv pod

Since amazon EBS provide only ReadWriteOnce mode

```
kubectl delete pod model-store-pod -n kfserving-test
```

## Create the InferenceService

Apply the CRD

```
kubectl apply -f torchserve.yaml
```

Expected Output

```
$ inferenceservice.serving.kubeflow.org/torchserve created
```

## Run a prediction
The first step is to [determine the ingress IP and ports](../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`


```
MODEL_NAME=torchserve
SERVICE_HOSTNAME=$(kubectl get route torchserve-predictor-default -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/predictions/mnist -T 1.png
```

Expected Output

```
*   Trying 52.89.19.61...
* Connected to a881f5a8c676a41edbccdb0a394a80d6-2069247558.us-west-2.elb.amazonaws.com (52.89.19.61) port 80 (#0)
> PUT /predictions/mnist HTTP/1.1
> Host: torchserve-predictor-default.kfserving-test.example.com
> User-Agent: curl/7.47.0
> Accept: */*
> Content-Length: 167
> Expect: 100-continue
> 
< HTTP/1.1 100 Continue
* We are completely uploaded and fine
< HTTP/1.1 200 OK
< cache-control: no-cache; no-store, must-revalidate, private
< content-length: 1
< date: Tue, 27 Oct 2020 08:26:19 GMT
< expires: Thu, 01 Jan 1970 00:00:00 UTC
< pragma: no-cache
< x-request-id: b10cfc9f-cd0f-4cda-9c6c-194c2cdaa517
< x-envoy-upstream-service-time: 6
< server: istio-envoy
< 
* Connection #0 to host a881f5a8c676a41edbccdb0a394a80d6-2069247558.us-west-2.elb.amazonaws.com left intact
1
```

## For Autoscaling 
Configurations for autoscaling pods [Auto scaling](docs/autoscaling.md)

## Canary Rollout
Configurations for canary [Canary Deployment](docs/canary.md)

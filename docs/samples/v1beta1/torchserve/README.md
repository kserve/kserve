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

### Create properties.json file
This file has model-name, version, model-file name, serialized-file name, extra-files, handlers, workers etc. of the models.

```
[
  {
    "model-name": "mnist",
    "version": "1.0",
    "model-file": "",
    "serialized-file": "mnist_cnn.pt",
    "extra-files": "",
    "handler": "mnist_handler.py",
    "min-workers" : 1,
    "max-workers": 3,
    "batch-size": 1,
    "max-batch-delay": 100,
    "response-timeout": 120,
    "requirements": ""
  },
  {
    "model-name": "densenet_161",
    "version": "1.0",
    "model-file": "",
    "serialized-file": "densenet161-8d451a50.pth",
    "extra-files": "index_to_name.json",
    "handler": "image_classifier",
    "min-workers" : 1,
    "max-workers": 3,
    "batch-size": 1,
    "max-batch-delay": 100,
    "response-timeout": 120,
    "requirements": ""
  }
]
```

### Copy Files:

Copy all the model and dependent files to the PV in the structure given below.
An empty config folder, a model-store folder containing model name as folder name. Within that model folder, the files required to build the marfile.

```
├── config
├── model-store
│   ├── densenet_161
│   │   ├── densenet161-8d451a50.pth
│   │   ├── index_to_name.json
│   │   └── model.py
│   ├── mnist
│   │   ├── mnist_cnn.pt
│   │   ├── mnist_handler.py
│   │   └── mnist.py
│   └── properties.json

```

#### Create folders in PV

```
kubectl exec -it model-store-pod -c model-store -n kfserving-test -- mkdir /pv/model-store/

kubectl exec -it model-store-pod -c model-store -n kfserving-test -- mkdir /pv/config/
```

### Copy model files

```
kubectl cp model-store model-store-pod:/pv/ -c model-store -n kfserving-test

```

### Delete pv pod

Since amazon EBS provide only ReadWriteOnce mode

```
kubectl delete pod model-store-pod -n kfserving-test
```

### Apply model-archiver

```
kubectl apply -f model-archiver.yaml -n kfserving-test
```

Verify mar files and config.properties

```
kubectl exec -it margen-pod -n kfserving-test -- ls -lR /home/model-server/model-store
kubectl exec -it margen-pod -n kfserving-test -- cat /home/model-server/config/config.properties
```

### Delete model archiver

```
kubectl delete -f model-archiver.yaml -n kfserving-test
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

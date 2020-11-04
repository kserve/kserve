# Predict on a InferenceService using Torchserve

## Setup

1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).

## Create PV and PVC

This document uses amazonEBS PV

### Create PV

Edit volume id in pv.yaml file

```bash
kubectl apply -f pv.yaml
```

Expected Output

```bash
persistentvolume/model-pv-volume created
```

### Create PVC

```bash
kubectl apply -f pvc.yaml
```

Expected Output

```bash
persistentvolumeclaim/model-pv-claim created
```

### Create PV Pod

```bash
kubectl apply -f pvpod.yaml
```

Expected Output

```bash
pod/model-store-pod created
```

### Create properties.json file

This file has model-name, version, model-file name, serialized-file name, extra-files, handlers, workers etc. of the models.

```json
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

### Copy Files

Copy all the model and dependent files to the PV in the structure given below.
An empty config folder, a model-store folder containing model name as folder name. Within that model folder, the files required to build the marfile.

```bash
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

```bash
kubectl exec -it model-store-pod -c model-store -n kfserving-test -- mkdir /pv/model-store/

kubectl exec -it model-store-pod -c model-store -n kfserving-test -- mkdir /pv/config/
```

### Copy model files

```bash
kubectl cp model-store model-store-pod:/pv/ -c model-store -n kfserving-test
```

### Delete pv pod

Since amazon EBS provide only ReadWriteOnce mode

```bash
kubectl delete pod model-store-pod -n kfserving-test
```

### Apply model-archiver

```bash
kubectl apply -f model-archiver.yaml -n kfserving-test
```

Verify mar files and config.properties

```bash
kubectl exec -it margen-pod -n kfserving-test -- ls -lR /home/model-server/model-store
kubectl exec -it margen-pod -n kfserving-test -- cat /home/model-server/config/config.properties
```

### Delete model archiver

```bash
kubectl delete -f model-archiver.yaml -n kfserving-test
```

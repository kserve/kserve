# Generate model archiver files for torchserve

## Setup

1. Your ~/.kube/config should point to a cluster with [KServe installed](https://github.com/kserve/kserve#installation).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).

## 1. Create PV and PVC

Create a Persistent volume and volume claim. This document uses amazonEBS PV. For AWS EFS storage you can refer to [AWS EFS storage](https://github.com/pytorch/serve/blob/master/kubernetes/EKS/README.md#setup-persistentvolume-backed-by-efs)

### 1.1 Create PV

Edit volume id in pv.yaml file

```bash
kubectl apply -f pv.yaml
```

Expected Output

```bash
persistentvolume/model-pv-volume created
```

### 1.2 Create PVC

```bash
kubectl apply -f pvc.yaml
```

Expected Output

```bash
persistentvolumeclaim/model-pv-claim created
```

## 2 Create model store files layout and copy to PV

We create a pod with the PV attached to copy the model files and config.properties for generating model archive file.

### 2.1 Create pod for copying model store files to PV

```bash
kubectl apply -f pvpod.yaml
```

Expected Output

```bash
pod/model-store-pod created
```

### 2.2 Create model store file layout on PV

#### 2.2.1 Create properties.json file

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

#### 2.2.2 Copy model and its dependent Files

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

#### 2.2.3 Create folders for model-store and config in PV

```bash
kubectl exec -it model-store-pod -c model-store -n kfserving-test -- mkdir /pv/model-store/

kubectl exec -it model-store-pod -c model-store -n kfserving-test -- mkdir /pv/config/
```

### 2.3 Copy model files and config.properties to the PV

```bash
kubectl cp model-store/* model-store-pod:/pv/model-store/ -c model-store -n kfserving-test
kubectl cp config.properties model-store-pod:/pv/config/ -c model-store -n kfserving-test
```

### 2.4 Delete pv pod

Since amazon EBS provide only ReadWriteOnce mode, we have to unbind the PV for use of model archiver.

```bash
kubectl delete pod model-store-pod -n kfserving-test
```

## 3 Generate model archive file and server configuration file

### 3.1 Create model archive pod and run model archive file generation script

```bash
kubectl apply -f model-archiver.yaml -n kfserving-test
```

### 3.2 Check the output and delete model archive pod

Verify mar files and config.properties

```bash
kubectl exec -it margen-pod -n kfserving-test -- ls -lR /home/model-server/model-store
kubectl exec -it margen-pod -n kfserving-test -- cat /home/model-server/config/config.properties
```

### 3.3 Delete model archiver

```bash
kubectl delete -f model-archiver.yaml -n kfserving-test
```

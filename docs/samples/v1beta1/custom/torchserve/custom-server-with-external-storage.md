# Torchserve with external storage

For running torchserve with external storage, the model archive files and config.properties should be copied to the storage.

The storage mount to ```/mnt/models``` directory.

## Folder structure

```json
├──config.properties
├── model-store
│   ├── mnist.mar
|   ├── densenet161.mar
```

The entrypoint should be modified, to start torchserve with config.properties in ```/mnt/models``` path.

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

Generate marfile from [here](https://github.com/pytorch/serve/tree/master/examples/image_classifier/mnist)

### Copy mar file and config properties to storage

Copy Marfile

```bash
kubectl exec --tty pod/model-store-pod -- mkdir /pv/model-store/
kubectl cp mnist.mar model-store-pod:/pv/model-store/mnist.mar
```

Copy config.properties

```bash
kubectl exec --tty pod/model-store-pod -- mkdir /pv/config/
kubectl cp config.properties model-store-pod:/pv/config/config.properties
```

### Delete pv pod

Since amazon EBS provide only ReadWriteOnce mode

## Create the InferenceService

In the `torchserve-custom-pv.yaml` file edit the container image with your Docker image and add your pv storage.

Apply the CRD

```bash
kubectl apply -f torchserve-custom-pv.yaml
```

Expected Output

```bash
$inferenceservice.serving.kubeflow.org/torchserve-custom-pv created
```

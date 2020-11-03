# Torchserve with external storage

For running torchserve with external storage, the model archive files and config.properties should be copied to the storage.

The storage mount to ```/mnt/models``` directory.

## Folder structure

```
├──config.properties
├── model-store
│   ├── mnist.mar
|   ├── densenet161.mar
```

The entrypoint should be modified, to start torchserve with config.properties in ```/mnt/models``` path.

## Create the InferenceService

In the `torchserve-custom-pv.yaml` file edit the container image with your Docker image and add your pv storage.

Apply the CRD

```
kubectl apply -f torchserve-custom.yaml
```

Expected Output

```
$ inferenceservice.serving.kubeflow.org/torchserve-custom created
```

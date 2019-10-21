
# Create a InferenceService for on-prem cluster

The guide shows how to train model and create InferenceService for the trained model for on-prem cluster.

## Prerequisites
Refer to the [document](https://kubernetes.io/docs/concepts/storage/persistent-volumes/) to create Persistent Volume (PV) and Persistent Volume Claim (PVC), the PVC will be used to store model.

## Training model

Following the mnist example [guide](https://github.com/kubeflow/examples/tree/master/mnist#local-storage) to train mnist model and store to PVC. In the example, the relative path of model will be `./export/` on the PVC.

## Create the InferenceService

Update the ${PVC_NAME} to the created PVC name in the `mnis-pvc.yaml` and apply:
```bash
kubectl apply -f mnist-pvc.yaml
```

Expected Output
```
$ inferenceservice.serving.kubeflow.org/mnist-sample configured
```

## Check the InferenceService

```bash
$ kubectl get inferenceservice
NAME           URL                                                               READY     DEFAULT TRAFFIC   CANARY TRAFFIC   AGE
mnist-sample   http://mnist-sample.kubeflow.example.com/v1/models/mnist-sample   True      100                                1m
```

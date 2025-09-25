# CatBoost InferenceService

This example shows how to deploy a CatBoost model using KServe.

## Prerequisites

1. Your cluster has KServe installed
2. Your model is saved in CatBoost format (.cbm or .bin)

## Create the InferenceService

Apply the CRD:

```bash
kubectl apply -f catboost.yaml
```

Expected Output:

```
inferenceservice.serving.kserve.io/catboost-iris created
```

## Check InferenceService status

```bash
kubectl get inferenceservices catboost-iris
```

## Run a prediction

The first step is to [determine the ingress IP and ports](../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`.

```bash
MODEL_NAME=catboost-iris
INPUT_PATH=@./iris-input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice catboost-iris -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict -d $INPUT_PATH
```

Expected Output:

```json
{
  "predictions": [
    [0.9, 0.05, 0.05]
  ]
}
```

## Delete the InferenceService

```bash
kubectl delete -f catboost.yaml
```
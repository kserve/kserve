# Bias detection on a InferenceService using aif360

## Create the InferenceService

Apply the CRD

```
kubectl apply -f bias.yaml
```

Expected Output

```
$ inferenceservice.serving.kubeflow.org/german-credit created
```

### Deploy the message dumper

Apply the message-dumper CRD which will collect the logs that are created when running predictions on the inferenceservice.

```
kubectl apply -f message-dumper.yaml
```

Expected Output

```
service.serving.knative.dev/message-dumper created
```

### Run a prediction

The first step is to [determine the ingress IP and ports](../../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```
MODEL_NAME=german-credit
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}' | cut -d "/" -f 3)
python simulate_predicts.py http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict ${SERVICE_HOSTNAME}
```

### Run a bias detection

```
python query_bias.py http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:explain ${SERVICE_HOSTNAME}
```

### Expected output

```
bash-3.2$ python3 query_bias.py http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:explain ${SERVICE_HOSTNAME}
Collecting logs...
Sending bias query...
TIME TAKEN:  0.21054291725158691
<Response [200]>
{'predictions': [1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 2.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 2.0, 1.0, 2.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 2.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 2.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 2.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 2.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 2.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 2.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0], 'metrics': {'base_rate': 0.9497206703910615, 'consistency': [0.9653631284916201], 'disparate_impact': 0.64, 'num_instances': 179.0, 'num_negatives': 9.0, 'num_positives': 170.0, 'statistical_parity_difference': -0.36}}
```

# Dataset

The dataset used in this example is the German Credit dataset maintained by the [UC Irvine Machine Learning Repository](https://archive.ics.uci.edu/ml/index.php).

# Using AIX to get explanations for MNIST classifications

This is an example of how to explain model outputs using [AIX360](https://github.com/Trusted-AI/AIX360) on KFServing. We will be using mnist, a dataset for handwritten digits, for this model and explain how the model decides the predicted results.

To deploy the inferenceservice with v1beta1 API

`kubectl apply -f aix-explainer.yaml`

Then find the url.

`kubectl get inferenceservice`

```
NAME         URL                                               READY   DEFAULT TRAFFIC   CANARY TRAFFIC   AGE
aixserver   http://aixserver.somecluster/v1/models/aixserver   True    100                                40m
```

## Prediction
The first step is to [determine the ingress IP and ports](../../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```
MODEL_NAME=aix-explainer
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}' | cut -d "/" -f 3)
python query_explain.py http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:explain ${SERVICE_HOSTNAME}
```

To try a different MNIST example add an integer to the end of the query between 0-10,000. The integer chosen will be the index of the image to be chosen in the MNIST dataset.

```
python query_explain.py http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:explain ${SERVICE_HOSTNAME} 100
```

## Stopping the Inference Service

`kubectl delete -f aix-explainer.yaml`

## Troubleshooting

`<504> Gateway Timeout <504>` - the explainer is probably taking to long and not sending a response back quickly enough. Either there aren't enough resources allocated or the number of samples the explainer is allowed to take needs to be reduced. To fix this go to aix-explainer.yaml and increase resources. Or to lower the number of allowed samples go to aix-explainer.yaml and add a flag to `explainer: command:` '--num_samples' (the default number of samples is 1000)

If you see `Configuration "aixserver-explainer-default" does not have any ready Revision` the container may have taken too long to download. If you run `kubectl get revision` and see your revision is stuck in `ContainerCreating` try deleting the inferenceservice and redeploying.

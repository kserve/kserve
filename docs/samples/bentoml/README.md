# Predict on an InferenceService using BentoML

This guide demonstrates how to serve a scikit-learn based iris classifier model with BentoML
and deploying the BentoML model server with KFServing. The same deployment
steps are also applicable for models trained with other machine learning frameworks, see
more BentoML examples [here](https://github.com/bentoml/BentoML/tree/main/examples).

[BentoML](https://bentoml.org) is an open-source platform for high-performance ML model
serving. It makes building production API endpoint for your ML model easy and supports all
major machine learning training frameworks, including Tensorflow, Keras, PyTorch, XGBoost,
scikit-learn and etc.

BentoML comes with a high-performance API model server with adaptive micro-batching support,
which achieves the advantage of batch processing in online serving. It also provides model
management and model deployment functionality, giving ML teams an end-to-end model serving
workflow, with DevOps best practices baked in.

## Deploy a custom InferenceService

### Setup

Before starting this guide, make sure you have the following:

* Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/#install-kfserving).
* Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).
* Docker and Docker hub must be properly configured on your local system
* Python 3.6 or above
  * Install required packages `bentoml` and `scikit-learn` on your local system:

    ```shell
    pip install "bentoml==0.13.1" "scikit-learn==1.0.2"
    ```

### Build API model server using BentoML

The following code defines a BentoML prediction service that requires a `scikit-learn` model, and
asks BentoML to figure out the required PyPI pip packages automatically. It also defines
an API, which is the entry point for accessing this prediction service. And the API is
expecting a `pandas.DataFrame` object as its input data.

```python
# iris_classifier.py
from bentoml import env, artifacts, api, BentoService
from bentoml.frameworks.sklearn import SklearnModelArtifact
from bentoml.adapters import DataframeInput
import pandas as pd

@env(auto_pip_dependencies=True)
@artifacts([SklearnModelArtifact('model')])
class IrisClassifier(BentoService):

    @api(input=DataframeInput(), batch=True)
    def predict(self, df: pd.DataFrame):
        return self.artifacts.model.predict(df)
```

The following code trains a classifier model and serve it with the IrisClassifier defined above:

```python
# main.py
from sklearn import svm
from sklearn import datasets

from iris_classifier import IrisClassifier
import pandas as pd

    # Load training data
iris = datasets.load_iris()
X, y = iris.data, iris.target

    # Model Training
clf = svm.SVC(gamma='scale')
clf.fit(X, y)

svc = IrisClassifier()
svc.pack("model", clf)

save_path = svc.save()
print("Saved to:", saved_path)

```

The sample code above can be found in the BentoML repository, run them directly with the
following command:

```bash
git clone git@github.com:bentoml/BentoML.git
python ./bentoml/guides/quick-start/main.py
```

After saving the BentoService instance, you can now start a REST API server with the
model trained and test the API server locally:

```bash
# Start BentoML API server:
bentoml serve IrisClassifier:latest
```

```bash
# Send test request:
curl -i \
  --header "Content-Type: application/json" \
  --request POST \
  --data '[[5.1, 3.5, 1.4, 0.2]]' \
  http://localhost:5000/predict
```

### Deploy InferenceService

BentoML provides a convenient way of containerizing the model API server with Docker. To
create a docker container image for the sample model above:

1. Find the file directory of the SavedBundle with `bentoml get` command, which is
directory structured as a docker build context.
2. Running docker build with this directory produces a docker image containing the API
model server.

```shell
model_path=$(bentoml get IrisClassifier:latest -q | jq -r ".uri.uri")

# Replace {docker_username} with your Docker Hub username
docker build -t {docker_username}/iris-classifier $model_path
docker push {docker_username}/iris-classifier
```

*Note: BentoML's REST interface is different than the Tensorflow V1 HTTP API that
KFServing expects. Requests will send directly to the prediction service and bypass the
top-level InferenceService.*

*Support for KFServing V2 prediction protocol with BentoML is coming soon.*

The following is an example YAML file for specifying the resources required to run an
InferenceService in KFServing. Replace `{docker_username}` with your Docker Hub username
and save it to `bentoml.yaml` file:

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  labels:
    controller-tools.k8s.io: "1.0"
  name: iris-classifier
spec:
  predictor:
    container:
      image: {docker_username}/iris-classifier
      ports:
        - containerPort: 5000
```

Use `kubectl apply` command to deploy the InferenceService:

```shell
kubectl apply -f bentoml.yaml
```

### Run prediction
The first step is to [determine the ingress IP and ports](../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```shell
MODEL_NAME=iris-classifier
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" \
  --header "Content-Type: application/json" \
  --request POST \
  --data '[[5.1, 3.5, 1.4, 0.2]]' \
  http://${INGRESS_HOST}:${INGRESS_PORT}/predict
```

### Delete deployment

```shell
kubectl delete -f bentoml.yaml
```

## Additional Resources

* [GitHub repository](https://github.com/bentoml/BentoML)
* [BentoML documentation](https://docs.bentoml.org)
* [Tutorial](https://docs.bentoml.org/en/latest/tutorial.html)
* [Community](https://join.slack.com/t/bentoml/shared_invite/enQtNjcyMTY3MjE4NTgzLTU3ZDc1MWM5MzQxMWQxMzJiNTc1MTJmMzYzMTYwMjQ0OGEwNDFmZDkzYWQxNzgxYWNhNjAxZjk4MzI4OGY1Yjg)

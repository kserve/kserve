# Predict on a Spark MLlib model PMML InferenceService
spark.mllib supports model export to Predictive Model Markup Language [PMML](https://en.wikipedia.org/wiki/Predictive_Model_Markup_Language).

## Setup
1. Your ~/.kube/config should point to a cluster with [KServe installed](https://github.com/kserve/kserve#installation).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).
3. Install `pyspark` 3.0.x and `pyspark2pmml`
```bash
pip install pyspark~=3.0.0
pip install pyspark2pmml
```
4. Get [JPMML-SparkML jar](https://github.com/jpmml/jpmml-sparkml/releases/download/1.6.3/jpmml-sparkml-executable-1.6.3.jar) 

## Train a Spark MLlib model and export to PMML file

Launch pyspark with `--jars` to specify the location of the `JPMML-SparkML` uber-JAR
```bash
pyspark --jars ./jpmml-sparkml-executable-1.6.3.jar
```

Fitting a Spark ML pipeline:
```python
from pyspark.ml import Pipeline
from pyspark.ml.classification import DecisionTreeClassifier
from pyspark.ml.feature import RFormula

df = spark.read.csv("Iris.csv", header = True, inferSchema = True)

formula = RFormula(formula = "Species ~ .")
classifier = DecisionTreeClassifier()
pipeline = Pipeline(stages = [formula, classifier])
pipelineModel = pipeline.fit(df)

from pyspark2pmml import PMMLBuilder

pmmlBuilder = PMMLBuilder(sc, df, pipelineModel)

pmmlBuilder.buildFile("DecisionTreeIris.pmml")
```

Upload the `DecisionTreeIris.pmml` to a GCS bucket, note that the `PMMLServer` expect model file name to be `model.pmml`
```bash
gsutil cp ./DecisionTreeIris.pmml gs://$BUCKET_NAME/sparkpmml/model.pmml
```
 
## Create the InferenceService with PMMLServer
Create the `InferenceService` with `pmml` predictor and specify the `storageUri` with bucket location you uploaded to
```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "spark-pmml"
spec:
  predictor:
    pmml:
      storageUri: gs://kfserving-examples/models/sparkpmml
```

Apply the `InferenceService` custom resource
```
kubectl apply -f spark_pmml.yaml
```

Expected Output
```
$ inferenceservice.serving.kserve.io/spark-pmml created
```

Wait the `InferenceService` to be ready
```bash
kubectl wait --for=condition=Ready inferenceservice spark-pmml
inferenceservice.serving.kserve.io/spark-pmml condition met
```

### Run a prediction
The first step is to [determine the ingress IP and ports](../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```
MODEL_NAME=spark-pmml
INPUT_PATH=@./pmml-input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice spark-pmml -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict -d $INPUT_PATH
```

Expected Output

```
* Connected to spark-pmml.default.35.237.217.209.xip.io (35.237.217.209) port 80 (#0)
> POST /v1/models/spark-pmml:predict HTTP/1.1
> Host: spark-pmml.default.35.237.217.209.xip.io
> User-Agent: curl/7.73.0
> Accept: */*
> Content-Length: 45
> Content-Type: application/x-www-form-urlencoded
>
* upload completely sent off: 45 out of 45 bytes
* Mark bundle as not supporting multiuse
< HTTP/1.1 200 OK
< content-length: 39
< content-type: application/json; charset=UTF-8
< date: Sun, 07 Mar 2021 19:32:50 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 14
<
* Connection #0 to host spark-pmml.default.35.237.217.209.xip.io left intact
{"predictions": [[1.0, 0.0, 1.0, 0.0]]}
```

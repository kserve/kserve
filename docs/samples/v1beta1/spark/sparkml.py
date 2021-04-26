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


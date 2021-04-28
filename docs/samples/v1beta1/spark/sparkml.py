from pyspark.sql import SparkSession
from pyspark.ml import Pipeline
from pyspark.ml.classification import DecisionTreeClassifier
from pyspark.ml.feature import RFormula
from pyspark2pmml import PMMLBuilder

spark = SparkSession.builder.appName('SparkByExamples.com').getOrCreate()
df = spark.read.csv("Iris.csv", header=True, inferSchema=True)

formula = RFormula(formula="Species ~ .")
classifier = DecisionTreeClassifier()
pipeline = Pipeline(stages=[formula, classifier])
pipelineModel = pipeline.fit(df)

pmmlBuilder = PMMLBuilder(spark, df, pipelineModel)

pmmlBuilder.buildFile("DecisionTreeIris.pmml")

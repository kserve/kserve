# Example Anchors Tabular Explaination for Income Prediction

Train a model for income prediction.

```
python train.py 
```

This will create the following files:

  * `model.joblib` : pickle of model.
  * `explainer.dill` : pickle of trained back box explainer for this model.

Now, run a KFServing sklearn server with this model:

```
python -m sklearnserver --model_dir .  --model_name income --protocol tensorflow.http
```

In a different terminal start the Alibi Explainer using the local saved explainer:

```
python -m alibiexplainer --explainer_name income --predict_url http://localhost:8080/models/income:predict --protocol tensorflow.http --http_port 8081 --type anchor_tabular --storageUri ${PWD}
```

You can now get an explaination for some particular features:

```
curl -H "Content-Type: application/json" -d '{"instances":[[39, 7, 1, 1, 1, 1, 4, 1, 2174, 0, 40, 9]]}' http://localhost:8081/models/income:explain
```

You should receive an output like the following which contains the core explanation and various examples and counter examples:

```JSON
{"names": ["Marital Status = Never-Married", "Occupation = Admin"], "precision": 0.9558232931726908, "coverage": 0.0492, "raw": {"feature": [3, 4], "mean": [0.9195402298850575, 0.9558232931726908], "precision": [0.9195402298850575, 0.9558232931726908], "coverage": [0.3296, 0.0492], "examples": [{"covered": [[39, "Private", "High School grad", "Never-Married", "Blue-Collar", "Own-child", "White", "Male", 0, 0, 40, "United-States"], [43, "Private", "High School grad", "Never-Married", "Blue-Collar", "Husband", "White", "Male", 0, 0, 44, "Latin-America"], [34, "Private", "Associates", "Never-Married", "Admin", "Not-in-family", "Asian-Pac-Islander", "Female", 8614, 0, 60, "United-States"], [22, "Private", "Dropout", "Never-Married", "Blue-Collar", "Unmarried", "White", "Male", 0, 0, 30, "United-States"], [30, "Private", "Dropout", "Never-Married", "Service", "Wife", "Black", "Female", 0, 0, 30, "United-States"], [31, "Federal-gov", "Associates", "Never-Married", "Professional", "Own-child", "White", "Male", 0, 0, 40, "United-States"], [48, "Private", "Bachelors", "Never-Married", "Professional", "Husband", "White", "Male", 0, 0, 50, "United-States"], [39, "Local-gov", "Dropout", "Never-Married", "Admin", "Unmarried", "White", "Female", 0, 0, 40, "United-States"], [30, "Local-gov", "Masters", "Never-Married", "Professional", "Husband", "White", "Male", 0, 1887, 45, "United-States"], [29, "Private", "High School grad", "Never-Married", "Service", "Not-in-family", "White", "Female", 0, 0, 41, "United-States"]], "covered_true": [[45, "Self-emp-inc", "High School grad", "Never-Married", "White-Collar", "Not-in-family", "White", "Male", 0, 0, 42, "United-States"], [44, "Self-emp-not-inc", "High School grad", "Never-Married", "Sales", "Unmarried", "White", "Male", 0, 0, 60, "United-States"], [44, "Private", "Bachelors", "Never-Married", "Professional", "Wife", "White", "Female", 0, 0, 20, "United-States"], [42, "Private", "High School grad", "Never-Married", "Service", "Wife", "White", "Female", 0, 0, 23, "United-States"], [48, "Private", "High School grad", "Never-Married", "Service", "Husband", "White", "Male", 0, 0, 40, "Latin-America"], [25, "Private", "High School grad", "Never-Married", "Blue-Collar", "Not-in-family", "White", "Male", 0, 0, 40, "United-States"], [39, "Private", "High School grad", "Never-Married", "Service", "Own-child", "White", "Female", 0, 0, 38, "United-States"], [67, "?", "High School grad", "Never-Married", "?", "Husband", "White", "Male", 0, 0, 6, "United-States"], [54, "Self-emp-not-inc", "High School grad", "Never-Married", "Blue-Collar", "Not-in-family", "White", "Male", 0, 0, 40, "?"], [35, "Private", "Dropout", "Never-Married", "Blue-Collar", "Not-in-family", "White", "Male", 0, 0, 50, "United-States"]], "covered_false": [[33, "Private", "Associates", "Never-Married", "White-Collar", "Not-in-family", "White", "Male", 0, 0, 55, "United-States"], [56, "Self-emp-not-inc", "High School grad", "Never-Married", "Blue-Collar", "Not-in-family", "White", "Female", 27828, 0, 20, "United-States"], [39, "Self-emp-not-inc", "Masters", "Never-Married", "Blue-Collar", "Husband", "White", "Male", 0, 0, 55, "United-States"], [57, "Self-emp-not-inc", "Prof-School", "Never-Married", "Professional", "Husband", "White", "Male", 0, 0, 36, "United-States"], [31, "Private", "Bachelors", "Never-Married", "White-Collar", "Not-in-family", "White", "Male", 14084, 0, 50, "United-States"], [29, "Private", "High School grad", "Never-Married", "Service", "Not-in-family", "White", "Female", 0, 0, 41, "United-States"], [44, "Private", "High School grad", "Never-Married", "Blue-Collar", "Not-in-family", "White", "Female", 14344, 0, 40, "United-States"], [53, "Self-emp-not-inc", "Dropout", "Never-Married", "White-Collar", "Husband", "White", "Male", 7688, 0, 67, "Euro_1"], [54, "Self-emp-not-inc", "Prof-School", "Never-Married", "Professional", "Husband", "Asian-Pac-Islander", "Male", 99999, 0, 50, "SE-Asia"], [50, "Local-gov", "Masters", "Never-Married", "Professional", "Wife", "White", "Female", 15024, 0, 40, "United-States"]], "uncovered_true": [], "uncovered_false": []}, {"covered": [[52, "Private", "High School grad", "Never-Married", "Admin", "Husband", "White", "Male", 0, 0, 40, "United-States"], [53, "Private", "Masters", "Never-Married", "Admin", "Husband", "White", "Male", 0, 1902, 50, "United-States"], [60, "Local-gov", "Associates", "Never-Married", "Admin", "Husband", "White", "Male", 0, 0, 60, "United-States"], [54, "Federal-gov", "High School grad", "Never-Married", "Admin", "Unmarried", "White", "Female", 0, 0, 35, "United-States"], [28, "Private", "Dropout", "Never-Married", "Admin", "Unmarried", "White", "Female", 0, 0, 40, "United-States"], [43, "Private", "Bachelors", "Never-Married", "Admin", "Not-in-family", "White", "Male", 0, 0, 40, "United-States"], [28, "Private", "High School grad", "Never-Married", "Admin", "Husband", "White", "Male", 0, 0, 50, "United-States"], [38, "Private", "Dropout", "Never-Married", "Admin", "Wife", "White", "Female", 0, 0, 32, "United-States"], [22, "Private", "Bachelors", "Never-Married", "Admin", "Own-child", "White", "Male", 0, 0, 40, "United-States"], [54, "Self-emp-inc", "Bachelors", "Never-Married", "Admin", "Husband", "White", "Male", 15024, 0, 40, "United-States"]], "covered_true": [[17, "Private", "Dropout", "Never-Married", "Admin", "Own-child", "White", "Female", 0, 0, 20, "United-States"], [44, "Private", "Bachelors", "Never-Married", "Admin", "Not-in-family", "White", "Male", 0, 0, 12, "?"], [59, "Private", "Bachelors", "Never-Married", "Admin", "Unmarried", "White", "Female", 0, 0, 40, "United-States"], [46, "Private", "High School grad", "Never-Married", "Admin", "Husband", "White", "Male", 4064, 0, 55, "United-States"], [43, "Private", "High School grad", "Never-Married", "Admin", "Unmarried", "White", "Female", 0, 0, 40, "United-States"], [22, "Private", "High School grad", "Never-Married", "Admin", "Own-child", "White", "Male", 0, 0, 30, "United-States"], [22, "Private", "Bachelors", "Never-Married", "Admin", "Own-child", "White", "Male", 0, 0, 40, "United-States"], [29, "Private", "High School grad", "Never-Married", "Admin", "Husband", "White", "Male", 0, 0, 40, "United-States"], [34, "Private", "High School grad", "Never-Married", "Admin", "Husband", "White", "Male", 0, 0, 40, "United-States"], [25, "Private", "High School grad", "Never-Married", "Admin", "Own-child", "Asian-Pac-Islander", "Female", 0, 0, 40, "United-States"]], "covered_false": [[39, "Private", "Prof-School", "Never-Married", "Admin", "Husband", "White", "Male", 0, 0, 55, "United-States"], [54, "Self-emp-inc", "Bachelors", "Never-Married", "Admin", "Husband", "White", "Male", 15024, 0, 40, "United-States"], [34, "Private", "Associates", "Never-Married", "Admin", "Not-in-family", "Asian-Pac-Islander", "Female", 8614, 0, 60, "United-States"], [48, "Private", "Prof-School", "Never-Married", "Admin", "Husband", "White", "Male", 99999, 0, 50, "United-States"], [37, "State-gov", "Bachelors", "Never-Married", "Admin", "Not-in-family", "White", "Female", 8614, 0, 40, "United-States"], [36, "Private", "Bachelors", "Never-Married", "Admin", "Not-in-family", "White", "Male", 0, 0, 60, "United-States"], [30, "Private", "Bachelors", "Never-Married", "Admin", "Not-in-family", "White", "Male", 13550, 0, 45, "United-States"], [37, "Self-emp-not-inc", "High School grad", "Never-Married", "Admin", "Husband", "White", "Male", 99999, 0, 50, "United-States"], [53, "Private", "Masters", "Never-Married", "Admin", "Husband", "White", "Male", 0, 1902, 50, "United-States"], [42, "Local-gov", "High School grad", "Never-Married", "Admin", "Husband", "White", "Male", 5178, 0, 40, "United-States"]], "uncovered_true": [], "uncovered_false": []}], "all_precision": 0, "num_preds": 1000101, "names": ["Marital Status = Never-Married", "Occupation = Admin"], "instance": [[39, "State-gov", "Bachelors", "Never-Married", "Admin", "Not-in-family", "White", "Male", 2174, 0, 40, "United-States"]], "prediction": 0}}
```

The core explanation is:

```
"names": ["Marital Status = Never-Married", "Occupation = Admin"], "precision": 0.9558232931726908
```

This says the reason for low income prediction is due to their marital-status of never married and their admin occuptation. These feature values would cause this prediction 95.5% of the time from this model.


## Running without a pretrained explainer

Run as above but start the explainer with the locations of the individual training components it needs in a config map:

```
python -m alibiexplainer --explainer_name income --predict_url http://localhost:8080/models/income:predict --protocol seldon.http --http_port 8081 --type anchor_tabular --config '{"training_data_url":"file:///home/clive/go/src/github.com/kubeflow/kfserving/docs/samples/explanation/income/train.joblib","feature_names_url":"file:///home/clive/go/src/github.com/kubeflow/kfserving/docs/samples/explanation/income/features.joblib","categorical_map_url":"file:///home/clive/go/src/github.com/kubeflow/kfserving/docs/samples/explanation/income/category_map.joblib"}'
```
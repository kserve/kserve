# Example Anchors Tabular Explaination for Income Prediction

Train a model for income prediction.

```
python train.py 
```

This will create the following files:

  * `model.joblib` : pickle of model
  * `train.joblib` : pickle of training data
  * `features.joblib` : pickle of feature names list
  * `category_map.joblib` : pickle of map of categorical variables


Now, run a KFServing sklearn server with this model:

```
python -m sklearnserver --model_dir ./  --model_name income --protocol seldon.http
```

In a different terminal start the Alibi Explainer:

```
TRAINING_DATA_URL=./train.joblib FEATURE_NAMES_URL=./features.joblib CATEGORICAL_MAP_URL=./category_map.joblib python -m alibiexplainer --model_url http://localhost:8080/models/income:predict --protocol seldon.http --http_port 8081
```

You can now get an explaination for some particular features:

```
curl -H "Content-Type: application/json" -d '{"data":{"ndarray":[[39, 7, 1, 1, 1, 1, 4, 1, 2174, 0, 40, 9]]}}' http://localhost:8081/explain
```

You should receive an output like:

```JSON
{
  names: ["Marital Status = Never-Married", "Hours per week <= 40.00", "Relationship = Not-in-family"],
  precision: 0.9791666666666666,
  coverage: 0.0953,
  raw: {
    feature: [3, 10, 5],
    mean: [0.9121212121212121, 0.9462686567164179, 0.9791666666666666],
    precision: [0.9121212121212121, 0.9462686567164179, 0.9791666666666666],
    coverage: [0.3225, 0.2564, 0.0953],
    examples: [
      {
        covered: [
          [19, "Private", "High School grad", "Never-Married", "Service", "Own-child", "White", "Female", 0, 0, 15, "United-States"],
          [29, "Private", "High School grad", "Never-Married", "Sales", "Unmarried", "Black", "Female", 0, 1138, 40, "United-States"],
          [18, "Private", "High School grad", "Never-Married", "Other", "Own-child", "White", "Male", 2176, 0, 20, "United-States"],
          [44, "Local-gov", "High School grad", "Never-Married", "Admin", "Unmarried", "White", "Female", 0, 0, 40, "United-States"],
          [36, "Self-emp-not-inc", "High School grad", "Never-Married", "White-Collar", "Husband", "White", "Male", 0, 0, 50, "United-States"],
          [33, "Private", "Masters", "Never-Married", "Professional", "Husband", "White", "Male", 0, 0, 37, "United-States"],
          [57, "Local-gov", "Bachelors", "Never-Married", "Professional", "Husband", "Asian-Pac-Islander", "Male", 0, 0, 35, "British-Commonwealth"],
          [58, "Private", "Doctorate", "Never-Married", "Professional", "Not-in-family", "White", "Male", 0, 0, 8, "United-States"],
          [32, "Federal-gov", "Associates", "Never-Married", "Professional", "Other-relative", "Asian-Pac-Islander", "Male", 0, 0, 40, "United-States"],
          [61, "Private", "High School grad", "Never-Married", "Other", "Husband", "White", "Male", 0, 0, 25, "United-States"]
        ],
        covered_true: [
          [24, "Private", "High School grad", "Never-Married", "Sales", "Not-in-family", "White", "Male", 0, 0, 40, "United-States"],
          [26, "Private", "Dropout", "Never-Married", "Blue-Collar", "Not-in-family", "White", "Male", 0, 0, 55, "Latin-America"],
          [43, "Private", "High School grad", "Never-Married", "Blue-Collar", "Not-in-family", "White", "Female", 0, 0, 24, "United-States"],
          [51, "Federal-gov", "High School grad", "Never-Married", "Professional", "Not-in-family", "Black", "Female", 0, 0, 40, "United-States"],
          [19, "State-gov", "High School grad", "Never-Married", "Admin", "Own-child", "White", "Female", 0, 0, 40, "United-States"],
          [18, "Private", "Dropout", "Never-Married", "Service", "Own-child", "White", "Female", 0, 0, 15, "United-States"],
          [56, "Private", "Associates", "Never-Married", "Blue-Collar", "Husband", "White", "Male", 0, 0, 48, "United-States"],
          [52, "Private", "Bachelors", "Never-Married", "White-Collar", "Husband", "White", "Male", 0, 0, 40, "United-States"],
          [37, "Private", "High School grad", "Never-Married", "Blue-Collar", "Not-in-family", "Black", "Male", 0, 0, 40, "United-States"],
          [38, "Private", "Bachelors", "Never-Married", "Professional", "Husband", "White", "Male", 0, 0, 50, "United-States"]
        ],
        covered_false: [
          [42, "Private", "Masters", "Never-Married", "Sales", "Husband", "White", "Male", 0, 0, 50, "United-States"],
          [43, "Private", "Bachelors", "Never-Married", "White-Collar", "Husband", "White", "Male", 0, 0, 45, "United-States"],
          [54, "Private", "High School grad", "Never-Married", "Blue-Collar", "Husband", "White", "Male", 15024, 0, 60, "United-States"],
          [40, "State-gov", "Masters", "Never-Married", "Professional", "Wife", "White", "Female", 15024, 0, 37, "United-States"],
          [40, "Private", "High School grad", "Never-Married", "Sales", "Husband", "White", "Male", 7298, 0, 45, "United-States"],
          [50, "Local-gov", "Prof-School", "Never-Married", "White-Collar", "Not-in-family", "Black", "Female", 0, 0, 52, "United-States"],
          [43, "Local-gov", "Masters", "Never-Married", "Professional", "Unmarried", "Black", "Female", 9562, 0, 40, "United-States"],
          [36, "State-gov", "High School grad", "Never-Married", "Other", "Husband", "White", "Male", 7298, 0, 40, "United-States"],
          [65, "Self-emp-not-inc", "Prof-School", "Never-Married", "Professional", "Not-in-family", "White", "Male", 0, 1668, 40, "United-States"],
          [37, "Self-emp-not-inc", "High School grad", "Never-Married", "Sales", "Husband", "White", "Male", 99999, 0, 50, "United-States"]
        ],
        uncovered_true: [ ],
        uncovered_false: [ ]
      },
      {
        covered: [
          [46, "Federal-gov", "Bachelors", "Never-Married", "Other", "Husband", "White", "Male", 3103, 0, 40, "United-States"],
          [36, "Private", "High School grad", "Never-Married", "Admin", "Not-in-family", "White", "Male", 2463, 0, 40, "United-States"],
          [47, "Self-emp-not-inc", "High School grad", "Never-Married", "Blue-Collar", "Not-in-family", "White", "Male", 0, 0, 40, "United-States"],
          [41, "Private", "Dropout", "Never-Married", "Blue-Collar", "Other-relative", "Black", "Female", 0, 0, 40, "United-States"],
          [21, "Private", "High School grad", "Never-Married", "Admin", "Not-in-family", "White", "Female", 0, 0, 30, "South-America"],
          [62, "Private", "Dropout", "Never-Married", "Blue-Collar", "Husband", "Asian-Pac-Islander", "Male", 0, 0, 40, "SE-Asia"],
          [33, "Self-emp-not-inc", "Bachelors", "Never-Married", "Sales", "Not-in-family", "White", "Female", 0, 0, 40, "United-States"],
          [32, "Private", "Associates", "Never-Married", "Blue-Collar", "Husband", "White", "Male", 0, 0, 40, "United-States"],
          [38, "Private", "High School grad", "Never-Married", "Blue-Collar", "Husband", "White", "Male", 0, 0, 40, "United-States"],
          [46, "Private", "High School grad", "Never-Married", "Sales", "Husband", "White", "Male", 0, 0, 36, "United-States"]
        ],
        covered_true: [
          [23, "Private", "High School grad", "Never-Married", "Admin", "Other-relative", "Asian-Pac-Islander", "Male", 0, 0, 14, "Latin-America"],
          [27, "Private", "Dropout", "Never-Married", "Blue-Collar", "Other-relative", "White", "Male", 0, 0, 40, "Latin-America"],
          [19, "Private", "High School grad", "Never-Married", "Service", "Own-child", "White", "Female", 0, 0, 15, "United-States"],
          [38, "Private", "High School grad", "Never-Married", "Blue-Collar", "Husband", "White", "Male", 0, 0, 40, "United-States"],
          [58, "Private", "Dropout", "Never-Married", "Service", "Husband", "White", "Male", 0, 0, 20, "United-States"],
          [47, "Private", "High School grad", "Never-Married", "Professional", "Husband", "White", "Male", 0, 0, 40, "British-Commonwealth"],
          [26, "Private", "Bachelors", "Never-Married", "Professional", "Not-in-family", "White", "Male", 0, 0, 38, "United-States"],
          [21, "Private", "High School grad", "Never-Married", "Blue-Collar", "Own-child", "White", "Male", 0, 0, 12, "United-States"],
          [43, "Private", "Dropout", "Never-Married", "Blue-Collar", "Wife", "Asian-Pac-Islander", "Female", 0, 0, 40, "SE-Asia"],
          [39, "Private", "Bachelors", "Never-Married", "Admin", "Husband", "White", "Male", 0, 0, 40, "United-States"]
        ],
        covered_false: [
          [50, "Self-emp-not-inc", "Prof-School", "Never-Married", "Professional", "Husband", "White", "Male", 0, 2415, 30, "United-States"],
          [51, "Private", "Doctorate", "Never-Married", "Professional", "Husband", "Asian-Pac-Islander", "Male", 0, 0, 40, "Euro_2"],
          [43, "Local-gov", "Masters", "Never-Married", "Professional", "Unmarried", "Black", "Female", 9562, 0, 40, "United-States"],
          [37, "Private", "High School grad", "Never-Married", "Blue-Collar", "Other-relative", "Amer-Indian-Eskimo", "Male", 27828, 0, 40, "United-States"],
          [36, "State-gov", "High School grad", "Never-Married", "Other", "Husband", "White", "Male", 7298, 0, 40, "United-States"],
          [46, "Private", "Doctorate", "Never-Married", "Professional", "Husband", "White", "Male", 0, 0, 40, "United-States"],
          [40, "State-gov", "Masters", "Never-Married", "Professional", "Wife", "White", "Female", 15024, 0, 37, "United-States"],
          [30, "Private", "Associates", "Never-Married", "Blue-Collar", "Own-child", "White", "Male", 0, 0, 40, "United-States"],
          [55, "Federal-gov", "Prof-School", "Never-Married", "White-Collar", "Husband", "White", "Male", 15024, 0, 40, "United-States"],
          [49, "Private", "High School grad", "Never-Married", "Blue-Collar", "Husband", "White", "Male", 7298, 0, 40, "United-States"]
        ],
        uncovered_true: [ ],
        uncovered_false: [ ]
      },
      {
        covered: [
          [34, "Private", "High School grad", "Never-Married", "White-Collar", "Not-in-family", "White", "Female", 0, 0, 40, "United-States"],
          [77, "Private", "Bachelors", "Never-Married", "Professional", "Not-in-family", "White", "Female", 0, 0, 10, "United-States"],
          [39, "Private", "High School grad", "Never-Married", "Blue-Collar", "Not-in-family", "White", "Male", 0, 0, 40, "United-States"],
          [58, "Local-gov", "Masters", "Never-Married", "Professional", "Not-in-family", "White", "Male", 0, 0, 35, "United-States"],
          [65, "Self-emp-not-inc", "Prof-School", "Never-Married", "Professional", "Not-in-family", "White", "Male", 0, 1668, 40, "United-States"],
          [41, "Private", "High School grad", "Never-Married", "Blue-Collar", "Not-in-family", "White", "Male", 0, 0, 40, "Euro_2"],
          [20, "Private", "High School grad", "Never-Married", "Admin", "Not-in-family", "Black", "Male", 0, 0, 20, "United-States"],
          [59, "Federal-gov", "Masters", "Never-Married", "White-Collar", "Not-in-family", "White", "Male", 0, 0, 40, "United-States"],
          [31, "Private", "Dropout", "Never-Married", "Other", "Not-in-family", "White", "Male", 0, 0, 40, "United-States"],
          [30, "Self-emp-inc", "High School grad", "Never-Married", "White-Collar", "Not-in-family", "White", "Male", 4650, 0, 40, "United-States"]
        ],
        covered_true: [
          [40, "Private", "Dropout", "Never-Married", "Blue-Collar", "Not-in-family", "White", "Male", 0, 0, 40, "United-States"],
          [55, "Private", "Dropout", "Never-Married", "Blue-Collar", "Not-in-family", "White", "Male", 0, 0, 40, "United-States"],
          [58, "Local-gov", "Masters", "Never-Married", "Professional", "Not-in-family", "White", "Male", 0, 0, 35, "United-States"],
          [57, "Private", "Bachelors", "Never-Married", "Sales", "Not-in-family", "White", "Male", 0, 0, 40, "United-States"],
          [28, "Private", "Dropout", "Never-Married", "Admin", "Not-in-family", "White", "Male", 0, 0, 40, "United-States"],
          [42, "State-gov", "Bachelors", "Never-Married", "Admin", "Not-in-family", "White", "Female", 0, 0, 40, "United-States"],
          [36, "Private", "High School grad", "Never-Married", "Admin", "Not-in-family", "White", "Male", 2463, 0, 40, "United-States"],
          [43, "Private", "Bachelors", "Never-Married", "Blue-Collar", "Not-in-family", "Black", "Male", 0, 0, 40, "United-States"],
          [60, "Private", "Dropout", "Never-Married", "Service", "Not-in-family", "White", "Male", 0, 0, 40, "United-States"],
          [22, "Private", "High School grad", "Never-Married", "Service", "Not-in-family", "White", "Male", 0, 0, 40, "United-States"]
        ],
        covered_false: [
          [40, "Self-emp-not-inc", "Prof-School", "Never-Married", "Professional", "Not-in-family", "Amer-Indian-Eskimo", "Male", 0, 1977, 35, "United-States"],
          [65, "Self-emp-not-inc", "Prof-School", "Never-Married", "Professional", "Not-in-family", "White", "Male", 0, 1668, 40, "United-States"],
          [52, "Private", "High School grad", "Never-Married", "Blue-Collar", "Not-in-family", "White", "Male", 0, 0, 40, "United-States"],
          [41, "Self-emp-inc", "Bachelors", "Never-Married", "Sales", "Not-in-family", "White", "Male", 99999, 0, 40, "United-States"]
        ],
        uncovered_true: [ ],
        uncovered_false: [ ]
      }
    ],
    all_precision: 0,
    num_preds: 1000101,
    names: ["Marital Status = Never-Married", "Hours per week <= 40.00", "Relationship = Not-in-family"],
    instance: [
      [39, "State-gov", "Bachelors", "Never-Married", "Admin", "Not-in-family", "White", "Male", 2174, 0, 40, "United-States"]
    ],
    prediction: 0
  }
}
```

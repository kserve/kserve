# Example Anchors Tabular Explanation for Income Prediction

This example uses a [US income dataset](https://archive.ics.uci.edu/ml/datasets/adult)

You can also try out the [Jupyter notebook](income_explanations.ipynb).

We can create a InferenceService with a trained sklearn predictor for this dataset and an associated model explainer. The black box explainer algorithm we will use is the Tabular version of Anchors from the [Alibi open source library](https://github.com/SeldonIO/alibi). More details on this algorithm and configuration settings that can be set can be found in the [Seldon Alibi documentation](https://docs.seldon.io/projects/alibi/en/stable/).

The InferenceService is shown below:

```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "income"
spec:
  default:
    predictor:
      minReplicas: 1
      sklearn:
        storageUri: "gs://seldon-models/sklearn/income/model"
        resources:
          requests:
            cpu: 0.1
          limits:
            cpu: 1
    explainer:
      minReplicas: 1
      alibi:
        type: AnchorTabular
        storageUri: "gs://seldon-models/sklearn/income/explainer-py37-0.6.0"
        resources:
          requests:
            cpu: 0.1
          limits:
            cpu: 1
```
Create this InferenceService:

```
kubectl create -f income.yaml
```

Set up some environment variables for the model name and cluster entrypoint.

```
MODEL_NAME=income
INGRESS_GATEWAY=istio-ingressgateway
CLUSTER_IP=$(kubectl -n istio-system get service $INGRESS_GATEWAY -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
```

Test the predictor:

```
curl -H "Host: ${MODEL_NAME}.default.example.com" http://$CLUSTER_IP/v1/models/$MODEL_NAME:predict -d '{"instances":[[39, 7, 1, 1, 1, 1, 4, 1, 2174, 0, 40, 9]]}'
```

You should receive the response showing the prediction is for low salary:

```
{"predictions": [0]}
```

Now lets get an explanation for this:


```
curl -v -H "Host: ${MODEL_NAME}.default.example.com" http://$CLUSTER_IP/v1/models/$MODEL_NAME:explain -d '{"instances":[[39, 7, 1, 1, 1, 1, 4, 1, 2174, 0, 40, 9]]}'
```

The returned explanation will be like:

```
{
  "names": [
    "Marital Status = Never-Married",
    "Workclass = State-gov"
  ],
  "precision": 0.9724770642201835,
  "coverage": 0.0147,
  "raw": {
    "feature": [
      3,
      1
    ],
    "mean": [
      0.9129746835443038,
      0.9724770642201835
    ],
    "precision": [
      0.9129746835443038,
      0.9724770642201835
    ],
    "coverage": [
      0.3327,
      0.0147
    ],
    "examples": [
      {
        "covered": [
          [
            30,
            "Self-emp-not-inc",
            "Bachelors",
            "Never-Married",
            "Sales",
            "Unmarried",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ],
          [
            69,
            "Private",
            "Dropout",
            "Never-Married",
            "Blue-Collar",
            "Husband",
            "White",
            "Male",
            9386,
            "Capital Loss <= 0.00",
            60,
            "United-States"
          ],
          [
            44,
            "Local-gov",
            "Bachelors",
            "Never-Married",
            "White-Collar",
            "Husband",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            52,
            "United-States"
          ],
          [
            59,
            "Private",
            "High School grad",
            "Never-Married",
            "White-Collar",
            "Husband",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            50,
            "United-States"
          ],
          [
            55,
            "Private",
            "Bachelors",
            "Never-Married",
            "White-Collar",
            "Husband",
            "White",
            "Male",
            15024,
            "Capital Loss <= 0.00",
            55,
            "United-States"
          ],
          [
            32,
            "?",
            "Bachelors",
            "Never-Married",
            "?",
            "Unmarried",
            "White",
            "Female",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            32,
            "United-States"
          ],
          [
            47,
            "Private",
            "Dropout",
            "Never-Married",
            "Blue-Collar",
            "Unmarried",
            "Black",
            "Female",
            6849,
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ],
          [
            35,
            "Private",
            "Associates",
            "Never-Married",
            "Service",
            "Not-in-family",
            "White",
            "Female",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            65,
            "United-States"
          ],
          [
            32,
            "Private",
            "High School grad",
            "Never-Married",
            "Blue-Collar",
            "Husband",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ],
          [
            48,
            "Private",
            "Masters",
            "Never-Married",
            "White-Collar",
            "Husband",
            "Black",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            45,
            "United-States"
          ]
        ],
        "covered_true": [
          [
            32,
            "Private",
            "High School grad",
            "Never-Married",
            "Blue-Collar",
            "Husband",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ],
          [
            44,
            "Local-gov",
            "Bachelors",
            "Never-Married",
            "White-Collar",
            "Husband",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            52,
            "United-States"
          ],
          [
            36,
            "Private",
            "High School grad",
            "Never-Married",
            "Blue-Collar",
            "Unmarried",
            "White",
            "Female",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            30,
            "United-States"
          ],
          [
            56,
            "Private",
            "High School grad",
            "Never-Married",
            "Blue-Collar",
            "Unmarried",
            "Black",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ],
          [
            49,
            "Local-gov",
            "High School grad",
            "Never-Married",
            "Service",
            "Unmarried",
            "White",
            "Female",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            30,
            "United-States"
          ],
          [
            20,
            "?",
            "High School grad",
            "Never-Married",
            "?",
            "Own-child",
            "White",
            "Female",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            10,
            "United-States"
          ],
          [
            22,
            "?",
            "High School grad",
            "Never-Married",
            "?",
            "Own-child",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            "Hours per week > 45.00",
            "United-States"
          ],
          [
            29,
            "Private",
            "High School grad",
            "Never-Married",
            "Service",
            "Own-child",
            "Asian-Pac-Islander",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            25,
            "SE-Asia"
          ],
          [
            45,
            "Local-gov",
            "Masters",
            "Never-Married",
            "Professional",
            "Unmarried",
            "White",
            "Female",
            1506,
            "Capital Loss <= 0.00",
            45,
            "United-States"
          ],
          [
            27,
            "Private",
            "High School grad",
            "Never-Married",
            "Blue-Collar",
            "Not-in-family",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            50,
            "United-States"
          ]
        ],
        "covered_false": [
          [
            29,
            "Private",
            "Bachelors",
            "Never-Married",
            "Service",
            "Husband",
            "White",
            "Male",
            7298,
            "Capital Loss <= 0.00",
            42,
            "United-States"
          ],
          [
            56,
            "Private",
            "Associates",
            "Never-Married",
            "Sales",
            "Husband",
            "White",
            "Male",
            15024,
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ],
          [
            47,
            "Private",
            "Masters",
            "Never-Married",
            "Sales",
            "Not-in-family",
            "White",
            "Male",
            27828,
            "Capital Loss <= 0.00",
            60,
            "United-States"
          ],
          [
            40,
            "Private",
            "Associates",
            "Never-Married",
            "White-Collar",
            "Husband",
            "White",
            "Male",
            7688,
            "Capital Loss <= 0.00",
            44,
            "United-States"
          ],
          [
            55,
            "Self-emp-not-inc",
            "High School grad",
            "Never-Married",
            "White-Collar",
            "Not-in-family",
            "White",
            "Male",
            34095,
            "Capital Loss <= 0.00",
            60,
            "United-States"
          ],
          [
            53,
            "Private",
            "Masters",
            "Never-Married",
            "White-Collar",
            "Husband",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            48,
            "United-States"
          ],
          [
            47,
            "Federal-gov",
            "Doctorate",
            "Never-Married",
            "Professional",
            "Husband",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ],
          [
            53,
            "Private",
            "High School grad",
            "Never-Married",
            "White-Collar",
            "Husband",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            1977,
            40,
            "United-States"
          ],
          [
            46,
            "Private",
            "Bachelors",
            "Never-Married",
            "Sales",
            "Not-in-family",
            "White",
            "Male",
            8614,
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ],
          [
            44,
            "Local-gov",
            "Prof-School",
            "Never-Married",
            "Professional",
            "Not-in-family",
            "White",
            "Male",
            10520,
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ]
        ],
        "uncovered_true": [],
        "uncovered_false": []
      },
      {
        "covered": [
          [
            41,
            "State-gov",
            "High School grad",
            "Never-Married",
            "White-Collar",
            "Not-in-family",
            "White",
            "Female",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ],
          [
            64,
            "State-gov",
            "High School grad",
            "Never-Married",
            "Blue-Collar",
            "Not-in-family",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ],
          [
            33,
            "State-gov",
            "High School grad",
            "Never-Married",
            "Service",
            "Unmarried",
            "Black",
            "Female",
            1831,
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ],
          [
            35,
            "State-gov",
            "High School grad",
            "Never-Married",
            "Blue-Collar",
            "Husband",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            60,
            "United-States"
          ],
          [
            25,
            "State-gov",
            "Dropout",
            "Never-Married",
            "Blue-Collar",
            "Own-child",
            "Black",
            "Female",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ],
          [
            40,
            "State-gov",
            "Associates",
            "Never-Married",
            "Blue-Collar",
            "Husband",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ],
          [
            19,
            "State-gov",
            "High School grad",
            "Never-Married",
            "Blue-Collar",
            "Other-relative",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            20,
            "United-States"
          ],
          [
            44,
            "State-gov",
            "Dropout",
            "Never-Married",
            "Blue-Collar",
            "Husband",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            88,
            "United-States"
          ],
          [
            80,
            "State-gov",
            "Associates",
            "Never-Married",
            "Blue-Collar",
            "Husband",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            24,
            "United-States"
          ],
          [
            21,
            "State-gov",
            "High School grad",
            "Never-Married",
            "Professional",
            "Own-child",
            "White",
            "Female",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            20,
            "United-States"
          ]
        ],
        "covered_true": [
          [
            22,
            "State-gov",
            "High School grad",
            "Never-Married",
            "Service",
            "Husband",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            25,
            "United-States"
          ],
          [
            49,
            "State-gov",
            "High School grad",
            "Never-Married",
            "Service",
            "Not-in-family",
            "White",
            "Female",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ],
          [
            22,
            "State-gov",
            "Bachelors",
            "Never-Married",
            "?",
            "Not-in-family",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            25,
            "United-States"
          ],
          [
            31,
            "State-gov",
            "Bachelors",
            "Never-Married",
            "Professional",
            "Husband",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            50,
            "United-States"
          ],
          [
            18,
            "State-gov",
            "Dropout",
            "Never-Married",
            "Blue-Collar",
            "Not-in-family",
            "Black",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ],
          [
            56,
            "State-gov",
            "High School grad",
            "Never-Married",
            "Blue-Collar",
            "Unmarried",
            "Black",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ],
          [
            26,
            "State-gov",
            "Dropout",
            "Never-Married",
            "Service",
            "Unmarried",
            "White",
            "Female",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ],
          [
            38,
            "State-gov",
            "Bachelors",
            "Never-Married",
            "White-Collar",
            "Husband",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ],
          [
            52,
            "State-gov",
            "High School grad",
            "Never-Married",
            "Blue-Collar",
            "Husband",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            70,
            "United-States"
          ],
          [
            25,
            "State-gov",
            "Associates",
            "Never-Married",
            "Professional",
            "Wife",
            "White",
            "Female",
            "Capital Gain <= 0.00",
            1887,
            40,
            "United-States"
          ]
        ],
        "covered_false": [
          [
            46,
            "State-gov",
            "Prof-School",
            "Never-Married",
            "Professional",
            "Husband",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            45,
            "United-States"
          ],
          [
            42,
            "State-gov",
            "Bachelors",
            "Never-Married",
            "White-Collar",
            "Husband",
            "White",
            "Male",
            15024,
            "Capital Loss <= 0.00",
            50,
            "United-States"
          ],
          [
            46,
            "State-gov",
            "Prof-School",
            "Never-Married",
            "Professional",
            "Husband",
            "White",
            "Male",
            15024,
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ],
          [
            54,
            "State-gov",
            "Doctorate",
            "Never-Married",
            "White-Collar",
            "Not-in-family",
            "White",
            "Female",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            40,
            "United-States"
          ],
          [
            42,
            "State-gov",
            "Masters",
            "Never-Married",
            "White-Collar",
            "Not-in-family",
            "White",
            "Female",
            14084,
            "Capital Loss <= 0.00",
            60,
            "United-States"
          ],
          [
            37,
            "State-gov",
            "Masters",
            "Never-Married",
            "Professional",
            "Husband",
            "White",
            "Male",
            "Capital Gain <= 0.00",
            "Capital Loss <= 0.00",
            45,
            "United-States"
          ]
        ],
        "uncovered_true": [],
        "uncovered_false": []
      }
    ],
    "all_precision": 0,
    "num_preds": 1000101,
    "names": [
      "Marital Status = Never-Married",
      "Workclass = State-gov"
    ],
    "instance": [
      [
        39
      ],
      [
        7
      ],
      [
        "28.00 < Age <= 37.00"
      ],
      [
        "28.00 < Age <= 37.00"
      ],
      [
        "28.00 < Age <= 37.00"
      ],
      [
        "28.00 < Age <= 37.00"
      ],
      [
        4
      ],
      [
        "28.00 < Age <= 37.00"
      ],
      [
        2174
      ],
      [
        "Age <= 28.00"
      ],
      [
        40
      ],
      [
        9
      ]
    ],
    "prediction": 0
  }
}

```


## Local Testing

If you wish to test locally first install the requirements:

```
pip install -r requirements.txt
```

Now train the model:

```
make train
```

You can then store the `model.joblib` for the model and `explainer.dill` for the explainer in a bucket accessible from your Kubernetes cluster.



# Example Anchors Text Explaination for Movie Sentiment

Train a model for movie sentiment.

```
python train.py 
```

This will create the following file:

  * `model.joblib` : pickle of model.

Now, run a KFServing sklearn server with this model:

```
python -m sklearnserver --model_dir .  --model_name moviesentiment --protocol tensorflow.http
```

Test the running model:

```
curl -H "Content-Type: application/json" -d '{"instances":["This is a good book ."]}' http://localhost:8080/models/moviesentiment:predict
```

This should produce a positive sentiment:

```
{"predictions": [1]}
```

Test a different example:

```
curl -H "Content-Type: application/json" -d '{"instances":["This is a bad book ."]}' http://localhost:8080/models/moviesentiment:predict
```

This should produce negative sentiment:

```
{"predictions": [0]}
```


In a different terminal start the Alibi Explainer:

```
python -m alibiexplainer --explainer_name moviesentiment --predict_url http://localhost:8080/models/moviesentiment:predict --protocol tensorflow.http --http_port 8081 --type anchor_text
```

You can now get an explaination for some particular features:

```
curl -H "Content-Type: application/json" -d '{"instances":["This is a bad book ."]}' http://localhost:8081/models/moviesentiment:explain
```

The returned explanation will be like:

```
{
  "names": [
    "bad"
  ],
  "precision": 1,
  "coverage": 0.5007,
  "raw": {
    "feature": [
      3
    ],
    "mean": [
      1
    ],
    "precision": [
      1
    ],
    "coverage": [
      0.5007
    ],
    "examples": [
      {
        "covered": [
          [
            "This is a bad book UNK"
          ],
          [
            "UNK is UNK bad book UNK"
          ],
          [
            "UNK is UNK bad book ."
          ],
          [
            "This UNK UNK bad book UNK"
          ],
          [
            "This is UNK bad UNK ."
          ],
          [
            "UNK UNK UNK bad book ."
          ],
          [
            "UNK is a bad UNK UNK"
          ],
          [
            "UNK UNK a bad UNK ."
          ],
          [
            "UNK UNK a bad book UNK"
          ],
          [
            "UNK is UNK bad book ."
          ]
        ],
        "covered_true": [
          [
            "UNK is UNK bad UNK UNK"
          ],
          [
            "UNK is UNK bad UNK UNK"
          ],
          [
            "UNK is UNK bad book UNK"
          ],
          [
            "This UNK UNK bad book UNK"
          ],
          [
            "UNK UNK a bad book ."
          ],
          [
            "This is UNK bad UNK UNK"
          ],
          [
            "UNK UNK UNK bad UNK UNK"
          ],
          [
            "This is UNK bad UNK ."
          ],
          [
            "This is UNK bad UNK ."
          ],
          [
            "This UNK a bad UNK ."
          ]
        ],
        "covered_false": [],
        "uncovered_true": [],
        "uncovered_false": []
      }
    ],
    "all_precision": 0,
    "num_preds": 1000101,
    "names": [
      "bad"
    ],
    "positions": [
      10
    ],
    "instance": "This is a bad book .",
    "prediction": 0
  }
}

```

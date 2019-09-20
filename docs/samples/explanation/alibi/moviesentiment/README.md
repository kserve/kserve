# Example Anchors Text Explaination for Movie Sentiment

This example uses a [movie sentiment dataset](http://www.cs.cornell.edu/people/pabo/movie-review-data/).

We can create a KFService with a trained sklearn predictor for this dataset and an associated explainer. The black box explainer algorithm we will use is the Text version of Anchors from the [Alibi open source library](https://github.com/SeldonIO/alibi). More details on this algorithm and configuration settings that can be set can be found in the [Seldon Alibi documentation](https://docs.seldon.io/projects/alibi/en/stable/).

The KFService is shown below:

```
apiVersion: "serving.kubeflow.org/v1alpha2"
kind: "KFService"
metadata:
  name: "moviesentiment"
spec:
  default:
    predictor:
      sklearn:
        storageUri: "gs://seldon-models/sklearn/moviesentiment"
        resources:
          requests:
            cpu: 0.1
    explainer:
      alibi:
        type: AnchorText
        resources:
          requests:
            cpu: 0.1
        
```

Create this KfService:

```
kubectl create -f moviesentiment.yaml
```

Set up some environment variables for the model name and cluster entrypoint:

```
MODEL_NAME=moviesentiment
CLUSTER_IP=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
```

Test the predictor on an example sentence:

```
curl -H "Host: ${MODEL_NAME}-predict.default.example.com" http://$CLUSTER_IP/v1/models/$MODEL_NAME:predict -d '{"instances":["This is a bad book ."]}'
```

You should receive the response showing negative sentiment:

```
{"predictions": [0]}
```

Test on another sentence:

```
curl -H "Host: ${MODEL_NAME}-predict.default.example.com" http://$CLUSTER_IP/v1/models/$MODEL_NAME:predict -d '{"instances":["This is a great book ."]}'
```

You should receive the response showing positive sentiment:

```
{"predictions": [1]}
```

Now lets get an explanation for the first sentence:


```
curl -v -H "Host: ${MODEL_NAME}-explain.default.example.com" http://$CLUSTER_IP/v1/models/$MODEL_NAME:explain -d '{"instances":["This is a bad book ."]}'
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

This shows the key word "bad" was indetified and examples show it in context using the default "UKN" placeholder for surrounding words.


## Local Testing

If you wish to test locally first install the requirements:

```
pip install -r requirements.txt
```

Now train the model:

```
make train
```

You can then store the `model.joblib` in a bucket accessible from your Kubernetes cluster.


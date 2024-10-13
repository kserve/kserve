# Logistic Regression Model on the German Credit dataset

## Build a development docker image

To build a development image first download these files and move them into the `server/` folder
- https://archive.ics.uci.edu/ml/machine-learning-databases/statlog/german/german.data
- https://archive.ics.uci.edu/ml/machine-learning-databases/statlog/german/german.doc

First build your docker image by changing directory to kfserving/python and replacing dockeruser with your docker username in the snippet below (running this will take some time).

```
docker build -t dockeruser/aifserver:latest -f aiffairness.Dockerfile .
```

Then push your docker image to your dockerhub repo (this will take some time)

```
docker push dockeruser/aifserver:latest
```

Once your docker image is pushed you can pull the image from dockeruser/aifserver:latest when deploying an inferenceservice by specifying the image in the yaml file.


# Running the Container Image locally

Replace the **CMD** argument with the following command to run the container locally.

```bash
CMD ["python", "model.py",
     "-m", "aifserver",
     "--predictor_host", "german-credit-predictor-default.default.svc.cluster.local",
     "--model_name", "german-credit",
     "--feature_names", "age", "sex", "credit_history=Delay", "credit_history=None/Paid", "credit_history=Other", "savings=500+", "savings=<500", "savings=Unknown/None", "employment=1-4 years", "employment=4+ years", "employment=unemployed",
     "--label_names", "credit",
     "--favorable_label", "1",
     "--unfavorable_label", "2",
     "--privileged_groups", "{\"age\": 1}",
     "--unprivileged_groups", "{\"age\": 0}"]
```

Once the image is built, you can run the request to test it:

```bash
# make sure you are in the example's root directory where the `query_bias.py` script and the `input.json` file are located.
python query_bias.py http://localhost:8080/v1/models/german-credit:predict input.json
```
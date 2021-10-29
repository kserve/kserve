# Bias detection on an InferenceService using AIF360

## End-to-end bias detection with german-credit dataset

This is an example of how to get bias metrics using [AI Fairness 360 (AIF360)](https://ai-fairness-360.org/) on KFServing. AI Fairness 360, an LF AI incubation project, is an extensible open source toolkit that can help users examine, report, and mitigate discrimination and bias in machine learning models throughout the AI application lifecycle. 

We will be using the German Credit dataset maintained by the [UC Irvine Machine Learning Repository](https://archive.ics.uci.edu/ml/index.php). The German Credit dataset is a dataset that contains data as to whether or not a creditor gave a loan applicant access to a loan along with data about the applicant. The data includes relevant data on an applicant's credit history, savings, and employment as well as some data on the applicant's demographic such as age, sex, and marital status. Data like credit history, savings, and employment can be used by creditors to accurately predict the probability that an applicant will repay their loans, however, data such as age and sex should not be used to decide whether an applicant should be given a loan. 

We would like to be able to check if these "protected classes" are being used in a model's predictions. In this example we will feed the model some predictions and calculate metrics based off of the predictions the model makes. We will be using KFServing payload logging capability collect the metrics. These metrics will give insight as to whether or not the model is biased for or against any protected classes. In this example we will look at the bias our deployed model has on those of age > 25 vs. those of age <= 25 and see if creditors are treating either unfairly.

### Create the InferenceService

Apply the CRD

```
kubectl apply -f bias.yaml
```

Expected Output

```
$ inferenceservice.serving.kubeflow.org/german-credit created
```

### Deploy the message dumper (sample backend receiver for payload logs)

Apply the message-dumper CRD which will collect the logs that are created when running predictions on the inferenceservice. In production setup, instead of message-dumper Kafka can be used to receive payload logs

```
kubectl apply -f message-dumper.yaml
```

Expected Output

```
service.serving.knative.dev/message-dumper created
```

### Run a prediction

The first step is to [determine the ingress IP and ports](../../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

```
MODEL_NAME=german-credit
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}' | cut -d "/" -f 3)
python simulate_predicts.py http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict ${SERVICE_HOSTNAME}
```

### Process payload logs for metrics calculation

Run `json_from_logs.py` which will craft a payload that AIF can interpret. First, the events logs are taken from the message-dumper and then those logs are parsed to match inputs with outputs. Then the input/outputs pairs are all combined into a list of inputs and a list of outputs for AIF to interpret. A `data.json` file should have been created in this folder which contains the json payload.

```
python json_from_logs.py
```

### Run an explanation

Finally, now that we have collected a number of our model's predictions and their corresponding inputs we will send these to the AIF server to calculate the bias metrics.

```
python query_bias.py http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:explain ${SERVICE_HOSTNAME} input.json
```

### Interpreting the results

Now let's look at one of the metrics. In this example disparate impact represents the ratio between the probability of applicants of the privileged class (age > 25) getting a loan and the probability of applicants of the unprivileged class (age <= 25) getting a loan `P(Y=1|D=privileged)/P(Y=1|D=unprivileged)`. Since, in the sample output below, the disparate impact is less that 1 then the probability that an applicant whose age is greater than 25 gets a loan is significantly higher than the probability that an applicant whose age is less than or equal to 25 gets a loan. This in and of itself is not proof that the model is biased, but does hint that there may be some bias and a deeper look may be needed.

```
bash-3.2$ python query_bias.py http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:explain ${SERVICE_HOSTNAME} input.json
Sending bias query...
TIME TAKEN:  0.21137404441833496
<Response [200]>
base_rate :  0.9329608938547486
consistency :  [0.982122905027933]
disparate_impact :  0.52
num_instances :  179.0
num_negatives :  12.0
num_positives :  167.0
statistical_parity_difference :  -0.48
```

# Dataset

The dataset used in this example is the German Credit dataset maintained by the [UC Irvine Machine Learning Repository](https://archive.ics.uci.edu/ml/index.php).

Dua, D. and Graff, C. (2019). UCI Machine Learning Repository [http://archive.ics.uci.edu/ml]. Irvine, CA: University of California, School of Information and Computer Science.

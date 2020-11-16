
# QA Inference with BERT model using Triton Inference Server
Bidirectional Embedding Representations from Transformers (BERT), is a method of pre-training language representations which obtains state-of-the-art results on a wide array of Natural Language Processing (NLP) tasks.

This example demonstrates
- Inference on Question Answering (QA) task with BERT Base/Large model
- The use of fine-tuned NVIDIA BERT models
- Deploy the BERT model on KFServing with Transformer for BERT tokenizer
- Use of BERT model with Triton Inference Server

We can run inference on a fine-tuned BERT model for tasks like Question Answering.

Here we use a BERT model fine-tuned on a SQuaD 2.0 Dataset which contains 100,000+ question-answer pairs on 500+ articles combined with over 50,000 new, unanswerable questions.

## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing 0.4 installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).
3. Skip [tag resolution](https://knative.dev/docs/serving/tag-resolution/) for `nvcr.io` which requires auth to resolve triton inference server image digest
```bash
kubectl patch cm config-deployment --patch '{"data":{"registriesSkippingTagResolving":"nvcr.io"}}' -n knative-serving
```
4. Increase progress deadline since pulling triton image and big bert model may longer than default timeout for 120s, this setting requires knative 0.15.0+
```bash
kubectl patch cm config-deployment --patch '{"data":{"progressDeadline": "600s"}}' -n knative-serving
```
## Extend KFServer and Implement pre/postprocess and predict

- The preprocess converts the paragraph and the question to BERT input with the help of the tokenizer
- The predict calls the Triton inference server PYTHON API to communicate with the inference server with HTTP
- The postprocess converts raw prediction to the answer with the probability
```python
class BertTransformer(kfserving.KFModel):
    def __init__(self, name: str, predictor_host: str):
        super().__init__(name)
        self.short_paragraph_text = "The Apollo program was the third United States human spaceflight program. First conceived as a three-man spacecraft to follow the one-man Project Mercury which put the first Americans in space, Apollo was dedicated to President John F. Kennedy's national goal of landing a man on the Moon. The first manned flight of Apollo was in 1968. Apollo ran from 1961 to 1972 followed by the Apollo-Soyuz Test Project a joint Earth orbit mission with the Soviet Union in 1975."

        self.predictor_host = predictor_host
        self.tokenizer = tokenization.FullTokenizer(vocab_file="/mnt/models/vocab.txt", do_lower_case=True)
        self.model_name = "bert_tf_v2_large_fp16_128_v2"
        self.model_version = -1
        self.protocol = ProtocolType.from_str('http')

    def preprocess(self, inputs: Dict) -> Dict:
        self.doc_tokens = data_processing.convert_doc_tokens(self.short_paragraph_text)
        self.features = data_processing.convert_examples_to_features(self.doc_tokens, inputs["instances"][0], self.tokenizer, 128, 128, 64)
        return self.features

    def predict(self, features: Dict) -> Dict:
        if not self.infer_ctx:
            self.infer_ctx = InferContext(self.predictor_host, self.protocol, self.model_name, self.model_version, http_headers='', verbose=True)

        batch_size = 1
        unique_ids = np.int32([1])
        segment_ids = features["segment_ids"]
        input_ids = features["input_ids"]
        input_mask = features["input_mask"]
        result = self.infer_ctx.run({'unique_ids': (unique_ids,),
                                     'segment_ids': (segment_ids,),
                                     'input_ids': (input_ids,),
                                     'input_mask': (input_mask,)},
                                    {'end_logits': InferContext.ResultFormat.RAW,
                                     'start_logits': InferContext.ResultFormat.RAW}, batch_size)
        return result

    def postprocess(self, result: Dict) -> Dict:
        end_logits = result['end_logits'][0]
        start_logits = result['start_logits'][0]
        n_best_size = 20

        # The maximum length of an answer that can be generated. This is needed
        #  because the start and end predictions are not conditioned on one another
        max_answer_length = 30

        (prediction, nbest_json, scores_diff_json) = \
           data_processing.get_predictions(self.doc_tokens, self.features, start_logits, end_logits, n_best_size, max_answer_length)
        return {"predictions": prediction, "prob": nbest_json[0]['probability'] * 100.0}
```

Build the KFServing Transformer image with above code
```bash
cd bert_tokenizer
docker build -t $USER/bert_transformer:latest . --rm
```
Or you can use the prebuild image `gcr.io/kubeflow-ci/kfserving/bert-transformer:latest`

## Create the InferenceService
Add above custom KFServing Transformer image and Triton Predictor to the `InferenceService` spec
```yaml
apiVersion: "serving.kubeflow.org/v1alpha2"
kind: "InferenceService"
metadata:
  name: "bert-large"
spec:
  default:
    transformer:
      custom:
        container:
          name: kfserving-container
          image: gcr.io/kubeflow-ci/kfserving/bert-transformer:latest
          resources:
            limits:
              cpu: "1"
              memory: 1Gi
            requests:
              cpu: "1"
              memory: 1Gi
          command:
            - "python"
            - "-m"
            - "bert_transformer"
          env:
            - name: STORAGE_URI
              value: "gs://kfserving-samples/models/triton/bert-transformer"
    predictor:
      triton:
        runtimeVersion: 20.03-py3 # Higher triton version does not work with KFS 0.4
        resources:
          limits:
            cpu: "1"
            memory: 16Gi
          requests:
            cpu: "1"
            memory: 16Gi
        storageUri: "gs://kfserving-samples/models/triton/bert"
```

Apply the inference service yaml.
```
kubectl apply -f bert.yaml 
```

Expected Output
```
inferenceservice.serving.kubeflow.org/bert-large created
```
## Check the InferenceService
```
kubectl get inferenceservice bert-large
NAME         URL                                                          READY   DEFAULT TRAFFIC   CANARY TRAFFIC   AGE
bert-large   http://bert-large.default.example.com/v1/models/bert-large   True    100                                7m15s
```
you will see both transformer and predictor services are created and in ready state
```
kubectl get revision -l serving.kubeflow.org/inferenceservice=bert-large
NAME                                   CONFIG NAME                      K8S SERVICE NAME                       GENERATION   READY   REASON
bert-large-predictor-default-2gh6p     bert-large-predictor-default     bert-large-predictor-default-2gh6p     1            True    
bert-large-transformer-default-pcztn   bert-large-transformer-default   bert-large-transformer-default-pcztn   1            True 
```
## Run a Prediction
The first step is to [determine the ingress IP and ports](../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

Send a question request with following input
```json
{
  "instances": [
    "What President is credited with the original notion of putting Americans in space?" 
  ]
}
```

```bash
MODEL_NAME=bert-large
INPUT_PATH=@./input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservices bert-large -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" -d $INPUT_PATH http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict
```

Expected output
```
{"predictions": "John F. Kennedy", "prob": 77.91848979818604}
```


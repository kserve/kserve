
# QA Inference with BERT model using Triton Inference Server
Bidirectional Embedding Representations from Transformers (BERT), is a method of pre-training language representations which obtains state-of-the-art results on a wide array of Natural Language Processing (NLP) tasks.

This example demonstrates
- Inference on Question Answering (QA) task with BERT Base/Large model
- The use of fine-tuned NVIDIA BERT models
- Deploy Transformer for preprocess using BERT tokenizer
- Deploy BERT model on Triton Inference Server
- Inference with V2 KServe protocol

We can run inference on a fine-tuned BERT model for tasks like Question Answering.

Here we use a BERT model fine-tuned on a SQuaD 2.0 Dataset which contains 100,000+ question-answer pairs on 500+ articles combined with over 50,000 new, unanswerable questions.

## Setup
1. Your ~/.kube/config should point to a cluster with [KServe 0.5 installed](https://github.com/kserve/kserve#installation).
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

- The `preprocess` handler converts the paragraph and the question to BERT input using BERT tokenizer
- The `predict` handler calls `Triton Inference Server` using PYTHON REST API  
- The `postprocess` handler converts raw prediction to the answer with the probability
```python
class BertTransformer(kserve.Model):
    def __init__(self, name: str, predictor_host: str):
        super().__init__(name)
        self.short_paragraph_text = "The Apollo program was the third United States human spaceflight program. First conceived as a three-man spacecraft to follow the one-man Project Mercury which put the first Americans in space, Apollo was dedicated to President John F. Kennedy's national goal of landing a man on the Moon. The first manned flight of Apollo was in 1968. Apollo ran from 1961 to 1972 followed by the Apollo-Soyuz Test Project a joint Earth orbit mission with the Soviet Union in 1975."

        self.predictor_host = predictor_host
        self.tokenizer = tokenization.FullTokenizer(vocab_file="/mnt/models/vocab.txt", do_lower_case=True)
        self.model_name = "bert_tf_v2_large_fp16_128_v2"
        self.triton_client = None

    def preprocess(self, inputs: Dict) -> Dict:
        self.doc_tokens = data_processing.convert_doc_tokens(self.short_paragraph_text)
        self.features = data_processing.convert_examples_to_features(self.doc_tokens, inputs["instances"][0], self.tokenizer, 128, 128, 64)
        return self.features

    def predict(self, features: Dict) -> Dict:
        if not self.triton_client:
            self.triton_client = httpclient.InferenceServerClient(
                url=self.predictor_host, verbose=True)
     
        unique_ids = np.zeros([1,1], dtype=np.int32)
        segment_ids = features["segment_ids"].reshape(1,128)
        input_ids = features["input_ids"].reshape(1,128)
        input_mask = features["input_mask"].reshape(1,128)
        
        inputs = []
        inputs.append(httpclient.InferInput('unique_ids', [1,1], "INT32"))
        inputs.append(httpclient.InferInput('segment_ids', [1, 128], "INT32"))
        inputs.append(httpclient.InferInput('input_ids', [1, 128], "INT32"))
        inputs.append(httpclient.InferInput('input_mask', [1, 128], "INT32"))
        inputs[0].set_data_from_numpy(unique_ids)
        inputs[1].set_data_from_numpy(segment_ids)
        inputs[2].set_data_from_numpy(input_ids)
        inputs[3].set_data_from_numpy(input_mask)
        
        outputs = []
        outputs.append(httpclient.InferRequestedOutput('start_logits', binary_data=False))
        outputs.append(httpclient.InferRequestedOutput('end_logits', binary_data=False))
        result = self.triton_client.infer(self.model_name, inputs, outputs=outputs)
        return result.get_response()
    
    def postprocess(self, result: Dict) -> Dict:
        end_logits = result['outputs'][0]['data']
        start_logits = result['outputs'][1]['data']
        n_best_size = 20

        # The maximum length of an answer that can be generated. This is needed 
        #  because the start and end predictions are not conditioned on one another
        max_answer_length = 30

        (prediction, nbest_json, scores_diff_json) = \
           data_processing.get_predictions(self.doc_tokens, self.features, start_logits, end_logits, n_best_size, max_answer_length)
        return {"predictions": prediction, "prob": nbest_json[0]['probability'] * 100.0}
```

Build the KServe Transformer image with above code
```bash
cd bert_tokenizer_v2
docker build -t $USER/bert_transformer-v2:latest . --rm
```
Or you can use the prebuild image `kfserving/bert-transformer-v2:latest`

## Create the InferenceService
Add above custom KServe Transformer image and Triton Predictor to the `InferenceService` spec
```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "bert-v2"
spec:
  transformer:
    containers:
      - name: kfserving-container      
        image: kfserving/bert-transformer-v2:latest
        command:
          - "python"
          - "-m"
          - "bert_transformer_v2"
        env:
          - name: STORAGE_URI
            value: "gs://kfserving-samples/models/triton/bert-transformer"
  predictor:
    triton:
      runtimeVersion: 20.10-py3
      resources:
        limits:
          cpu: "1"
          memory: 8Gi
        requests:
          cpu: "1"
          memory: 8Gi
      storageUri: "gs://kfserving-examples/models/triton/bert"
```

Apply the `InferenceService` yaml.
```
kubectl apply -f bert_v1beta1.yaml 
```

Expected Output
```
inferenceservice.serving.kserve.io/bert-v2 created
```
## Check the InferenceService
```
kubectl get inferenceservice bert-v2
NAME      URL                                           READY   AGE
bert-v2   http://bert-v2.default.35.229.120.99.xip.io   True    71s
```
you will see both transformer and predictor are created and in ready state
```
kubectl get revision -l serving.kserve.io/inferenceservice=bert-v2
NAME                                CONFIG NAME                   K8S SERVICE NAME                    GENERATION   READY   REASON
bert-v2-predictor-default-plhgs     bert-v2-predictor-default     bert-v2-predictor-default-plhgs     1            True    
bert-v2-transformer-default-sd6nc   bert-v2-transformer-default   bert-v2-transformer-default-sd6nc   1            True  
```
## Run a Prediction
The first step is to [determine the ingress IP and ports](../../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

Send a question request with following input, the transformer expects sending a list of `instances` or `inputs` and `preprocess` then converts
the inputs to expected tensor sending to `Triton Inference Server`.
```json
{
  "instances": [
    "What President is credited with the original notion of putting Americans in space?" 
  ]
}
```

```bash
MODEL_NAME=bert-v2
INPUT_PATH=@./input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservices bert-v2 -o jsonpath='{.status.url}' | cut -d "/" -f 3)

curl -v -H "Host: ${SERVICE_HOSTNAME}" -d $INPUT_PATH http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/$MODEL_NAME:predict
```

Expected output
```
{"predictions": "John F. Kennedy", "prob": 77.91848979818604}
```


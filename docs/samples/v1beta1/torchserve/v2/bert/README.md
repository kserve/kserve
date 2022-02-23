# TorchServe example with Huggingface bert model

In this example we will show how to serve [Huggingface Transformers with TorchServe](https://github.com/pytorch/serve/tree/master/examples/Huggingface_Transformers)
on KServe.

## Model archive file creation

Clone [pytorch/serve](https://github.com/pytorch/serve) repository,
navigate to `examples/Huggingface_Transformers` and follow the steps for creating the MAR file including serialized model and other dependent files.
TorchServe supports both eager model and torchscript and here we save as the pretrained model. 
Download the preprocess script from [here](./sequence_classification/Transformer_handler_generalized_v2.py)
 
```bash
torch-model-archiver --model-name BERTSeqClassification --version 1.0 \
--serialized-file Transformer_model/pytorch_model.bin \
--handler ./Transformer_kserve_handler.py \
--extra-files "Transformer_model/config.json,./setup_config.json,./Seq_classification_artifacts/index_to_name.json,./Transformer_handler_generalized.py"
```

## Create the InferenceService

Apply the CRD

```bash
kubectl apply -f bert.yaml
```

Expected Output

```bash
$inferenceservice.serving.kserve.io/torchserve-bert-v2 created
```

For deploying it in gpu use

```bash
kubectl apply -f bert_gpu.yaml
```

## Run a prediction

The first step is to [determine the ingress IP and ports](../../../../../README.md#determine-the-ingress-ip-and-ports) and set `INGRESS_HOST` and `INGRESS_PORT`

Setting variables

```
MODEL_NAME=BERTSeqClassification
SERVICE_HOSTNAME=$(kubectl get inferenceservice torchserve-bert-v2 -o jsonpath='{.status.url}' | cut -d "/" -f 3)
```

```bash
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v2/models/BERTSeqClassification/infer -d @./sequence_classification/bytes/bert_v2.json
```

Expected Output

```bash
{"id": "d3b15cad-50a2-4eaf-80ce-8b0a428bd298", "model_name": "BERTSeqClassification", "model_version": "1.0", "outputs": [{"name": "predict", "shape": [], "datatype": "BYTES", "data": ["Not Accepted"]}]}
```

For tensor input

```
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v2/models/BERTSeqClassification/infer -d @./sequence_classification/tensor/bert_v2.json
```

Expected output
```bash
{"id": "33abc661-7265-42fc-b7d9-44e5f79a7a67", "model_name": "BERTSeqClassification", "model_version": "1.0", "outputs": [{"name": "predict", "shape": [], "datatype": "BYTES", "data": ["Not Accepted"]}]}
```

## Captum Explanations
In order to understand the word importances and attributions when we make an explanation Request, we use Captum Insights for the HuggingFace Transformers pre-trained model.
```bash
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v2/models/BERTSeqClassification/explain -d ./sequence_classification/bytes/bert_v2.json

```

Expected output

```bash
{"id": "d3b15cad-50a2-4eaf-80ce-8b0a428bd298", "model_name": "BERTSeqClassification", "model_version": "1.0", "outputs": [{"name": "explain", "shape": [], "datatype": "BYTES", "data": [{"words": ["[CLS]", "bloomberg", "has", "decided", "to", "publish", "a", "new", "report", "on", "the", "global", "economy", ".", "[SEP]"], "importances": [0.0, -0.43571255624310423, -0.11062097534384648, 0.11323803203829622, 0.05438679692935377, -0.11364841625009202, 0.15214504085858935, -0.0013061684457894148, 0.05712844103997178, -0.02296408323390218, 0.1937543236757826, -0.12138265438655091, 0.20713335609474381, -0.8044260616647264, 0.0], "delta": -0.019047775223331675}]}]}
```



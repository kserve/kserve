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
--handler ./Transformer_handler_generalized_v2.py \
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
{"id": "fa0f4a16-24be-4e82-822b-7ce21cff1016", "model_name": "bert_test", "model_version": "1", "outputs": [{"name": "explain", "shape": [], "datatype": "BYTES", "data": [{"words": ["[CLS]", "[unused65]", "[unused103]", "[unused106]", "[unused106]", "[unused104]", "[unused97]", "[CLS]", "[unused109]", "[MASK]", "[unused31]", "[unused99]", "[unused96]", "[unused110]", "[unused31]", "[unused109]", "[CLS]", "[unused107]", "[unused106]", "[unused109]", "[unused111]", "[CLS]", "[UNK]", "[unused31]", "[unused106]", "[unused105]", "[unused31]", "[unused111]", "[unused99]", "[CLS]", "[unused31]", "[CLS]", "[unused98]", "[unused106]", "[unused105]", "[unused106]", "[unused104]", "[unused116]", "[SEP]"], "importances": [-0.5779647849140105, 0.017149979253482668, 0.02520071691362777, 0.10127131153071542, 0.11157838511306105, 0.10381272285539787, 0.11320268752645515, -0.18749022141160918, 0.09715615163453448, -0.23825046155397892, 0.07830538237901745, 0.052386644292540425, 0.06916019909789417, 0.0489200370513321, 0.06125091233381835, 0.10910945892939933, -0.20546550665577787, 0.03657186541090417, 0.03873832137700618, 0.07419369954398138, 0.03729456936648431, -0.2576498669080684, -0.14288095272100626, 0.04121622648595307, 0.06318685560063542, 0.012703899463284731, 0.03181142622138418, 0.03485410565174061, 0.049515843720263124, -0.18949917348232484, 0.03956454265824759, -0.2113086240763918, 0.028525852720988263, 0.04318882441540453, 0.018988349248547743, 0.07123601660669067, 0.061472429104257806, 0.023899392506903514, 0.49172702017614983], "delta": 0.9374768388549066}]}]}
```



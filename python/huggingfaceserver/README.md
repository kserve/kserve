# Huggingface Serving Runtime

The Huggingface serving runtime implements a runtime that can serve huggingface transformer based model out of the box.
The preprocess and post-process handlers are implemented based on different ML tasks, for example text classification,
token-classification, text-generation, text2text generation. Based on the performance requirement, you can choose to perform
the inference on a more optimized inference engine like triton inference server and vLLM for text generation.


## Run Huggingface Server Locally

```bash
python -m huggingfaceserver --model_id=bert-base-uncased --model_name=bert

INFO:kserve:successfully loaded tokenizer for task: 5
Some weights of the model checkpoint at bert-base-uncased were not used when initializing BertForMaskedLM: ['cls.seq_relationship.weight', 'bert.pooler.dense.bias', 'cls.seq_relationship.bias', 'bert.pooler.dense.weight']
- This IS expected if you are initializing BertForMaskedLM from the checkpoint of a model trained on another task or with another architecture (e.g. initializing a BertForSequenceClassification model from a BertForPreTraining model).
- This IS NOT expected if you are initializing BertForMaskedLM from the checkpoint of a model that you expect to be exactly identical (initializing a BertForSequenceClassification model from a BertForSequenceClassification model).
INFO:kserve:successfully loaded huggingface model from path bert-base-uncased
INFO:kserve:Registering model: model
INFO:kserve:Setting max asyncio worker threads as 16
INFO:kserve:Starting uvicorn with 1 workers
2024-01-08 06:32:08.801 uvicorn.error INFO:     Started server process [75012]
2024-01-08 06:32:08.801 uvicorn.error INFO:     Waiting for application startup.
2024-01-08 06:32:08.804 75012 kserve INFO [start():62] Starting gRPC server on [::]:8081
2024-01-08 06:32:08.804 uvicorn.error INFO:     Application startup complete.
2024-01-08 06:32:08.805 uvicorn.error INFO:     Uvicorn running on http://0.0.0.0:8080 (Press CTRL+C to quit)
```

If you want to change the datatype, you can use --dtype flag. Available choices are float16, bfloat16, float32, auto, float, half.
auto uses float16 if GPU is available and uses float32 otherwise to ensure consistency between vLLM and HuggingFace backends. encoder model defaults to float32. float is shorthand for float32. half is float16. The rest are as the name reads.
```bash
python -m huggingfaceserver --model_id=bert-base-uncased --model_name=bert --dtype=float16
```

Perform the inference

```bash
curl -H "content-type:application/json" -v localhost:8080/v1/models/bert:predict -d '{"instances": ["The capital of france is [MASK]."] }'

{"predictions":["paris"]}
```

## Deploy Huggingface Server on KServe

> 1. `SAFETENSORS_FAST_GPU` is set by default to improve the model loading performance.
> 2. `HF_HUB_DISABLE_TELEMETRY` is set by default to disable the telemetry.

1. Serve the huggingface model using KServe python runtime for both preprocess(tokenization)/postprocess and inference.
```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: huggingface-bert
spec:
  predictor:
    model:
      modelFormat:
        name: huggingface
      args:
      - --model_name=bert
      - --model_id=bert-base-uncased
      - --tensor_input_names=input_ids
      resources:
        limits:
          cpu: "1"
          memory: 2Gi
          nvidia.com/gpu: "1"
        requests:
          cpu: 100m
          memory: 2Gi
```

2. Serve the huggingface model using triton inference runtime and KServe transformer for the preprocess(tokenization) and postprocess.
```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: huggingface-triton
spec:
  predictor:
    model:
      args:
      - --log-verbose=1
      modelFormat:
        name: triton
      protocolVersion: v2
      resources:
        limits:
          cpu: "1"
          memory: 8Gi
          nvidia.com/gpu: "1"
        requests:
          cpu: "1"
          memory: 8Gi
      runtimeVersion: 23.10-py3
      storageUri: gs://kfserving-examples/models/triton/huggingface/model_repository
  transformer:
    containers:
    - args:
      - --model_name=bert
      - --model_id=bert-base-uncased
      - --predictor_protocol=v2
      - --tensor_input_names=input_ids
      image: kserve/huggingfaceserver:latest
      name: kserve-container
      resources:
        limits:
          cpu: "1"
          memory: 2Gi
        requests:
          cpu: 100m
          memory: 2Gi
```
3. Serve the huggingface model using vllm runtime. Note - Model need to be supported by vllm otherwise KServe python runtime will be used as a failsafe.
vllm supported models - https://docs.vllm.ai/en/latest/models/supported_models.html 
```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: huggingface-llama2
spec:
  predictor:
    model:
      modelFormat:
        name: huggingface
      args:
      - --model_name=llama2
      - --model_id=meta-llama/Llama-2-7b-chat-hf
      resources:
        limits:
          cpu: "6"
          memory: 24Gi
          nvidia.com/gpu: "1"
        requests:
          cpu: "6"
          memory: 24Gi
          nvidia.com/gpu: "1"

```

If vllm needs to be disabled include the flag `--backend=huggingface` in the container args. In this case the KServe python runtime will be used.

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: huggingface-llama2
spec:
  predictor:
    model:
      modelFormat:
        name: huggingface
      args:
      - --model_name=llama2
      - --model_id=meta-llama/Llama-2-7b-chat-hf
      - --backend=huggingface
      resources:
        limits:
          cpu: "6"
          memory: 24Gi
          nvidia.com/gpu: "1"
        requests:
          cpu: "6"
          memory: 24Gi
          nvidia.com/gpu: "1"
```

Perform the inference for vllm specific runtime

vllm runtime deployments only support OpenAI `v1/completions` and `v1/chat/completions` endpoints for inference.

Sample OpenAI Completions request
```bash
curl -H "content-type:application/json" -v localhost:8080/openai/v1/completions -d '{"model": "gpt2", "prompt": "<prompt>", "stream":false, "max_tokens": 30 }'

{"id":"cmpl-7c654258ab4d4f18b31f47b553439d96","choices":[{"finish_reason":"length","index":0,"logprobs":null,"text":"<generated_text>"}],"created":1715353182,"model":"gpt2","system_fingerprint":null,"object":"text_completion","usage":{"completion_tokens":26,"prompt_tokens":4,"total_tokens":30}}
```

Sample OpenAI Chat request
```bash
curl -H "content-type:application/json" -v localhost:8080/openai/v1/chat/completions -d '{"model": "gpt2", "messages": [{"role": "user","content": "<message>"}], "stream":false }'

{"id":"cmpl-87ee252062934e2f8f918dce011e8484","choices":[{"finish_reason":"length","index":0,"message":{"content":"<generated_response>","tool_calls":null,"role":"assistant","function_call":null},"logprobs":null}],"created":1715353461,"model":"gpt2","system_fingerprint":null,"object":"chat.completion","usage":{"completion_tokens":30,"prompt_tokens":3,"total_tokens":33}}
```

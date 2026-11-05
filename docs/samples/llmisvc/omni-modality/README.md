# Omni Modality (TTS/TTI) Deployment Example

Deploys a vLLM-Omni model behind the llm-d router for non-text endpoints:
`POST /v1/audio/speech` (text-to-speech), `POST /v1/audio/transcriptions`
(speech-to-text), and `POST /v1/images/generations` (text-to-image).

## Overview

Setting `spec.runtime: kserve-llm-omni` layers the vLLM-Omni overlay on the
base vLLM workload template (swapping in the vLLM-Omni image and appending
`--omni` to the launch args) and selects a load-only EPP profile
(`active-request-scorer` + `max-score-picker`). Diffusion and speech
synthesis expose no prefix-cache signal, so the text prefix-cache chain does
not apply; the router prefers the least-busy endpoint instead.

## Limitations

- Single-node deployments only: disaggregated prefill/decode is not supported
  for omni-modality models.
- Base models only: LoRA adapters are not supported (the load-only EPP profile
  carries no adapter affinity, and vLLM-Omni LoRA support is still maturing).
- NVIDIA GPUs are required; the vLLM-Omni image is CUDA-based.

## Prerequisites

- Kubernetes cluster with NVIDIA GPU nodes
- Model weights accessible via HuggingFace or PVC
- A HuggingFace token secret for gated models

## Examples

### Text-to-Speech ([llm-inference-service-omni-tts-gpu.yaml](llm-inference-service-omni-tts-gpu.yaml))

Qwen3-TTS voice synthesis over the OpenAI-compatible speech endpoint.

**Deployment:**

```bash
kubectl apply -f llm-inference-service-omni-tts-gpu.yaml
```

**Verification:**

The controller publishes the service's path-based URL in `status.url`; no
gateway IP or port-forward is needed.

```bash
BASE_URL=$(kubectl get llminferenceservice omni-tts-single \
  -o jsonpath='{.status.url}')

# Text-to-speech
curl -s -X POST "${BASE_URL}/v1/audio/speech" \
    -H 'Content-Type: application/json' \
    -o speech.wav \
    -d '{
        "model": "Qwen/Qwen3-TTS-12Hz-1.7B-CustomVoice",
        "input": "Hello from KServe.",
        "voice": "vivian",
        "response_format": "wav"
    }'
head -c 4 speech.wav # expect: RIFF
```

The same service exposes the sibling modality endpoints:

```bash
# Speech-to-text (swap in a transcription-capable model when deploying)
curl -s -X POST "${BASE_URL}/v1/audio/transcriptions" \
    -F model=Qwen/Qwen3-TTS-12Hz-1.7B-CustomVoice \
    -F file=@speech.wav

# Text-to-image (the response carries base64-encoded PNG data)
curl -s -X POST "${BASE_URL}/v1/images/generations" \
    -H 'Content-Type: application/json' \
    -d '{
        "model": "Qwen/Qwen-Image",
        "prompt": "A watercolor painting of the Golden Gate Bridge",
        "size": "1024x1024"
    }' | jq -r '.data[0].b64_json' | base64 -d > image.png
```

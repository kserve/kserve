# Omni Modality (TTS/TTI) Deployment Example

Deploys a vLLM-Omni model behind the llm-d router for non-text endpoints:
`POST /v1/audio/speech` (text-to-speech), `POST /v1/audio/transcriptions`
(speech-to-text), and `POST /v1/images/generations` (text-to-image).

## Overview

Setting `spec.runtime: kserve-llm-omni` selects the vLLM-Omni workload
template (`vllm serve --omni`) and a load-only EPP profile
(`active-request-scorer` + `max-score-picker`). Diffusion and speech
synthesis expose no prefix-cache signal, so the text prefix-cache chain does
not apply; the router prefers the least-busy endpoint instead.

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

```bash
curl -s -X POST http://$GATEWAY_IP/v1/audio/speech \
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

# Prefix-affine prefill/decode rollouts with Istio

One KServe LLMInferenceService owns both P/D roles in a DisaggregatedSet.
The stable Istio HTTPRoute points to one InferencePool. During a workload
rollout, the EPP screener picks a revision based on the conversation prefix
and available capacity, then chooses both prefill and decode from that revision.

## Prerequisites

- KServe with the DisaggregatedSet feature gate enabled and the LWS controller
  and DisaggregatedSet CRD installed.
- Istio with the [Gateway API Inference Extension](https://istio.io/latest/docs/tasks/traffic-management/ingress/gateway-api-inference-extension/)
  enabled, and the Gateway API and InferencePool CRDs installed.
- A CPU kind node can run the four inference-sim pods. The pinned demonstration
  EPP image includes [prefix-keyed revision affinity](https://github.com/llm-d/llm-d-router/pull/3456).

The DestinationRule uses TLS with the fixture's self-signed scheduler
certificate for this local test.

## Run

```sh
kubectl apply -f istio.yaml
kubectl apply -f llmisvc.yaml
kubectl -n llmisvc-prefix-demo wait llmisvc/qwen-pd-prefix --for=condition=Ready --timeout=180s
kubectl -n llmisvc-prefix-demo port-forward svc/prefix-rollout-istio 8080:80
```

Send requests to the same prefix before and during the rollout.

```sh
curl -si http://localhost:8080/llmisvc-prefix-demo/qwen-pd-prefix/v1/chat/completions \
  -H 'content-type: application/json' \
  -d '{"model":"Qwen/Qwen3-0.6B","messages":[{"role":"user","content":"Summarize how this model is served."}],"max_tokens":20}'
```

Update both roles in one patch to create a single DisaggregatedSet revision.

```sh
kubectl -n llmisvc-prefix-demo patch llmisvc qwen-pd-prefix --type=json -p '[
  {"op":"replace","path":"/spec/template/containers/0/env/0/value","value":"r1"},
  {"op":"replace","path":"/spec/prefill/template/containers/0/env/0/value","value":"r1"}
]'
kubectl -n llmisvc-prefix-demo get pods -L disaggregatedset.x-k8s.io/revision,disaggregatedset.x-k8s.io/role -w
```

Compare the `x-llm-d-disagg-revision` response header and the decode pod's
`x-inference-pod` header across turns. The example demonstrates workload
revision selection with one EPP and simulated inference. To verify that every
actual prefill/decode pair shares a revision, collect decode-sidecar target
logs across both revisions.

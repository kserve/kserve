# Dynamic LoRA Adapter Management Demo

This walkthrough demonstrates dynamic LoRA adapter lifecycle management
without restarting the model server. It shows:

1. The HTTPRoute only exposes inference endpoints — adapter management is blocked from external traffic
2. In-cluster Jobs load adapters onto specific pods (uneven distribution)
3. The LoRA affinity scorer routes requests to the pod that has the requested adapter

## Prerequisites

- The LLMInferenceService, HTTPRoute, and RBAC resources are deployed:
  ```bash
  kubectl apply -f rbac.yaml
  kubectl apply -f llm-inference-service-lora.yaml
  kubectl apply -f httproute.yaml
  ```
- The gateway hostname is resolvable:
  ```bash
  export GATEWAY_HOST=$(kubectl get gateway openshift-ai-inference -n openshift-ingress -o jsonpath='{.status.addresses[0].value}')
  ```

## Step 1: Verify adapter management is blocked through the gateway

The HTTPRoute only allows `/v1/chat/completions`, `/v1/completions`,
`/v1/responses`, and `/v1/models`. Attempting to load an adapter through
the gateway should fail — there is no route for `/v1/load_lora_adapter`.

```bash
# Try to load a LoRA adapter through the external gateway — this should be rejected
curl -s -w "\nHTTP Status: %{http_code}\n" \
  -X POST "https://${GATEWAY_HOST}/greg/qwen2-lora/v1/load_lora_adapter" \
  -H "Content-Type: application/json" \
  -d '{
    "lora_name": "k8s-lora",
    "lora_path": "cimendev/kubernetes-qa-qwen2.5-7b-lora"
  }'
```

Expected: HTTP 404 — the gateway has no matching route rule.

## Step 2: Confirm no adapters are loaded yet

```bash
# List loaded models — only the base model should appear
curl -s "https://${GATEWAY_HOST}/greg/qwen2-lora/v1/models" | jq .
```

Expected: only `Qwen/Qwen2.5-7B-Instruct` in the model list.

## Step 3: Load adapters unevenly across replicas

We deliberately load different adapters on different pods. The LoRA
affinity scorer is configured at weight **20.0** — high enough to guarantee
strict affinity routing regardless of load on the pods.

**Why 20.0?** The scorer assigns 1.0 when the adapter is active on a pod
and 0.8 when the pod has capacity but doesn't have it loaded. The gap is
0.2, so: `0.2 × 20.0 = 4.0`. The maximum combined score from all other
scorers is 3.5 (queue 1.0 + kv-cache 1.0 + prefix-cache 1.5), so the
pod with the adapter **always wins** — even if it's under heavy load and
the other pod is idle.

```bash
# Find the vLLM pod names
kubectl get pods -l app.kubernetes.io/name=qwen2-lora,app.kubernetes.io/part-of=llminferenceservice \
  -o custom-columns=NAME:.metadata.name --no-headers
```

Pick two pod names from the output and set them:

```bash
export POD_1=<first-pod-name>
export POD_2=<second-pod-name>
```

### Load `k8s-lora` on Pod 1 only

```bash
cat <<EOF | kubectl apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: load-k8s-lora-pod1
  labels:
    app.kubernetes.io/part-of: lora-demo
spec:
  backoffLimit: 3
  template:
    spec:
      serviceAccountName: lora-manager
      restartPolicy: Never
      containers:
      - name: loader
        image: REPLACE_ME_LORA_MANAGER_IMAGE
        args: ["load"]
        env:
          - name: POD_NAME
            value: "${POD_1}"
          - name: ADAPTER_NAME
            value: "k8s-lora"
          - name: ADAPTER_SOURCE
            value: "cimendev/kubernetes-qa-qwen2.5-7b-lora"
EOF

kubectl wait --for=condition=complete job/load-k8s-lora-pod1 --timeout=120s
kubectl logs job/load-k8s-lora-pod1
```

### Load `finance-lora` on Pod 2 only

```bash
cat <<EOF | kubectl apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: load-finance-lora-pod2
  labels:
    app.kubernetes.io/part-of: lora-demo
spec:
  backoffLimit: 3
  template:
    spec:
      serviceAccountName: lora-manager
      restartPolicy: Never
      containers:
      - name: loader
        image: REPLACE_ME_LORA_MANAGER_IMAGE
        args: ["load"]
        env:
          - name: POD_NAME
            value: "${POD_2}"
          - name: ADAPTER_NAME
            value: "finance-lora"
          - name: ADAPTER_SOURCE
            value: "Max1690/qwen2.5-7b-finance-lora"
EOF

kubectl wait --for=condition=complete job/load-finance-lora-pod2 --timeout=120s
kubectl logs job/load-finance-lora-pod2
```

## Step 4: Verify adapters are loaded

```bash
# The models endpoint should now list the base model + both adapters
curl -s "https://${GATEWAY_HOST}/greg/qwen2-lora/v1/models" | jq '.data[].id'
```

Expected output:
```
"Qwen/Qwen2.5-7B-Instruct"
"k8s-lora"
"finance-lora"
```

## Step 5: LoRA affinity routing in action

Since `k8s-lora` is only on Pod 1 and `finance-lora` is only on Pod 2,
the `lora-affinity-scorer` (weight 20.0) guarantees each request is
routed to the pod that has the adapter — no matter how loaded that pod is.

For example, a `k8s-lora` request scores:
```
Pod 1 (has k8s-lora):  1.0 × 20.0 = 20.0  +  other scorers (up to 3.5) = 23.5
Pod 2 (has capacity):  0.8 × 20.0 = 16.0  +  other scorers (up to 3.5) = 19.5
→ Pod 1 always wins
```

```bash
# Request using the k8s-lora adapter — should be routed to Pod 1
curl -s "https://${GATEWAY_HOST}/greg/qwen2-lora/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "k8s-lora",
    "messages": [{"role": "user", "content": "What is a Kubernetes Pod?"}],
    "max_tokens": 100
  }' | jq .
```

```bash
# Request using the finance-lora adapter — should be routed to Pod 2
curl -s "https://${GATEWAY_HOST}/greg/qwen2-lora/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "finance-lora",
    "messages": [{"role": "user", "content": "Explain the concept of dollar-cost averaging."}],
    "max_tokens": 100
  }' | jq .
```

```bash
# Request using the base model — can go to either pod
curl -s "https://${GATEWAY_HOST}/greg/qwen2-lora/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen/Qwen2.5-7B-Instruct",
    "messages": [{"role": "user", "content": "Hello, what model are you?"}],
    "max_tokens": 100
  }' | jq .
```

## Step 6: Clean up adapter Jobs

```bash
kubectl delete job -l app.kubernetes.io/part-of=lora-demo
```

## What just happened?

| Aspect | How it works |
|---|---|
| **Security** | The HTTPRoute only exposes inference endpoints. Adapter management (`/v1/load_lora_adapter`) is unreachable from outside the cluster. |
| **Per-replica control** | Jobs target individual pods by name (`POD_NAME`), allowing different adapters on different replicas. |
| **Strict affinity routing** | The `lora-affinity-scorer` at weight 20.0 guarantees requests are routed to the pod with the adapter loaded — the 4.0-point affinity advantage always exceeds the 3.5 max from all other scorers combined. |
| **No restarts** | Adapters are loaded and unloaded at runtime via the vLLM API. The model server keeps serving throughout. |

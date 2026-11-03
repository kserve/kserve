# Managed DRA Examples

This directory contains example configurations that request specialized devices (GPUs,
TPUs, NICs, etc.) on an `LLMInferenceService` via the **Managed DRA** convenience layer,
without authoring a `ResourceClaimTemplate` by hand.

## Overview

Kubernetes Dynamic Resource Allocation (DRA) lets workloads claim attached devices through
`DeviceClass` and `ResourceClaimTemplate` objects. KServe's Managed DRA is an
**experimental**, intentionally limited-scope convenience that drives those primitives from
a small set of `serving.kserve.io/exp-dra-*` annotations on an `LLMInferenceService`. It
covers the common case (*"give me N devices of class X, optionally filtered by CEL"*) and
is not a replacement for hand-authored claims when more advanced topologies are needed.

When you set `serving.kserve.io/exp-dra-device-class` on an `LLMInferenceService`, the
controller creates a `ResourceClaimTemplate` named `<llmisvc-name>-managed-dra`, injects a
matching `resourceClaims` entry on the pod template, and references it from the target
container. Removing the annotation (or deleting the `LLMInferenceService`) cleans the
template up.

## Prerequisites

- Kubernetes **v1.34+** - DRA is GA in v1.34 and `resource.k8s.io/v1` is enabled by default.
  The controller imports the v1 API directly and does not fall back to the beta versions.
- A `DeviceClass` published by your DRA driver (e.g. `gpu.nvidia.com`)
- The DRA driver's daemonset running and healthy:
  ```bash
  kubectl get deviceclasses
  kubectl get resourceslices
  ```

## Examples

### 1. Minimal - single device, no filters ([llm-inference-service-managed-dra-minimal.yaml](llm-inference-service-managed-dra-minimal.yaml))

The smallest possible Managed DRA config: just `exp-dra-device-class`. Requests one device
of the class and injects it into the first container of the pod template.

**Configuration:**
- Model: Qwen2.5-7B-Instruct, 1 replica
- Devices: 1 of class `gpu.nvidia.com`, no CEL selectors

**Use Case:**
- Quickest way to opt a workload into DRA
- Cluster only exposes one DeviceClass relevant to the workload

**Deployment:**
```bash
kubectl apply -f llm-inference-service-managed-dra-minimal.yaml
```

**YAML snippet:**
```yaml
metadata:
  annotations:
    serving.kserve.io/exp-dra-device-class: gpu.nvidia.com
```

### 2. Full Surface - multi-device, CEL filters, container targeting ([llm-inference-service-managed-dra-multi-device.yaml](llm-inference-service-managed-dra-multi-device.yaml))

Combines all four `exp-dra-*` annotations in one file to showcase the full surface. Each
annotation is independent, use whichever subset matches your workload:

- `exp-dra-device-count: "2"` requests multiple devices (default is `1`).
- `exp-dra-cel-selector` filters eligible devices. Each non-empty line of the YAML `|`
  block scalar becomes one selector, and all selectors must evaluate to true.
- `exp-dra-container-name: main` routes the claim to the named container instead of the
  first one, only relevant when the pod template has multiple containers.

**Configuration:**
- Model: Qwen2.5-7B-Instruct, 1 replica
- Devices: 2 of class `gpu.nvidia.com`, filtered by `type == 'A100'` AND `memory >= 40Gi`
- Containers: `sidecar` (first) and `main`; claim targets `main`

**Use Case:**
- Cluster has heterogeneous GPUs and only a subset is appropriate
- Multi-GPU workloads (tensor parallelism, large models)
- Multi-container pods where the claim must not leak onto the wrong container

**Deployment:**
```bash
kubectl apply -f llm-inference-service-managed-dra-multi-device.yaml
```

**YAML snippet:**
```yaml
metadata:
  annotations:
    serving.kserve.io/exp-dra-device-class: gpu.nvidia.com
    serving.kserve.io/exp-dra-device-count: "2"
    serving.kserve.io/exp-dra-container-name: main
    serving.kserve.io/exp-dra-cel-selector: |
      device.attributes['gpu.nvidia.com']['type'] == 'A100'
      device.capacity['gpu.nvidia.com']['memory'].compareTo(quantity('40Gi')) >= 0
```

The attribute keys above are illustrative, replace them with the keys your driver
publishes (`kubectl get resourceslices -o yaml`). For the CEL syntax see the
[Kubernetes DRA docs](https://kubernetes.io/docs/concepts/scheduling-eviction/dynamic-resource-allocation/).

## How It Works

The four `serving.kserve.io/exp-dra-*` annotations drive a single `DeviceRequest` on the
generated `ResourceClaimTemplate`:

| Annotation             | Required | Default         | Maps to                                    |
|------------------------|----------|-----------------|--------------------------------------------|
| `exp-dra-device-class` | yes      | —               | `request.exactly.deviceClassName`          |
| `exp-dra-device-count` | no       | `1`             | `request.exactly.count` (only set when >1) |
| `exp-dra-cel-selector` | no       | —               | `request.exactly.selectors[].cel.expression` (one per non-empty line) |
| `exp-dra-container-name` | no     | first container | The pod-template container that receives the claim |

The controller injects a pod-level `resourceClaims` entry named `managed-device` and a
matching `resources.claims` reference on the target container. The same claim is injected
into **every** workload pod template on the `LLMInferenceService` - the main `Template`,
`Worker`, `Prefill.Template`, and `Prefill.Worker`, using the same DeviceClass, count, and
selectors for all of them.

The admission webhook validates the four annotations together and rejects the
`LLMInferenceService` if a value is malformed (non-numeric count, non-DNS device class or
container name, etc.).

### Coexisting with hand-authored claims

The controller only ever touches claims named `managed-device`. Any pod-level
`resourceClaims` or container-level `resources.claims` entry with a different name is left
alone, so a hand-authored claim and Managed DRA can coexist on the same workload.

## Verification

```bash
# Confirm the LLMInferenceService is healthy
kubectl get llminferenceservice <name>

# The controller should have created a ResourceClaimTemplate named <name>-managed-dra
kubectl get resourceclaimtemplate <name>-managed-dra -o yaml

# Each pod gets a generated ResourceClaim
kubectl get resourceclaims

# The target container should reference the claim under resources.claims
kubectl get pod <pod> -o jsonpath='{.spec.containers[*].resources.claims}'
```

## When *not* to Use Managed DRA

Managed DRA generates **one** `DeviceRequest`, from **one** `DeviceClass`, applied to
**every** pod template on the `LLMInferenceService`. Author a `ResourceClaimTemplate` (or a
direct `ResourceClaim`) yourself if you need:

- Different claims for different pod templates on the same service, e.g. a prefill-decode
  workload where the prefill pool and the decode pool need different device classes,
  device counts, or CEL filters
- Multiple distinct `DeviceRequest`s in one claim (e.g. GPU + NIC), or `DeviceConstraints`
  across them
- `firstAvailable` subrequest lists for preferred-then-fallback hardware
- A `ResourceClaim` shared across pods (template-generated claims are per-pod)
- Alpha features like `AdminAccess`, device taints/tolerations, or per-request `capacity`

A hand-authored claim named anything other than `managed-device` will coexist with Managed
DRA on the same workload.

## Troubleshooting

### LLMInferenceService rejected by the admission webhook

Read the error message, it names the offending annotation and the failing rule (invalid
count, non-DNS device class or container name, missing required `exp-dra-device-class`,
etc.) and adjust the annotation accordingly.

### Pods stay Pending with `FailedScheduling: cannot allocate all claims`

No node can satisfy the claim. Typically the CEL selectors match no device, the requested
count exceeds available devices, or all matching devices are already in use. The upstream
message is generic and does not name the offending selector. Inspect what the driver
published and cross-check it against your selectors:

```bash
kubectl get resourceslices -o yaml
kubectl describe resourceclaim <claim-name>
```

### Reconciliation fails with `no matches for kind "ResourceClaimTemplate"`

The cluster does not serve `resource.k8s.io/v1`. Either upgrade to Kubernetes v1.34+, or
remove the `exp-dra-device-class` annotation to disable Managed DRA on this workload, the
cleanup path is tolerant of the missing API and lets the rest of the reconciliation
proceed normally.

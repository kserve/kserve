# 07 - Secure Adapter Supply Chain

This directory demonstrates securing the LoRA adapter supply chain by adding
signature verification and safety scanning to the storage initializer. A bad
adapter is stopped before it ever reaches GPU memory.

## Why this matters for LoRA specifically

LoRA adapters introduce a unique supply chain risk compared to base models:

- **Adapters are small and frequently updated.** A 50 MB adapter is easy to
  tamper with, easy to redistribute, and changes much more often than a 14 GB
  base model. Every update is a new trust decision.

- **Dynamic loading amplifies the attack surface.** With `02_dynamic_lora_lifecycle`,
  adapters can be loaded at runtime from HuggingFace without a CR update. If
  someone pushes a malicious adapter to a repo you trust, it gets loaded.

- **Base model integrity checks don't help.** The base model hash is untouched —
  weight-poisoning backdoors live entirely in the adapter tensors. A compromised
  LoRA adapter on a clean base model passes all base model verification.

### Known attack vectors

| Attack | Impact | Mitigation |
|--------|--------|------------|
| **Pickle deserialization** | Arbitrary code execution when model is loaded | ModelScan + safetensors format |
| **Weight poisoning** ([LoRATK](https://aclanthology.org/2025.findings-emnlp.1253.pdf), [CBA](https://arxiv.org/abs/2512.19297)) | Backdoor triggers in inference output | Signature verification (provenance) |
| **Supply chain hijack** | Attacker overwrites adapter in registry | Sigstore signature + transparency log |
| **Typosquatting** | User loads `finance-Iora` (capital I) instead of `finance-lora` | Signature verification (identity pinning) |

## How it works

The storage initializer runs two verification steps after downloading artifacts
and before the vLLM container starts:

```
┌─────────────────────────────────────────────────────────────────┐
│ Init Container: storage-initializer                             │
│                                                                 │
│  1. Download base model + LoRA adapters (existing behavior)     │
│                                                                 │
│  2. Verify Sigstore signatures (KSERVE_VERIFY_SIGNATURES=true)  │
│     └─ Checks signature exists and is valid                     │
│     └─ Verifies signer identity against transparency log        │
│     └─ FAILS CLOSED: no signature = pod does not start          │
│                                                                 │
│  3. Scan for unsafe patterns (KSERVE_SCAN_MODELS=true)          │
│     └─ Detects pickle deserialization attacks                    │
│     └─ Detects malicious Lambda layers                          │
│     └─ FAILS CLOSED: unsafe pattern = pod does not start        │
│                                                                 │
│  ✓ Only after both pass → vLLM container starts                 │
└─────────────────────────────────────────────────────────────────┘
```

### Enabling verification

Set environment variables on the `storage-initializer` init container:

```yaml
spec:
  template:
    initContainers:
    - name: storage-initializer
      env:
        - name: KSERVE_VERIFY_SIGNATURES
          value: "true"
        - name: KSERVE_SCAN_MODELS
          value: "true"
```

## Code changes

This prototype extends two files in the storage initializer:

### `python/storage/kserve_storage/model_verification.py` (new)

Post-download verification module with two pluggable checks:

- **`verify_signatures()`** — Uses the [OpenSSF model-signing](https://github.com/sigstore/model-transparency)
  library to verify Sigstore signatures on downloaded artifacts. The model-signing
  library is the emerging standard for ML model signing, adopted by HuggingFace,
  NVIDIA NGC, and Cohere.

- **`scan_models()`** — Uses [ModelScan](https://github.com/protectai/modelscan)
  to detect unsafe serialization patterns (pickle code execution, malicious layers).

### `python/storage-initializer/scripts/initializer-entrypoint` (modified)

Calls `run_post_download_verification(dest_paths)` after `Storage.download_files()`
completes. Both verification steps are gated by environment variables and are
no-ops when disabled.

## Deployment

```bash
# Edit kustomization.yaml to set your namespace
# Edit httproute.yaml to set your gateway name and namespace

kubectl apply -k .
```

When the pod starts, check the init container logs for verification output:

```bash
kubectl logs <pod-name> -c storage-initializer
```

Expected output when verification passes:
```
INFO: Post-download verification enabled (signatures=True, scan=True)
INFO: Verifying signature for: /mnt/models
INFO: Signature verification passed for /mnt/models (signer: publisher@example.com)
INFO: Verifying signature for: /mnt/lora/finance-lora
INFO: Signature verification passed for /mnt/lora/finance-lora (signer: publisher@example.com)
INFO: Scanning for unsafe patterns: /mnt/models
INFO: Safety scan passed for /mnt/models (scanned 12 files)
INFO: Scanning for unsafe patterns: /mnt/lora/finance-lora
INFO: Safety scan passed for /mnt/lora/finance-lora (scanned 3 files)
INFO: All post-download verification passed.
```

When verification fails, the pod stays in `Init:Error`:
```
ERROR: Post-download verification failed: Signature verification FAILED for
/mnt/lora/finance-lora: no signature found. Ensure the model was signed with
Sigstore before publishing.
```

## Signing your own adapters

To sign a LoRA adapter before pushing to HuggingFace:

```bash
pip install model-signing

# Sign the adapter directory (uses Sigstore keyless signing via OIDC)
model-signing sign --path ./my-lora-adapter/

# Push to HuggingFace (signature files are included automatically)
huggingface-cli upload my-org/my-lora-adapter ./my-lora-adapter/
```

## Integration with Red Hat Trusted Artifact Signer (RHTAS)

For enterprise deployments, Red Hat Trusted Artifact Signer provides a
private Sigstore deployment that integrates with your organization's identity
provider. This gives you:

- **Private transparency log** — Signatures are recorded in your own Rekor
  instance, not the public Sigstore log.
- **Corporate identity** — Adapters are signed with your organization's OIDC
  provider (e.g., RHSSO, Keycloak), not personal GitHub/Google accounts.
- **Policy enforcement** — Define policies for which identities are allowed
  to sign adapters (e.g., only the ML platform team).

To use RHTAS, configure the storage initializer with your private Sigstore
endpoints via environment variables (details TBD as the integration matures).

## Limitations

- **Weight-poisoning backdoors are not detected by scanning.** ModelScan
  catches code execution attacks, but backdoors embedded in tensor values
  require statistical analysis of the weights. The primary mitigation for
  weight poisoning is signature verification (proving who created the adapter).
- **Safetensors format is recommended** but not enforced. Adapters in pickle
  format will be scanned but are inherently riskier.
- **Signature verification requires adapters to be signed.** Most public
  HuggingFace adapters are not yet signed. This feature is most useful for
  enterprise-internal adapters where you control the signing pipeline.

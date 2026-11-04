# LLMInferenceService samples

Read the root `AGENTS.md` first. This guide covers `docs/samples/llmisvc/` only; InferenceService, InferenceGraph and cache samples elsewhere under `docs/samples/` serve other API versions. Nothing in CI applies these manifests, so they drift unless the author checks them.

- Work out which preset under `config/llmisvcconfig/` the sample resolves to. A single-node vLLM spec without `prefill` selects `config-llm-template`, where vLLM serves on port 8000. A prefill/decode spec selects the decode and prefill templates, where the decode vLLM serves on 8001. sglang and omni runtimes select their own templates. Match probes and ports to that template.
- Keep only the fields that differ from that preset.
- Do not spell out `baseRefs` the controller injects.
- Do not add probes for ports the selected template does not use.
- Use the typed field where one exists, for example `router.scheduler.config`. Do not use raw config.
- Use `serving.kserve.io/v1alpha2` and the current field names. Why: `kubectl apply` prunes a block under a field that does not exist, such as `spec.workload`, in silence.
- Run the README top to bottom on a fresh cluster before you open the PR.
- Give every command its namespace.
- Give every placeholder a working default. Use `gatewayClassName: envoy`, not `change-me`.
- Point every link at a file that exists.
- Keep a `kustomization.yaml` deploying what it deployed before.
- Set `router.gateway.refs` in a sample that binds a custom Gateway.
- Check the preset's flags against the new version when you bump an image a preset also pins. Keep the sample's tag consistent with the e2e fixtures.
- Say which cluster-wide knobs a sample flips. Example: `ociInsecureRegistry` switches every OCI import.
- Say which script must run first.

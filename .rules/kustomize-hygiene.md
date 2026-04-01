# Midstream Kustomize Hygiene Rules

These rules apply to **YAML files under `config/`**. Skip for non-YAML or non-`config/` file diffs.

This repository uses kustomize overlays to separate upstream manifests from midstream
(OpenShift/ODH) customizations. Upstream manifests under `config/` must stay clean. All OCP/ODH
additions go into `config/overlays/odh/` as kustomize strategic merge or JSON patches - never as
direct edits to upstream manifests.

## Violations - flag as blocking

1. **OCP/ODH content outside the ODH overlay** - If a YAML file outside `config/overlays/odh/`
   contains `openshift.io` or `opendatahub.io` in annotations, labels, resource API groups, or any
   other field, flag it. This content belongs in a patch under `config/overlays/odh/patches/`.
   Direct edits to upstream manifests cause merge conflicts on every sync.

2. **Commented-out YAML blocks** - Commented-out sections in any `config/**/*.yaml` file are a
   violation. Use kustomize patches or transformers to remove or replace content instead. Flag any
   consecutive `#`-prefixed lines that, if the `#` were removed, would parse as valid YAML key-value
   pairs, sequences, or mapping nodes. Do not flag single-line descriptive comments (e.g. `# This file
   configures X`). Suggest the appropriate kustomize patch type (strategic merge or JSON patch).
   Place the patch under `config/overlays/odh/patches/` and reference it in the overlay's
   `kustomization.yaml`.

## Exemptions - do not flag

- `config/crd/external/` - vendored third-party CRDs; `opendatahub.io` group references here are
  expected and correct
- `config/overlays/odh/` - the ODH overlay itself; OCP content here is the point
- `config/overlays/odh-test/` - ODH-specific test overlay; OCP content is intentional here
- `config/overlays/odh-xks/` - ODH on external Kubernetes; midstream overlay, OCP content is intentional here
- `config/overlays/test/configmap/inferenceservice-openshift-ci-raw.yaml` - known pre-existing drift, tracked for migration to `config/overlays/odh-test/`
- `config/overlays/test/configmap/inferenceservice-openshift-ci-serverless.yaml` - known pre-existing drift, tracked for migration to `config/overlays/odh-test/`
- `config/overlays/test/configmap/inferenceservice-openshift-ci-serverless-predictor.yaml` - known pre-existing drift, tracked for migration to `config/overlays/odh-test/`
- `config/rbac/role.yaml` - known pre-existing drift (`route.openshift.io` at lines 188 and 200), tracked for migration to the ODH overlay

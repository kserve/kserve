# Midstream RBAC Isolation Rules

These rules apply to **Go source files** (`*.go`, `*_test.go`). Skip for non-Go file diffs.

This repository uses kubebuilder RBAC markers (`//+kubebuilder:rbac:...`) to generate ClusterRole
manifests. Markers for OCP/ODH-specific API groups must live in a dedicated `distro/` sub-package
so they generate a separate ClusterRole and do not contaminate the upstream-generated `role.yaml`.

Correct pattern:
- `pkg/<controller>/distro/controller_rbac_ocp.go` - file containing only `//+kubebuilder:rbac:`
  markers, **no build tag** (controller-gen must scan it in all configurations), processed by a
  separate `controller-gen` invocation in `Makefile.overrides.mk`

## Violations - flag as blocking

1. **OCP/ODH RBAC markers outside `distro/`** - If a file that is NOT under a `distro/`
   package path contains `//+kubebuilder:rbac:` markers referencing `*.opendatahub.io` or
   `*.openshift.io` groups, flag it. These markers pollute the upstream
   `role.yaml` with OCP-specific permissions. Move them to
   `pkg/<controller>/distro/controller_rbac_ocp.go`.

2. **OCP/ODH RBAC markers without build tag outside `distro/`** - A stricter form of
   violation #1: same markers as above, additionally in a file without a `//go:build distro` header
   and not in a `distro/` path. If this applies, flag #1 as well. The absence of a build tag means
   the OCP-specific code is compiled unconditionally into the upstream build, making this more
   urgent than #1 alone. The fix is the same: move markers to `distro/` and add the build tag to
   any remaining OCP logic.

## Advisory - flag as non-blocking comment

3. **Istio RBAC markers outside `distro/`** - If a file outside a `distro/` path contains
   `//+kubebuilder:rbac:` markers referencing `*.istio.io` groups, leave a non-blocking comment
   asking whether this permission is OCP/midstream-specific. If it is, it should move to `distro/`.
   If it is genuinely needed by upstream kserve (istio is used upstream too), no action is needed.

   Note: this advisory treatment is intentionally more lenient than the import rule in
   `build-tags.md`, which requires `//go:build distro` for any `istio.io/` import. The reason:
   an import is a compilation artifact (isolate it to avoid upstream build failures), but an RBAC
   permission for `networking.istio.io` may be legitimately needed by upstream kserve regardless of
   where the import lives. If the RBAC marker is midstream-only, move it to `distro/`; if it is
   upstream-needed, leave it and the import rule still applies to the import itself.

## Exemptions - do not flag

- Files under a `distro/` sub-package that contain **only** a `package` declaration and
  `//+kubebuilder:rbac:` marker comments - no function definitions, no type definitions, no imports.
  No build tag is intentional - `controller-gen` must parse them regardless of build configuration,
  and since the file contains no executable code, there is nothing for a build constraint to exclude.
  The Go compiler parses and compiles this file in all configurations (it is just nearly empty);
  `controller-gen` scans it unconditionally to extract the RBAC markers - that is why no build tag
  is needed or wanted.
  This is the canonical correct pattern; see `pkg/controller/v1alpha2/llmisvc/distro/controller_rbac_ocp.go`
  as a reference.

## Pre-existing technical debt - do not flag on unmodified files

The following files carry OCP-group RBAC markers predating this policy. Flag only if new markers
are **added** in this PR:

- `pkg/controller/v1beta1/inferenceservice/controller.go` (`route.openshift.io`,
  `networking.istio.io` (treated as OCP-specific for exemption purposes))
- `pkg/controller/v1alpha1/inferencegraph/controller.go` (`route.openshift.io`,
  `networking.istio.io` (treated as OCP-specific for exemption purposes))

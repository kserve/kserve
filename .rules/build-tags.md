# Midstream Build Tags and Companion File Rules

These rules apply to **Go source files** (`*.go`, `*_test.go`). Skip this rule for non-Go file diffs.

This repository is a midstream fork of kserve/kserve running on OpenShift (OCP). Platform-specific
code must be isolated using Go build tags so upstream syncs stay conflict-free. OCP-specific logic
lives in `*_ocp.go` files compiled only with `//go:build distro`; the upstream fallback lives in
`*_default.go` files compiled with `//go:build !distro`.

## Violations - flag as blocking

1. **Missing build tag on OCP imports** - If a file imports packages whose import path contains
   `openshift/`, `opendatahub/`, or `istio.io/`, it must have `//go:build distro` before the `package`
   declaration. Flag if the header is absent.

2. **`*_ocp.go` without build tag** - Any file named `*_ocp.go` or `*_ocp_test.go` must have
   `//go:build distro` before the `package` declaration. Flag if missing.

3. **`*_default.go` without build tag** - Any file named `*_default.go` must have
   `//go:build !distro` before the `package` declaration. Flag if missing.

4. **Commented-out code blocks** - Commented-out function bodies, struct fields, type definitions,
   or conditional branches are a violation. They rot, cause merge conflicts, and are never
   re-enabled. Use `//go:build` compile-time exclusion instead. Flag any `//` comment that wraps
   meaningful code (not documentation or inline explanation). Suggest the author remove the commented
   block or move it behind a `//go:build` constraint instead.

5. **OCP logic in non-companion file** - If a file contains OCP-specific imports or logic (signals:
   `openshift/`, `opendatahub/`, `istio.io/` import paths) and is not named `*_ocp.go` or
   `*_ocp_test.go`, flag it. Suggest extracting the OCP-specific parts to a `<basename>_ocp.go`
   companion with a `<basename>_default.go` with `//go:build !distro` and stub implementations of the same function signatures.

6. **Missing default companion** - If a `*_ocp.go` file is added in this PR but no corresponding
   `*_default.go` exists in the same package (check both the PR diff and the existing repo tree),
   flag it. Upstream builds without `GOTAGS=distro` will fail to link. If the reviewer cannot inspect the
   full repository tree (diff-only mode), flag tentatively and ask the author to confirm whether a
   `*_default.go` companion exists in the same package.

## Exemptions - do not flag

- Files under a `distro/` sub-package (e.g. `pkg/controller/.../distro/controller_rbac_ocp.go`)
  that contain no executable code - package declaration, license/copyright header, explanatory
  comments, and `//+kubebuilder:rbac:` markers are all fine. These intentionally have no build tag
  and are not named `*_ocp.go` - `controller-gen` must scan them in all build configurations.
  Exempt from violations #2, #3, and #5.

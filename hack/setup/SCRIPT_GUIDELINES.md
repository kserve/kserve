# Script Guidelines

Guidelines for writing infrastructure installation scripts.

## Quick Reference

**Required structure:**
```bash
#!/bin/bash

# INIT
SCRIPT_DIR="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
source "${SCRIPT_DIR}/../common.sh"
REINSTALL="${REINSTALL:-false}"
UNINSTALL="${UNINSTALL:-false}"
if [[ "$*" == *"--uninstall"* ]]; then
    UNINSTALL=true
elif [[ "$*" == *"--reinstall"* ]]; then
    REINSTALL=true
fi
# INIT END

check_cli_exist helm kubectl  # Optional

# VARIABLES (Optional)
NAMESPACE="${NAMESPACE:-default}"
VERSION="${VERSION:-v1.0.0}"
# VARIABLES END

# INCLUDE_IN_GENERATED_SCRIPT_START (Optional)
if [ "${CUSTOM_MODE}" = "true" ]; then
    NAMESPACE="custom-namespace"
fi
# INCLUDE_IN_GENERATED_SCRIPT_END

uninstall() {
    # Cleanup logic
}

install() {
    # Check if installed, handle reinstall, install component
}

# Main
if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi
install
```

---

## Function Naming

**Individual scripts**: Use simple names
```bash
install() { ... }
uninstall() { ... }
```

**Why**: Easy to find. Generator renames to `install_<component>()` in combined scripts.

---

## Standard Sections

### INIT Section (Required)

```bash
# INIT
SCRIPT_DIR="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
source "${SCRIPT_DIR}/../common.sh"
REINSTALL="${REINSTALL:-false}"
UNINSTALL="${UNINSTALL:-false}"
if [[ "$*" == *"--uninstall"* ]]; then
    UNINSTALL=true
elif [[ "$*" == *"--reinstall"* ]]; then
    REINSTALL=true
fi
# INIT END
```

### VARIABLES Section (Optional)

For component-specific variables extracted by the generator:

```bash
# VARIABLES
NAMESPACE="${NAMESPACE:-default}"
RELEASE_NAME="${RELEASE_NAME:-my-component}"
# VARIABLES END
```

**Rules**:
- Only variable assignments (`VAR=value`)
- No control flow (`if`, `for`, etc.) - use INCLUDE section instead

### INCLUDE Section (Optional)

For code that must be included in generated scripts (conditionals, helper functions, etc.):

```bash
# INCLUDE_IN_GENERATED_SCRIPT_START
if [ "${LLMISVC}" = "true" ]; then
    CRD_DIR="${REPO_ROOT}/config/crd/llmisvc"
    CONFIG_DIR="${REPO_ROOT}/config/overlays/llmisvc"
fi
# INCLUDE_IN_GENERATED_SCRIPT_END
```

**Rules**:
- Must appear after VARIABLES END
- Can contain any bash code
- Placed before component functions in generated scripts

### Install Function Pattern

```bash
install() {
    # Check if already installed
    if <already-installed>; then
        if [ "$REINSTALL" = false ]; then
            log_info "Already installed. Use --reinstall to reinstall."
            exit 0
        fi
        uninstall
    fi

    # Install
    log_info "Installing component ${VERSION}..."
    helm install ...
    log_success "Installed component ${VERSION}"

    # Verify (optional)
    wait_for_deployment "namespace" "selector" "timeout"
}
```

### Main Execution

```bash
if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi
install
```

---

## Environment Variables

### Versions

Centralized in `kserve-deps.env`:
```bash
CERT_MANAGER_VERSION=v1.16.2
ISTIO_VERSION=1.24.1
```

Use in scripts:
```bash
helm install component ... --version "${COMPONENT_VERSION}"
```

**Never hardcode versions.** Generated scripts embed versions from `kserve-deps.env`.

### Custom Helm Args

Allow users to override defaults:
```bash
# In code:
helm install component repo/chart \
    --version "${VERSION}" \
    ${COMPONENT_EXTRA_ARGS:-}
```

---

## Helper Functions (from common.sh)

### Logging
```bash
log_info "message"       # Blue
log_success "message"    # Green
log_error "message"      # Red
log_warning "message"    # Yellow
```

### Wait Functions
```bash
wait_for_pods "namespace" "label-selector" "timeout"        # Wait for pods by label
wait_for_deployment "namespace" "deployment-name" "timeout" # Wait for deployment
wait_for_crds "timeout" "crd1" "crd2"                       # Wait for CRDs
```

### Utilities
```bash
check_cli_exist helm kubectl
create_or_skip_namespace "namespace"
detect_platform  # Returns: kind, minikube, openshift, kubernetes
```

---

## Validation

All scripts are automatically validated to ensure they follow these guidelines.

### Running Validation

```bash
# Validate all scripts
./scripts/validate-install-scripts.py

# Validate specific file
./scripts/validate-install-scripts.py infra/manage.component.sh

# Validate directory
./scripts/validate-install-scripts.py infra

# CI mode (fail on errors)
./scripts/validate-install-scripts.py --strict
```

### What's Checked

- **Required sections**: INIT markers and content
- **Required functions**: `install()` and `uninstall()`
- **Optional sections**: VARIABLES and INCLUDE_IN_GENERATED_SCRIPT markers (if used)
- **Best practices**: Proper section ordering, no control flow in VARIABLES section

Run the validator to see detailed error messages with rule IDs (e.g., `TEMPLATE-01`, `FUNC-01`) and fix suggestions.


---

## Testing Checklist

```bash
# Install
./manage.component.sh

# Already installed (should skip)
./manage.component.sh

# Reinstall
./manage.component.sh --reinstall

# Uninstall
./manage.component.sh --uninstall

# Custom args
COMPONENT_EXTRA_ARGS="--set foo=bar" ./manage.component.sh
```

---

## Generator Integration

Scripts following these guidelines auto-generate into quick-install scripts.

**Definition file** (`.definition`):
```yaml
FILE_NAME: llmisvc-quick-install
DESCRIPTION: Install LLM InferenceService dependencies and components
METHOD: helm
RELEASE: true

COMPONENTS:
  - name: cert-manager
  - name: kserve-helm
    env:
      LLMISVC: "true"
```

**Generate**:
```bash
./scripts/generate-install-script.py quick-install/definitions/llmisvc-install.definition
```

**What happens**:
1. Extracts VARIABLES from each component script
2. Extracts INCLUDE sections (placed before component functions)
3. Renames `install()` → `install_<component>()`
4. If `RELEASE: true`, embeds Kubernetes manifests from kustomize
5. Creates single-file installer with all dependencies

**Result**: Standalone installer that works without the repository.

---

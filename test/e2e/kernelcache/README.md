# KernelCache E2E Tests

End-to-end tests for KernelCache functionality in KServe.

## Overview

KernelCache provides GPU kernel caching to accelerate inference workload startup by pre-compiling and caching GPU kernels (PyTorch, Triton, vLLM) in OCI images.

**Test Image:** Tests use `quay.io/gkm/cache-examples:vector-add-cache-rocm-v2` - a GKM example kernel cache image with ROCm kernels.

These E2E tests verify the complete workflow:
1. KernelCache CRD creation triggers extraction
2. KernelCacheNode auto-created for each cluster node
3. GPU detection populates hardware info (stub mode for testing)
4. Extraction Job downloads and extracts cache from OCI image
5. Cache becomes Ready and available for pod mounting
6. Proper cleanup via finalizers

## Test Files

- `test_kernelcache_basic.py` - Basic KernelCache creation, extraction, and cleanup

## Running Tests

### Prerequisites

1. **Kubernetes cluster** (minikube recommended for local testing):
   ```bash
   # Start minikube with multiple nodes
   minikube start --nodes 3 --driver=docker --cpus=4 --memory=8192
   ```

2. **Install uv (Python package manager)**:
   ```bash
   # From kserve project root
   ./test/scripts/gh-actions/setup-uv.sh
   
   # Or install manually
   # curl -LsSf https://astral.sh/uv/install.sh | sh
   # export PATH="$HOME/.local/bin:$PATH"
   ```

3. **Install dependencies (cert-manager, Knative, Istio)**:
   ```bash
   # Install infrastructure dependencies
   ./test/scripts/gh-actions/setup-deps.sh
   ```

   This installs:
   - cert-manager for webhook certificates
   - Knative Serving
   - Istio networking layer

4. **Build and load KServe images**:
   
   For local testing, build images and load into minikube:

   ```bash
   # Build images
   export TAG=latest
   export KO_DOCKER_REPO=kserve
   make docker-build                    # main controller
   make docker-build-kernelcache        # kernelcache controller
   make docker-build-kernelcachenode-agent  # kernelcache agent

   # Load into minikube (automatically loads to all nodes)
   minikube image load kserve/kserve-controller:latest
   minikube image load kserve/kserve-kernelcache-controller:latest
   minikube image load kserve/kserve-kernelcachenode-agent:latest

   # Verify images loaded
   minikube ssh -- docker images | grep kserve
   ```

   **Alternative:** Build directly in minikube's docker (faster):
   ```bash
   eval $(minikube docker-env)
   export TAG=latest
   export KO_DOCKER_REPO=kserve
   make docker-build docker-build-kernelcache docker-build-kernelcachenode-agent
   eval $(minikube docker-env -u)
   ```

5. **Install KServe**:
   ```bash
   # Deploy KServe controllers and webhooks
   export TAG=latest
   export SET_KSERVE_VERSION=latest
   ./test/scripts/gh-actions/setup-kserve.sh
   ```

   This deploys:
   - KServe controller and webhooks
   - Creates necessary namespaces
   - Installs KServe Python SDK with test dependencies
   - Sets up local S3 storage (seaweedfs)

6. **Configure KernelCache for testing**:
   
   **Note:** KernelCache is already enabled via `config/overlays/test` (component included in kustomization.yaml). The test uses signed GKM example images that pass cosign verification.

   ```bash
   # Enable stub GPU mode and permission init container (required for minikube)
   kubectl patch configmap inferenceservice-config -n kserve --type merge -p '{
     "data": {
       "kernelcache": "{\"noGPU\": true, \"jobNamespace\": \"kserve\", \"enablePermissionInitContainer\": true}"
     }
   }'
   ```
   
   **Why `enablePermissionInitContainer: true`?** Minikube's storage provisioner creates volumes owned by root. The init container fixes permissions so the extraction Job can write as user 1000.

7. **Label nodes for KernelCache**:
   ```bash
   kubectl label nodes -l '!node-role.kubernetes.io/control-plane' kserve/kernelcache=worker
   ```

### Run Tests

```bash
# Activate the virtual environment created by setup-kserve.sh
source python/kserve/.venv/bin/activate

# Run all KernelCache E2E tests
cd test/e2e
pytest -m "kernelcache" kernelcache/

# Run with verbose output
pytest -m "kernelcache" kernelcache/ -v --log-cli-level=INFO

# Run specific test
pytest kernelcache/test_kernelcache_basic.py::test_kernelcache_basic -v

# Deactivate when done
deactivate
```

### Cleanup / Teardown

After running tests, clean up resources:

```bash
# Delete all KernelCache CRs (triggers finalizer cleanup)
kubectl delete kernelcaches --all -A

# Delete KernelCacheNode CRs
kubectl delete kernelcachenodes --all

# Wait for finalizers to complete (up to 60s)
sleep 60

# Verify PVCs/Jobs cleaned up
kubectl get pvc -A | grep kernel
kubectl get jobs -n kserve | grep extract

# Uninstall KServe (optional - full teardown)
kubectl delete -k config/overlays/test

# Delete cert-manager (if installed for test only)
kubectl delete -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.4/cert-manager.yaml

# Stop minikube
minikube stop

# Delete minikube cluster (full cleanup)
minikube delete
```

**Quick cleanup** (keep cluster, remove only test resources):
```bash
kubectl delete kernelcaches --all -A
kubectl delete kernelcachenodes --all
kubectl delete ns kserve-test  # if using separate test namespace
```

## Test Markers

- `@pytest.mark.kernelcache` - All kernel cache tests
- `@pytest.mark.asyncio(scope="session")` - Async test with session scope

## Expected Behavior

### Successful Test Flow

1. **Create KernelCache** with OCI image URI
2. **Mutating webhook** resolves image digest and adds annotations
3. **Validating webhook** verifies signature and Kyverno annotations (if enabled)
4. **Controller creates**:
   - KernelCacheNode per cluster node (auto-discovery)
   - Download PVC in kserve namespace
   - Extraction Job to pull and extract OCI image
5. **GPU detection** populates KernelCacheNode.Status.GPUInfo (stub mode returns 2x AMD MI210)
6. **Extraction Job** completes successfully
7. **Controller creates** ServingPVC in cache namespace for pod mounting
8. **Cache ready** when cacheCopies.available == cacheCopies.total and failed == 0
9. **Cleanup** deletes Job, PVCs, PVs via finalizers

### Verification Points

Tests verify:
- ✓ KernelCacheNode exists for each worker node
- ✓ GPUInfo populated with stub GPU data (2x AMD MI210 in stub mode)
- ✓ Extraction Job created with correct labels
- ✓ Extraction Job completes (status.succeeded > 0)
- ✓ Cache ready (status.cacheCopies.available == status.cacheCopies.total)
- ✓ ServingPVC created in cache namespace
- ✓ Download PVC created in kserve namespace
- ✓ Resources cleaned up after deletion

## Troubleshooting

### Image Pull Errors

If pods show `ImagePullBackOff` or `ErrImagePull`:

```bash
# Check if images exist in minikube
minikube ssh -- docker images | grep kserve

# If missing, build and load images
export TAG=latest
export KO_DOCKER_REPO=kserve
make docker-build docker-build-kernelcache docker-build-kernelcachenode-agent
minikube image load kserve/kserve-controller:latest
minikube image load kserve/kserve-kernelcache-controller:latest
minikube image load kserve/kserve-kernelcachenode-agent:latest

# Restart pods to pick up images
kubectl rollout restart deployment/kserve-controller-manager -n kserve
kubectl rollout restart daemonset/kserve-kernelcachenode-agent -n kserve
```

### Test Failures

**KernelCacheNode not created:**
- Check controller logs: `kubectl logs -n kserve deployment/kserve-controller-manager`
- Verify RBAC permissions for node watching

**GPU detection empty:**
- Verify `NO_GPU=true` in controller deployment
- Check KernelCacheNode agent logs (when deployed)

**Extraction Job fails:**
- Check Job logs: `kubectl logs -n kserve job/extract-<cache-name>-<hash>`
- Verify image exists and is accessible
- Check webhook added digest annotation

**Permission denied errors in extraction Job:**
```bash
# Check Job logs for permission errors
kubectl logs -n kserve job/extract-<job-name>

# If you see "Permission denied" or "chown: Operation not permitted":
# Enable permission init container
kubectl patch configmap inferenceservice-config -n kserve --type merge -p '{
  "data": {
    "kernelcache": "{\"noGPU\": true, \"jobNamespace\": \"kserve\", \"enablePermissionInitContainer\": true}"
  }
}'

# Restart controller to pick up new config
kubectl rollout restart deployment/kserve-controller-manager -n kserve
```

**Cache not ready (cacheCopies.available < total):**
- Check Job status: `kubectl get job -n kserve`
- Check if any copies failed: `kubectl get kernelcache <name> -o jsonpath='{.status.cacheCopies}'`
- Verify ServingPVC created: `kubectl get pvc -n <namespace>`
- Check controller logs for reconciliation errors

**Webhook errors:**
- Verify webhook certificates: `kubectl get secret -n kserve kernelcache-webhook-server-cert`
- Check webhook configuration: `kubectl get mutatingwebhookconfiguration`
- Review webhook logs: `kubectl logs -n kserve deployment/kserve-controller-manager`

### Debugging

```bash
# Watch cache status
kubectl get kernelcache -n default -w

# Check KernelCacheNode
kubectl get kernelcachenode

# View GPU detection results
kubectl get kernelcachenode <node-name> -o jsonpath='{.status.gpuInfo}'

# Check extraction Job
kubectl get job -n kserve -l app=kernel-cache-extract

# View Job logs
kubectl logs -n kserve job/<job-name>

# Check PVCs
kubectl get pvc -A | grep kernel-cache

# Controller logs
kubectl logs -n kserve deployment/kserve-controller-manager -f
```

## Adding New Tests

Follow the pattern from `test_kernelcache_basic.py`:

1. Mark with `@pytest.mark.kernelcache`
2. Use `@pytest.mark.asyncio(scope="session")`
3. Get KServeClient and K8s clients from fixtures
4. Create KernelCache CR via custom object API
5. Wait and verify expected resources
6. Clean up in finally block

Example test ideas:
- Multiple caches on same node
- Cache update (change image)
- Delete with ServingStatus.TotalPods > 0 (should fail)
- Node failure handling
- Heterogeneous GPU types

## CI/CD Integration

For CI pipelines, see `.github/workflows/e2e-test-kernelcache.yaml` for the complete workflow.

**Manual CI-style run:**

```bash
# Setup cluster
minikube start --nodes 3 --cpus=4 --memory=8192 --driver=docker

# Install uv
./test/scripts/gh-actions/setup-uv.sh

# Install infrastructure dependencies (cert-manager, Knative, Istio)
./test/scripts/gh-actions/setup-deps.sh

# Build and load KServe images
export TAG=latest
export KO_DOCKER_REPO=kserve
make docker-build docker-build-kernelcache docker-build-kernelcachenode-agent
minikube image load kserve/kserve-controller:latest
minikube image load kserve/kserve-kernelcache-controller:latest
minikube image load kserve/kserve-kernelcachenode-agent:latest

# Install KServe
export SET_KSERVE_VERSION=latest
./test/scripts/gh-actions/setup-kserve.sh

# Configure KernelCache
kubectl patch configmap inferenceservice-config -n kserve --type merge -p '{
  "data": {
    "kernelcache": "{\"noGPU\": true, \"jobNamespace\": \"kserve\", \"enablePermissionInitContainer\": true}"
  }
}'

# Label nodes
kubectl label nodes -l '!node-role.kubernetes.io/control-plane' kserve/kernelcache=worker

# Run tests
source python/kserve/.venv/bin/activate
cd test/e2e
pytest -m "kernelcache" kernelcache/ --junitxml=results.xml
deactivate
```

## Related Documentation

- [KernelCache CRD Design](../../../../pkg/apis/serving/v1alpha1/kernel_cache_types.go)
- [KernelCache Controller](../../../../pkg/controller/v1alpha1/kernelcache/)
- [GPU Detection](../../../../pkg/controller/v1alpha1/kernelcachenode/gpu_detection.go)
- [Webhook Implementation](../../../../pkg/apis/serving/v1alpha1/kernel_cache_webhook.go)

# Creating a Custom AutoGluon Serving Runtime on Red Hat OpenShift AI

This guide describes how to add a **custom serving runtime** on Red Hat OpenShift AI so you can deploy AutoGluon TabularPredictor models on the **single-model serving platform** (KServe). The runtime uses the AutoGluon server image (for example from Quay.io).

Based on [Red Hat OpenShift AI documentation](https://docs.redhat.com/en/documentation/red_hat_openshift_ai_self-managed/latest/html/configuring_your_model-serving_platform/configuring_model_servers_on_the_single_model_serving_platform) and [AI on OpenShift custom runtime tutorials](https://ai-on-openshift.io/odh-rhoai/custom-runtime-triton/).

---

## Prerequisites

- **OpenShift AI** installed with the **model serving platform** (KServe) enabled.
- **Administrator** access to OpenShift AI (e.g. cluster admin or dedicated admin).
- **AutoGluon server image** built and pushed to a container registry (e.g. `quay.io/your-org/kserve-autogluonserver:latest`).  
  See the main [README](README.md) for building and pushing the image with `nerdctl` or Docker.
- Models must be saved with `TabularPredictor.save(path)` and stored in object storage (S3-compatible, Azure Blob, or GCS) or another supported storage that KServe can mount at `/mnt/models`.

---

## Important notes

- Ensure the AutoGluon image is **pulled from a registry your cluster can access** (e.g. Quay.io, or a mirrored registry in disconnected environments).

---

## Step 1: Log in as an administrator

1. Log in to the **OpenShift AI dashboard** with a user that has **administrator** privileges (e.g. cluster-admin or dedicated admin group).

---

## Step 2: Open Serving runtimes

1. In the left menu, go to **Settings** → **Model resources and operations** → **Serving runtimes**.  
2. The **Serving runtimes** page lists preinstalled runtimes and any custom runtimes already added.

---

## Step 3: Add the custom AutoGluon runtime

1. Click **Add serving runtime**.  
2. Choose the **single-model serving platform** (Model serving platform / KServe).  
   - Do **not** choose the multi-model serving platform (ModelMesh); the AutoGluon runtime is for KServe single-model.  
3. Select the API protocol: **REST** (the AutoGluon server exposes HTTP on port 8080).  
4. Add the runtime definition using one of:
   - **Upload a YAML file**  
     Click **Upload files** and select a file that contains the YAML below.  
   - **Start from scratch**  
     Click **Start from scratch** and paste the YAML into the editor.  
5. Use the following YAML (adjust the image and, if needed, namespace/labels to match your environment):

```yaml
apiVersion: serving.kserve.io/v1alpha1
kind: ServingRuntime
metadata:
  name: kserve-autogluonserver
  annotations:
    openshift.io/display-name: "AutoGluon Server (KServe)"
spec:
  annotations:
    prometheus.kserve.io/port: "8080"
    prometheus.kserve.io/path: "/metrics"
  supportedModelFormats:
    - name: autogluon
      version: "1"
      autoSelect: true
      priority: 2
  protocolVersions:
    - v1
    - v2
  containers:
    - name: kserve-container
      image: quay.io/rh-ee-dlaczak-org/kserve-autogluonserver:latest
      args:
        - --model_name=autogluon
        - --model_dir=/mnt/models
        - --http_port=8080
      securityContext:
        allowPrivilegeEscalation: false
        privileged: false
        runAsNonRoot: true
        capabilities:
          drop:
            - ALL
      resources:
        requests:
          cpu: "1"
          memory: 2Gi
        limits:
          cpu: "1"
          memory: 2Gi
```

6. Replace the **image** with your own if different, for example:
   - `quay.io/YOUR_ORG/kserve-autogluonserver:v0.0.1`
7. Click **Add**.

The new runtime appears in the list and is **enabled** by default. You can reorder runtimes on this page; the order affects how they are presented when deploying models.

---

## Step 4: Deploy an AutoGluon model

1. In a **Data Science Project**, open **Models and Model Servers** (or equivalent).  
2. **Configure server** (or add a model server) and ensure the **model serving platform** (KServe) is selected.  
3. **Deploy model** (or add a model):
   - Choose a **model name**.
   - Select **framework / format**: **autogluon** (this matches `supportedModelFormats.name` in the runtime).  
4. Configure **model storage**:
   - Create or select a **data connection** that points to your object storage (S3-compatible, Azure Blob, or GCS).
   - Set the **path** to the directory that contains the model saved with `TabularPredictor.save(path)` (e.g. `models/iris/`).  
5. Select the **AutoGluon** serving runtime (e.g. "AutoGluon Server (KServe)" or `kserve-autogluonserver`) if multiple runtimes are available.  
6. Set **resources** (CPU/memory) as needed; the runtime YAML above requests 1 CPU and 2Gi memory.  
7. Deploy; KServe creates an `InferenceService` that uses this runtime and mounts your model at `/mnt/models`.

---

## Step 5: Optional — Apply the runtime with the CLI

If you prefer to manage the runtime via the OpenShift/ Kubernetes CLI:

1. Save the YAML above to a file (e.g. `autogluon-serving-runtime.yaml`).  
2. Ensure the **namespace** is set in `metadata` if your OpenShift AI version expects it (e.g. the namespace where KServe runtimes are created, such as `redhat-ods-applications` or your data science project namespace).  
3. Apply the file:

```bash
oc apply -f autogluon-serving-runtime.yaml -n <namespace>
```

Use the namespace indicated by your OpenShift AI documentation or by inspecting where other serving runtimes are created.

---

## Configuration reference

| Field | Purpose |
|-------|--------|
| `supportedModelFormats.name: autogluon` | Matches the `modelFormat.name` in an InferenceService so this runtime is selected for AutoGluon models. |
| `protocolVersions: v1, v2` | KServe inference protocol versions the server supports. |
| `containers[0].image` | Your AutoGluon server image (Quay or other registry). |
| `containers[0].args` | `--model_dir=/mnt/models` is required; KServe mounts the model at `/mnt/models`. |
| `prometheus.kserve.io/*` | Optional; used by Prometheus to scrape metrics from the server. |

---

## Optional: Use probability predictions

To use **class probabilities** (`predict_proba`) instead of class labels, add an environment variable to the runtime container. In the YAML above, under the container, add:

```yaml
      env:
        - name: PREDICT_PROBA
          value: "true"
```

Then redeploy or re-apply the runtime.

---

## Verification

- In **Serving runtimes**, the custom runtime is listed and **enabled**.  
- When you deploy a model with format **autogluon**, the deployment uses this runtime and the model loads from `/mnt/models`.  
- You can call the inference endpoint (from the dashboard or via `curl`) to confirm predictions; see the main [README](README.md) for request format.

---

## References

- [Red Hat OpenShift AI – Configuring model servers (single-model serving platform)](https://docs.redhat.com/en/documentation/red_hat_openshift_ai_self-managed/latest/html/configuring_your_model-serving_platform/configuring_model_servers_on_the_single_model_serving_platform)
- [Red Hat OpenShift AI – Adding a custom model-serving runtime](https://docs.redhat.com/en/documentation/red_hat_openshift_ai_self-managed/latest/html/configuring_your_model-serving_platform/configuring_model_servers#adding-a-custom-model-serving-runtime_rhoai-admin)
- [Deploying a custom serving runtime in ODH/RHOAI (AI on OpenShift)](https://ai-on-openshift.io/odh-rhoai/custom-runtime-triton/)
- [KServe ModelMesh runtimes (examples)](https://github.com/kserve/modelmesh-serving/tree/main/config/runtimes)

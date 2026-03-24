# 📦 KServe Deployment Pipeline Template

A template to use **Data Science Pipelines** to deploy a model to **KServe** in **Open Data Hub (ODH)** or **Red Hat OpenShift AI (RHOAI)**.

---

## 📋 Prerequisites

- Add the appropriate ServingRuntime manifest to the manifests/ directory.

Ensure the following components are installed and properly configured on your OpenShift cluster:

- ✅ Red Hat OpenShift Serverless
- ✅ Red Hat OpenShift Service Mesh
- ✅ Authorino
- ✅ Open Data Hub (ODH)

After installing ODH:
- Create a **DataScienceClusterInitiator (DSCI)** resource
- Create a **DataScienceCluster (DSC)** resource

---

## 🚀 Getting Started

### 1. Clone this repository
```bash
git clone https://github.com/opendatahub-io/kserve.git
cd kserve/docs/kfp/template
```

### 2. (Optional) Create and activate a virtual environment
```bash
virtualenv -p python3.11 /tmp/venv
source /tmp/venv/bin/activate
```

### 3. Install project dependencies
```bash
uv pip compile requirements.in -o requirements.txt
pip install -r requirements.txt
```

### 4. Add your `ServingRuntime` manifest to the `manifests/` directory
Ensure that your servingruntime file is renamed to ```runtime.yaml```
Ensure it is properly formatted and includes the correct metadata name.

## 📦 Deploying a Model

Run the `main.py` script with the required flags:
```bash
python main.py \
  --namespace {NAMESPACE} \
  --action [apply|create] \
  --model_name {MODEL_NAME} \
  --model_uri {MODEL_URI} \
  --framework {FRAMEWORK}
```

This will generate a `pipeline.yaml` file.

---

## 🧹 Deleting a Model

To delete the model:
```bash
python main.py --action delete --model_name {MODEL_NAME}
```

---

## 📂 Running the Pipeline in ODH/RHOAI

1. Open the **ODH/RHOAI Dashboard**
2. Navigate to:  
   `Data Science Pipelines > Pipelines > {NAMESPACE}`
3. Click **Import Pipeline**
4. Select the generated `pipeline.yaml` file
5. Click **Import Pipeline**
6. Go to **Actions > Create Run**

---

## 📄 Example Usage

```bash
python main.py \
  --namespace demo-namespace \
  --action create \
  --model_name sklearn-model \
  --model_uri gs://kfserving-examples/models/sklearn/1.0/model \
  --framework sklearn
```

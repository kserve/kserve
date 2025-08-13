This project makes it easy to install KServe and its key dependencies (Istio, Cert Manager, Prometheus) using Helm and a lightweight Python CLI. It is designed to help users — especially beginners — get started with KServe quickly without needing deep Kubernetes expertise.

📦 What’s Included
✅ A Helm chart for installing KServe and dependencies
🛠️ A lightweight Python CLI (kserve-lite.py) for quick install and model deployment
📈 A sample Scikit-learn model InferenceService
⚙️ Easy customization via values.yaml
✅ Prerequisites
Before starting, make sure you have the following installed:

A working Kubernetes cluster (Minikube, KIND, GKE, etc.)
kubectl
helm (version 3 or later)
Python 3.x
Git
🧭 Step-by-Step Instructions
Step 1: Clone the Repository
<pre>

git clone https://github.com/YOUR_USERNAME/kserve.git
cd kserve/contrib/helm/kserve-onboarding-chart
</pre>
Step 2: Install All Components Using the CLI
<pre> ```bash python cli/kserve-lite.py install ``` </pre>
This will install:

KServe

Istio Ingress Gateway

Cert Manager

Prometheus (optional, based on values.yaml)

All components are installed in the kserve namespace.

Step 3: Deploy the Sample ML Model (Scikit-learn)
<pre> ```bash python cli/kserve-lite.py deploy-sample ``` </pre>
This will deploy an example InferenceService that serves a Scikit-learn model using KServe.

Step 4: Verify the Deployment
<pre> ```bash kubectl get inferenceservices -n kserve ``` </pre>
You should see a service with Ready: True.

Step 5: Uninstall Everything (If Needed)
<pre> ```bash python cli/kserve-lite.py uninstall ``` </pre>
This will remove all installed components and the sample service.

⚙️ Configuration
All install options can be customized in values.yaml.

Example: Disable Prometheus
<pre> ```yaml prometheus: enabled: false ``` </pre>
Example: Change Istio Service Type
<pre> ```yaml istio: service: type: NodePort ``` </pre>
After updating the config, reinstall with:

<pre> ```bash python cli/kserve-lite.py uninstall helm install kserve-lite . -f values.yaml -n kserve --create-namespace ``` </pre>
🗂️ File Structure
pgsql
Copy
Edit
kserve-onboarding-chart/
├── Chart.yaml
├── values.yaml
├── templates/
│   ├── istio.yaml
│   ├── cert-manager.yaml
│   ├── prometheus.yaml
│   ├── sklearn-sample.yaml
├── cli/
│   └── kserve-lite.py
└── README.md
👩‍💻 Author
Diya Sharma
Contributor to KServe
Mentor: Akshay Mittal

📄 Short-form RFC
📘 Long-form Design Doc


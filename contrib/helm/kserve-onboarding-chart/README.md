KServe Onboarding Helm Chart + CLI Tool
This project makes it easy to install KServe and its key dependencies (Istio, Cert Manager, Prometheus) using Helm and a Python-based CLI. It is designed to help users — especially beginners — get started with KServe quickly without needing deep Kubernetes expertise.

What this includes:

A Helm chart for installing KServe and dependencies

A lightweight Python CLI (kserve-lite.py) for installation and sample deployment

A sample Scikit-learn model deployment

Easy customization through values.yaml

Prerequisites
Before starting, make sure you have the following installed:

A working Kubernetes cluster (Minikube, KIND, GKE, etc.)

kubectl

helm version 3 or later

Python 3

Git

Step-by-Step Instructions
Step 1: Clone the repository

bash

git clone https://github.com/YOUR_USERNAME/kserve.git
cd kserve/contrib/helm/kserve-onboarding-chart
Step 2: Install all components using the CLI

bash

python cli/kserve-lite.py install
This command installs the following:

KServe

Istio Ingress Gateway

Cert Manager

Prometheus (optional, based on values.yaml)
All components are installed in the kserve namespace.

Step 3: Deploy the sample ML model (Scikit-learn)

bash

python cli/kserve-lite.py deploy-sample
This deploys a working InferenceService that serves a Scikit-learn model using KServe.

Step 4: Verify the deployment

To check if the InferenceService is running:

Arduino

kubectl get inferenceservices -n kserve
You should see a service with Ready: True.

Step 5: Uninstall everything (if needed)

To clean up your cluster:

bash

python cli/kserve-lite.py uninstall
This removes all installed components and the sample service.

Configuration
All default installation options are defined in the values.yaml file. You can modify this file to enable/disable specific components or change settings.

Example: To disable Prometheus, set:

yaml

prometheus:
  enabled: false
Example: To change Istio service type:

yaml

istio:
  service:
    type: NodePort
After modifying the file, you can reinstall everything:

pgsql

python cli/kserve-lite.py uninstall
helm install kserve-lite . -f values.yaml -n kserve --create-namespace

File Structure
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
└── README


Diya Sharma
Contributor to KServe
Mentor: Akshay Mittal

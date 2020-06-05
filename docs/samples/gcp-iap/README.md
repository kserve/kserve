## KFServing GCP/IAP Example 
When using Kubeflow with GCP it is common to use a [GCP Identity Aware Proxy](https://cloud.google.com/iap) (IAP) to manage client authentication to the KFServing endpoints.  The proxy intercepts and authenticates users and passes identity assertion (JWT) to kubernetes service/pods.  Whilst it is also possible to add access control (i.e. programmable or service mesh authorization), this is not described here.

### Prerequisites
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#deploy-kfserving) and have applied the [knative istio probe fix](https://github.com/kubeflow/manifests/commit/928cf483361730121ac18bc4d0e7a9c129f15ee2) (see below).
2. Your gcloud config is initialised to the project containing the k8s cluster and has a service-account that can download IAP key file.
3. You are using Knative serving v0.11.2 or v0.14.0+
4. You are using a recent version of KFServing (v0.3+)

To ensure your cluster has the **knative istio probe fix**, you can use [kfctl_gcp_iap.v1.0.2.yaml](https://raw.githubusercontent.com/kubeflow/manifests/v1.0-branch/kfdef/kfctl_gcp_iap.v1.0.2.yaml), editing the repos section in the yaml before deploying to your Kubernetes cluster.
```
    repos:
    - name: manifests
+     uri: https://github.com/kubeflow/manifests/archive/master.tar.gz
-     uri: https://github.com/kubeflow/manifests/archive/v1.0.2.tar.gz
-   version: v1.0.2
```

Once IAP is enabled and configured, your k8s cluster should have a GCP external http loadbalancer protected by IAP.

### Overview
This example shows how the [sklearn iris sample](https://github.com/kubeflow/kfserving/tree/master/docs/samples/sklearn) can be adapted to support GCP IAP.  We will use a pre-trained model that is hosted on a public gcs bucket.  The model predicts a binary classificaton of iris specie using 4 numerical features.

In this example we will:
 1. **Enable GCP IAP**
 1. **Change firewall rules (for private k8s clusters)**
 1. **Create the inference service**
 1. **Test the private predict endpoint (using port-forwarding)**
 1. **Expose the inference service externally using an additional Istio Virtual Service**
 1. **Test the external predict endpoint (using iap_request.py)**

Step 5. will become unnecessary when this [kfserving issue](https://github.com/kubeflow/kfserving/issues/824) is resolved.

### Enable GCP IAP for k8s
You have followed the steps to [Set up OAuth for Cloud IAP](https://www.kubeflow.org/docs/gke/deploy/oauth-setup/)

We assume that you have created Kubeflow profiles and that [GCP workload identity](https://www.kubeflow.org/docs/gke/authentication/) is configured.

This may not be essential for this example, but is recommended as it maps GCP service-accounts (understood by the IAP) to kubernetes service-accounts (understood by kubernetes and istio RBAC).

### Change firewall rules (for private k8s clusters) 
If you are running Kubeflow in a [private GKE cluster](https://cloud.google.com/kubernetes-engine/docs/how-to/private-clusters) it will be configured with a restrictive firewall.  
You will need to ensure that the k8s master node(s) can talk to the following k8s worker node(s) ports: 
 - 6443 (for cert-manager) 
 - 8443 (for kfserving webhook)

To create these rules firewall rules, modify and run:

```
gcloud compute firewall-rules create kubeflow-webhook-probe --allow=tcp:8443 --target-tags kubeflow-worker --direction INGRESS --network default --priority 1000 --source-ranges 172.16.0.0/28

gcloud compute firewall-rules create kubeflow-cert-manager --allow=tcp:6443 --target-tags kubeflow-worker --direction INGRESS --network default --priority 1000 --source-ranges 172.16.0.0/28

```
Be careful to check whether the **source-ranges**, **target-tags** and **network** are suitable for your environment.  e.g. it assumes your worker nodes have been tagged with `kubeflow-worker` tags.


### Create inference service

To deploy the inference service apply the inferenceservice CRD:
```
kubectl apply -f sklearn-iap-no-authz.yaml
```

Expected Output
```
$ inferenceservice.serving.kubeflow.org/sklearn-iap created
```

When the **KFServing Controller** detects the new InferenceService it creates a **KNative Service**, in turn [Knative Serving](https://knative.dev/docs/serving/) creates a configuration, revision and route.  

See the [debug guide](https://github.com/kubeflow/kfserving/blob/master/docs/KFSERVING_DEBUG_GUIDE.md) for inferenceservice deployment issues.

**Warning:** The [sklearn-iap-no-authz.yaml](./sklearn-iap-no-authz.yaml) has an annotation that prevents the istio sidecar from being injected and thus disables istio RBAC authorization.  This is unlikely to be suitable for production.

We can't yet test the inferenceservice predict endpoint externally because GCP IAP only supports path-based routing but KFServing has exposed a host-based routing url.  

### Expose the inference service externally using an additional Istio Virtual Service

Until this [Issue/824](https://github.com/kubeflow/kfserving/issues/824) is resolved it will be necessary to manually create an additional istio virtual-service.

The service will match on a path-based route (required by IAP) such as:
```https://<Ingress_DNS>/kfserving/<namespace>/sklearn-iap:predict```
and will forward to cluster-local-gateway whilst rewriting host and uri.  The uri is then a host based route as expected by kfserving:
```https://sklearn-iap-predictor-default.<namespace>.svc.cluster.local/v1/models/sklearn-iap:predict```

To create the Istio virtual service:
```
kubectl apply -f virtual-service.yaml
```

Expected Output
```
$ VirtualService/kfserving-iap created
```
This will be deployed to the namespace in the current kube config context.

### Test the external predict endpoint (using iap_request.py)

Perhaps the easiest way to test the inference service's external predict endpoint is by using the [iap_request.py](https://github.com/kubeflow/kubeflow/blob/master/docs/gke/iap_request.py) script described in the [TensorFlow Serving](https://www.kubeflow.org/docs/components/serving/tfserving_new/) documentation.  

The [make-prediction.sh](./make-prediction.sh) explains some of the parameters required by iap_request.py


## Adding Authorization
The steps above authenticate but don't authorize.  You may wish to alter the inference service to enable service mesh authorization
```
kubectl apply -f sklearn-iap-with-authz.yaml
```
The inference service pod will have an additional container called istio-proxy.  Making requests to the service may be blocked (403) by the new istio sidecar container until a new AuthorizationPolicy is added that allows access to this inference URI from a specified ServiceAccount or Namespace.

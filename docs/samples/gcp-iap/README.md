## KFServing GCP/IAP Example 
When using Kubeflow with GCP it is common to use a [GCP Identity Aware Proxy](https://cloud.google.com/iap) (IAP) to manage client authentication to the KFServing endpoints.  The proxy intercepts and authenticates users and passes identity assertion (JWT) to kubernetes service/pods.  Whilst it is also possible to add access control (i.e. programmable or service mesh authorization), this is not described here.

### Prerequisites

The following prerequisites will be met if you follow [Full Kubeflow on Google Cloud](https://www.kubeflow.org/docs/distributions/gke/deploy/overview/) deployment.

1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your gcloud config is initialised to the project containing the k8s cluster and has a service-account that can download IAP key file.
3. You are using KNative serving v0.22.0+
4. You are using KFServing v0.3+ and v0.7-. (KFserving has renamed to KServe in v0.7, and will use `serving.kserve.io` API guide)

Once IAP is enabled and configured, your k8s cluster should have a GCP external http loadbalancer protected by IAP.

### Overview
This example shows how the [sklearn iris sample](https://github.com/kserve/kserve/tree/master/docs/samples/v1beta1/sklearn) can be adapted to support GCP IAP.  We will use a pre-trained model that is hosted on a public gcs bucket.  The model predicts a binary classification of iris specie using 4 numerical features.

In this example we will:
 1. **Enable GCP IAP**
 1. **Change firewall rules (for private k8s clusters)**
 1. **Create the inference service**
 1. **Test the private predict endpoint (using port-forwarding)**
 1. **Expose the inference service externally using an additional Istio Virtual Service**
 1. **Test the external predict endpoint without authorization (using iap_request.py)**
 1. **Test the external predict endpoint with authorization (using iap_request_auth.py)**

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

To deploy the inference service, set the `<namespace>` field with your user profile's namespace in sklearn-iap-no-authz.yaml file.

Apply the inferenceservice CRD:
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
and will forward to knative-local-gateway whilst rewriting host and uri.  The uri is then a host based route as expected by kfserving:
```https://sklearn-iap-predictor-default.<namespace>.svc.cluster.local/v1/models/sklearn-iap:predict```

To create the Istio virtual service, edit [virtual-service.yaml](./virtual-service.yaml) to replace all appearances of `<namespace>` with your user profile' namespace. Then run command:

```
kubectl apply -f virtual-service.yaml
```

Expected Output
```
$ VirtualService/kfserving-iap created
```


### Test the external predict endpoint (using iap_request.py)

Perhaps the easiest way to test the inference service's external predict endpoint is by using the [iap_request.py](https://github.com/kubeflow/kubeflow/blob/master/docs/gke/iap_request.py) script described in the [TensorFlow Serving: Sending prediction request through ingress and IAP](https://www.kubeflow.org/docs/external-add-ons/serving/tfserving_new/#sending-prediction-request-through-ingress-and-iap/) documentation.  

Create a new service account or use existing service account like `<kubeflow-cluster-name>-admin@<project>.iam.gserviceaccount.com`.
If you need to create a new service account, run command:

```
gcloud iam service-accounts create --project=$PROJECT $SERVICE_ACCOUNT
```

Grant the service account access to IAP enabled resource

```
gcloud projects add-iam-policy-binding $PROJECT \
 --role roles/iap.httpsResourceAccessor \
 --member serviceAccount:$SERVICE_ACCOUNT
```

Follow the instruction in [make-prediction.sh](./make-prediction.sh) to download service account key [key.json] and [iap_request.py](https://github.com/kubeflow/kubeflow/blob/master/docs/gke/iap_request.py) script to current folder. Continue with the instruction to set parameters in [make-prediction.sh](./make-prediction.sh). This file explains the parameters required by iap_request.py. Finally, run command:

```
bash make-prediction.sh
```

If request is successful, you can see response like:

```
{"predictions": [1, 1]}
```


## Test the external predict endpoint with authorization (using iap_request_auth.py)

The steps above authenticate but don't authorize.  You may wish to alter the inference service to enable service mesh authorization.

Set the `<namespace>` field with your user profile's namespace in sklearn-iap-with-authz.yaml file.

Run command:
```
kubectl apply -f sklearn-iap-with-authz.yaml
```

The inference service pod will have an additional container called istio-proxy.  Making requests to the service may be blocked (403) by the new istio sidecar container until a new AuthorizationPolicy is added that allows access to this inference URI from a specified ServiceAccount or Namespace.

Therefore, running `bash make-prediction.sh` will throw the following exception:

```
Exception: Service account <service-account>@<project>.iam.gserviceaccount.com does not have permission to access the IAP-protected application.
```


Below is an example of accessing inference service from outside of cluster using user account when authorization is enabled. For more guide on authenticating with IAP, refer to [Programmatic Authentication](https://cloud.google.com/iap/docs/authentication-howto).


Set the parameters in [`make-prediction-auth.sh`](./make-prediction-auth.sh), pair the instruction in bash script and [Authenticating from a desktop app](https://cloud.google.com/iap/docs/authentication-howto#authenticating_from_a_desktop_app) to understand the process. This bash script calls [iap_request_auth.py](./iap_request_auth.py), which is an altered version of authentication request script from [KFP SDK auth script](https://github.com/kubeflow/pipelines/blob/master/sdk/python/kfp/_auth.py).

Make request to sidecar injected inference service using command:

```
bash make-prediction-auth.sh
```

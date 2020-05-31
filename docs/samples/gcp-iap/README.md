## KFServing GCP/IAP Example 
When using Kubeflow with GCP it is common to use a [GCP Identity Aware Proxy](https://cloud.google.com/iap) (IAP) to manage client authentication to the KFServing endpoints.  The proxy intercepts and authenticates users and passes identity assertion (JWT) to kubernetes service/pods.  Whilst it is also possible to add access control (i.e. programmable or service mesh authorization), this is not described here.

### Prerequisites
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#deploy-kfserving).
2. Your gcloud config is initialised to the project containing the k8s cluster and has a service-account that can download IAP key file.
2. Your k8s cluster has a GCP external http loadbalancer (i.e. you've configured an ingress gateway).
3. You are using a fairly recent version of istio (istio-networking is v0.11.2+ )
4. You are using a recent version of KFServing (v0.3+)

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
 - 8443 (for probes such as kfserving)

To create these rules firewall rules, modify and run:

```
gcloud compute firewall-rules create kubeflow-webhook-probe --allow=tcp:8443 --target-tags kubeflow-worker --direction INGRESS --network default --priority 1000 --source-ranges 172.16.0.0/28

gcloud compute firewall-rules create kubeflow-cert-manager --allow=tcp:8443 --target-tags kubeflow-worker --direction INGRESS --network default --priority 1000 --source-ranges 172.16.0.0/28

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

As well as the InferenceService, the kfserving webhook typically creates a Deployment, Replicaset, Pod(s) + Knative Service, Route, Configuration and Revision!

See the [debug guide](https://github.com/kubeflow/kfserving/blob/master/docs/KFSERVING_DEBUG_GUIDE.md) for inferenceservice deployment issues.

**Warning:** The [sklearn-iap-no-authz.yaml](./sklearn-iap-no-authz.yaml) has an annotation that prevents the istio sidecar from being injected and thus disables istio RBAC authorization.  This is unlikely to be suitable for production.

### Test the private predict endpoint (using port-forwarding)
We can't yet test the inferenceservice predict endpoint externally because GCP IAP only supports path-based routing but KFServing has exposed a host-based routing url.  We can, however, test the private (internal) service using port forwarding.

We can use kubectl port-forward (bypassing IAP) to tunnel to the private service:
```
# terminal 1
kubectl port-forward svc/sklearn-iap-predictor-default-<guid>-private 8080:80
```
And then make a request through the tunnel:
```
# terminal 2
curl -v -H "Content-Type: application/json" http://localhost:8080/v1/models/sklearn-iris:predict -d @iris-input.json
```

Expected Output

```
*   Trying 127.0.0.1:8080...
* TCP_NODELAY set
* Connected to 127.0.0.1:8080 (127.0.0.1:8080) port 80 (#0)
> POST /models/sklearn-iris:predict HTTP/1.1
> Host: localhost
> User-Agent: curl/7.60.0
> Accept: */*
> Content-Length: 76
> Content-Type: application/x-www-form-urlencoded
>
* upload completely sent off: 76 out of 76 bytes
< HTTP/1.1 200 OK
< content-length: 23
< content-type: application/json; charset=UTF-8
< date: Mon, 20 May 2019 20:49:02 GMT
< server: istio-envoy
< x-envoy-upstream-service-time: 1943
<
* Connection #0 to host 127.0.0.1 left intact
{"predictions": [1, 1]}
```


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

### Test the external predict endpoint (using iap_request.py)

Perhaps the easiest way to test the inference service's external predict endpoint is by using the [iap_request.py](https://github.com/kubeflow/kubeflow/blob/master/docs/gke/iap_request.py) script described in the [TensorFlow Serving](https://www.kubeflow.org/docs/components/serving/tfserving_new/) documentation.  

The [make-prediction.sh](./make-prediction.sh) explains some of the parameters required by iap_request.py


## Adding Authorization
The steps above authenticate but don't authorize.  You may wish to alter the inference service to enable service mesh authorization
```
kubectl apply -f sklearn-iap-with-authz.yaml
```
The inference service pod will have an additional container called istio-proxy.  Making requests to the service may be blocked (403) by the new istio sidecar container until a new AuthorizationPolicy is added that allows access to this inference URI from a specified ServiceAccount or Namespace.
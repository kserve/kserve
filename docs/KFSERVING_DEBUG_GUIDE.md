# Debug KFServing InferenceService Status
You deployed an InferenceService to KFServing, but it is not in ready state. Go through this step by step guide to understand what failed :heart:.
```bash
kubectl get inferenceservices sklearn-iris 
NAME                  URL   READY   DEFAULT TRAFFIC   CANARY TRAFFIC   AGE
model-example               False                                      1m
```

## Check Service Status
KFServing `InferenceService` creates [KNative Service](https://knative.dev/docs/serving/spec/knative-api-specification-1.0/#service) under the hood to instantiate a 
serverless container.

If you see `IngressNotConfigured` error, this indicates `Istio Ingress Gateway` probes are failing and you can check KNative `networking-istio` pod logs for more details.

```bash
kubectl get ksvc
NAME                             URL                                                            LATESTCREATED                          LATESTREADY                            READY     REASON
sklearn-iris-predictor-default   http://sklearn-iris-predictor-default.default.example.com   sklearn-iris-predictor-default-jk794   mnist-sample-predictor-default-jk794   Unknown   IngressNotConfigured
```

## Check Revision Status
If you see `RevisionMissing` error, then your service pods are not in ready state. `Knative Service` creates [KNative Revision](https://knative.dev/docs/serving/spec/knative-api-specification-1.0/#revision) 
which represents a snapshot of the `InferenceService` code and configuration.


### Storage Initializer fails to download model
```bash
kubectl get revision $(kubectl get configuration sklearn-iris-predictor-default --output jsonpath="{.status.latestCreatedRevisionName}") 
NAME                                   CONFIG NAME                      K8S SERVICE NAME                       GENERATION   READY     REASON
sklearn-iris-predictor-default-csjpw   sklearn-iris-predictor-default   sklearn-iris-predictor-default-csjpw   2            Unknown   Deploying
```

If you see `READY` status in `Unknown` error, this usually indicates that the KFServing `Storage Initializer` init container fails to download the model and you can
check the init container logs to see why it fails, **note that the pod scales down after sometime if the init container fails**. 
```bash
kubectl get pod -l model=sklearn-iris
NAME                                                              READY   STATUS       RESTARTS   AGE
sklearn-iris-predictor-default-29jks-deployment-5f7d4b9996hzrnc   0/3     Init:Error   1          10s

kubectl logs -l model=sklearn-iris -c storage-initializer
[I 200517 03:56:19 initializer-entrypoint:13] Initializing, args: src_uri [gs://kfserving-samples/models/sklearn/iris-1] dest_path[ [/mnt/models]
[I 200517 03:56:19 storage:35] Copying contents of gs://kfserving-samples/models/sklearn/iris-1 to local
Traceback (most recent call last):
  File "/storage-initializer/scripts/initializer-entrypoint", line 14, in <module>
    kfserving.Storage.download(src_uri, dest_path)
  File "/usr/local/lib/python3.7/site-packages/kfserving/storage.py", line 48, in download
    Storage._download_gcs(uri, out_dir)
  File "/usr/local/lib/python3.7/site-packages/kfserving/storage.py", line 116, in _download_gcs
    The path or model %s does not exist." % (uri))
RuntimeError: Failed to fetch model. The path or model gs://kfserving-samples/models/sklearn/iris-1 does not exist.
[I 200517 03:40:19 initializer-entrypoint:13] Initializing, args: src_uri [gs://kfserving-samples/models/sklearn/iris] dest_path[ [/mnt/models]
[I 200517 03:40:19 storage:35] Copying contents of gs://kfserving-samples/models/sklearn/iris to local
[I 200517 03:40:20 storage:111] Downloading: /mnt/models/model.joblib
[I 200517 03:40:20 storage:60] Successfully copied gs://kfserving-samples/models/sklearn/iris to /mnt/models
```

### Inference Service in OOM status
If you see revision fail reason `ExitCode137`, this usually indicates that the inference service pod is out of memory and you might need to bump up the
memory limit of the `InferenceService`.
```bash
kubectl get revision $(kubectl get configuration sklearn-iris-predictor-default --output jsonpath="{.status.latestCreatedRevisionName}") 
NAME                                   CONFIG NAME                      K8S SERVICE NAME                       GENERATION   READY   REASON
sklearn-iris-predictor-default-84bzf   sklearn-iris-predictor-default   sklearn-iris-predictor-default-84bzf   8            False   ExitCode137s
```

### Inference Service fails to start
If you see other exit codes from the revision status you can further check the pod status.
```bash
kubectl get pods -l model=sklearn-iris
sklearn-iris-predictor-default-rvhmk-deployment-867c6444647tz7n   1/3     CrashLoopBackOff        3          80s
```

If you see the `CrashLoopBackOff`, then check the `kfserving-container` log to see more details where it fails, the error log is usually propagated on revision container status also.
```bash
kubectl logs sklearn-iris-predictor-default-rvhmk-deployment-867c6444647tz7n  kfserving-container
[I 200517 04:58:21 storage:35] Copying contents of /mnt/models to local
Traceback (most recent call last):
  File "/usr/local/lib/python3.7/runpy.py", line 193, in _run_module_as_main
    "__main__", mod_spec)
  File "/usr/local/lib/python3.7/runpy.py", line 85, in _run_code
    exec(code, run_globals)
  File "/sklearnserver/sklearnserver/__main__.py", line 33, in <module>
    model.load()
  File "/sklearnserver/sklearnserver/model.py", line 36, in load
    model_file = next(path for path in paths if os.path.exists(path))
StopIteration
```

## Debug KFServing Request flow

```
  +----------------------+        +-----------------------+      +--------------------------+
  |Istio Virtual Service |        |Istio Virtual Service  |      | K8S Service              |
  |                      |        |                       |      |                          |
  |sklearn-iris          |        |sklearn-iris-predictor |      | sklearn-iris-predictor   |
  |                      +------->|  -default             +----->|   -default-$revision     |
  |                      |        |                       |      |                          |
  |KFServing Route       |        |Knative Route          |      | Knative Revision Service |
  +----------------------+        +-----------------------+      +------------+-------------+
   Istio Ingress Gateway           Istio Local Gateway                    Kube Proxy
                                                                              |
                                                                              |
                                                                              |
  +-------------------------------------------------------+                   |
  |  Knative Revision Pod                                 |                   |
  |                                                       |                   |
  |  +-------------------+      +-----------------+       |                   |
  |  |                   |      |                 |       |                   |
  |  |kfserving-container|<-----+ Queue Proxy     |       |<------------------+
  |  |                   |      |                 |       |
  |  +-------------------+      +-----------------+       |
  |                                                       |
  +-----------------------^-------------------------------+
                          | scale deployment
                 +--------+--------+
                 |  Knative        |
                 |  Autoscaler     |
                 |  KPA/HPA        |
                 +-----------------+
```
1. Traffic arrive though:
   - The `Istio Ingress Gateway` for external traffic
   - The `Istio Cluster Local Gateway` for internal traffic
   
`Istio Gateway` describes the edge of the mesh receiving incoming or outgoing HTTP/TCP connections. The specification describes a set of ports
that should be exposed and the type of protocol to use. If you are using `Standalone KFServing`, it uses the `Gateway` in `knative-serving` namespace,
if you are using `Kubeflow KFServing`, it uses the `Gateway` in `kubeflow` namespace e.g on GCP the gateway is protected behind `IAP` with [Istio 
authentication policy](https://istio.io/docs/tasks/security/authentication/authn-policy).
```bash
kubectl get vs sklearn-iris -oyaml
```
```yaml
kind: Gateway
metadata:
  labels:
    networking.knative.dev/ingress-provider: istio
    serving.knative.dev/release: v0.12.1
  name: knative-ingress-gateway
  namespace: knative-serving
spec:
  selector:
    istio: ingressgateway
  servers:
  - hosts:
    - '*'
    port:
      name: http
      number: 80
      protocol: HTTP
  - hosts:
    - '*'
    port:
      name: https
      number: 443
      protocol: HTTPS
    tls:
      mode: SIMPLE
      privateKey: /etc/istio/ingressgateway-certs/tls.key
      serverCertificate: /etc/istio/ingressgateway-certs/tls.crt
```
The `InferenceService` request hitting the `Istio Ingress Gateway` first matches the network port, by default http is configured. You can [configure
HTTPS with TLS certificates](https://knative.dev/docs/serving/using-a-tls-cert).
 
2. KFServing creates a `Istio virtual service` to specify routing rule for predictor, transformer, explainer and canary
```yaml
apiVersion: networking.istio.io/v1alpha3
kind: VirtualService
metadata:
  name: sklearn-iris
  namespace: default
spec:
  gateways:
  - knative-ingress-gateway.knative-serving
  - knative-serving/cluster-local-gateway
  hosts:
  - sklearn-iris.default.example.com
  - sklearn-iris.default.svc.cluster.local
  http:
  - match:
    - authority:
        regex: ^sklearn-iris.default\.default\.example\.com(?::\d{1,5})?$
      gateways:
      - knative-ingress-gateway.knative-serving
      uri:
        prefix: /v1/models/sklearn-iris:predict
    - authority:
        regex: ^sklearn-iris\.default(\.svc(\.cluster\.local)?)?(?::\d{1,5})?$
      gateways:
      - knative-serving/cluster-local-gateway
      uri:
        prefix: /v1/models/sklearn-iris:predict
    retries:
      attempts: 3
      perTryTimeout: 600s
    route:
    - destination:
        host: cluster-local-gateway.istio-system.svc.cluster.local
        port:
          number: 80
      headers:
        request:
          set:
            Host: sklearn-iris-predictor-default.default.svc.cluster.local
      weight: 100
```
- KFServing creates the routing rule based on uri prefix according to [KFServing V1 data plane](./README.md#data-plane-v1) and traffic
is forwarded to `KFServing Predictor` if you only have `Predictor` specified on `InferenceService`,
note that if you have custom container and the endpoint is not conforming to the protocol you get `HTTP 404` when you hit the KFServing
top level virtual service.
- When `Transformer` and `Explainer` are specified on `InferenceService` the routing rule sends the traffic to `Transformer`
or `Explainer` based on the verb.
- The top level virtual service also does `Canary Traffic Split` if canary is specified on `InferenceService`.

3. KNative creates a `Istio virtual service` to configure the gateway to route the user traffic to correct revision
The request then hits `Knative` created virtual service via local gateway, it matches with the in cluster host name.
```yaml
apiVersion: networking.istio.io/v1alpha3
kind: VirtualService
metadata:
  name: sklearn-iris-predictor-default-mesh
  namespace: default
spec:
  gateways:
  - mesh
  hosts:
  - sklearn-iris-predictor-default.default
  - sklearn-iris-predictor-default.default.svc
  - sklearn-iris-predictor-default.default.svc.cluster.local
  http:
  - headers:
      request:
        set:
          K-Network-Hash: dee002f4a2db24e3827d8088b7ddacf3
    match:
    - authority:
        prefix: sklearn-iris-predictor-default.default
      gateways:
      - mesh
    retries:
      attempts: 3
      perTryTimeout: 600s
      retryOn: 5xx,connect-failure,refused-stream,cancelled,resource-exhausted,retriable-status-codes
    route:
    - destination:
        host: sklearn-iris-predictor-default-fhmjk.default.svc.cluster.local
        port:
          number: 80
      headers:
        request:
          set:
            Knative-Serving-Namespace: default
            Knative-Serving-Revision: sklearn-iris-predictor-default-fhmjk
      weight: 100
    timeout: 600s
    websocketUpgrade: true
```
The destination here is the `k8s Service` for the latest ready `Knative Revision` and it is reconciled by `Knative` every time
user rolls out a new revision. When a new revision is rolled out and in ready state, the old revision is then scaled down, after
configured revision GC time the revision resource is garbage collected if the revision no longer has traffic referenced.

4. Once the revision pods are ready, the `Kubernetes Service` sends the requests to the `queue proxy` sidecar on `port 8012`.
```yaml
apiVersion: v1
kind: Service
metadata:
  name: sklearn-iris-predictor-default-fhmjk-private
  namespace: default
spec:
  clusterIP: 10.105.186.18
  ports:
  - name: http
    port: 80
    protocol: TCP
    targetPort: 8012
  - name: queue-metrics
    port: 9090
    protocol: TCP
    targetPort: queue-metrics
  - name: http-usermetric
    port: 9091
    protocol: TCP
    targetPort: http-usermetric
  - name: http-queueadm
    port: 8022
    protocol: TCP
    targetPort: 8022
  selector:
    serving.knative.dev/revisionUID: a8f1eafc-3c64-4930-9a01-359f3235333a
  sessionAffinity: None
  type: ClusterIP

```
5. The `queue proxy` sends single or multi-threaded requests that the `kfserving container` can handle at a time.
If the `queue proxy` has more requests than it can handle, the autoscaler creates more pods to handle additional requests.

6. Finally The `queue proxy` sends traffic to the `kfserving-container`.


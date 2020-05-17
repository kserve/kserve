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
   - The Ingress Gateway for external traffic
   - The Cluster Local Gateway for internal traffic
2. KFServing creates a Istio virtual service to specify routing rule for predictor, transformer, explainer and canary
3. KNative creates a Istio virtual service to configure the gateway to route the user traffic to correct revision
4. If the revision pods are ready, the kubernetes service sends the requests to the queue proxy sidecar.
5. The queue proxy sends single or multi-threaded requests that the KFServing container can handle at a time.
6. If the queue proxy has more requests than it can handle, the autoscaler creates more pods to handle additional requests.
7. The queue proxy sends traffic to the `kfserving-container`   

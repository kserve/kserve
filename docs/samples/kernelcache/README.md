# Setup for KernelCache

This directory includes setup scripts for using KernelCache.
Scripts are provided for both Minikube and OpenShift. The  setup for Minikube is already covered by [GitHub Actions Script](/home/jlee/temp/20260522_gkm/kserve/test/scripts/gh-actions/setup-kernelcache.sh), so you can use that flow when working with Minikube.
The documentation here focuses on setups that use ServiceAccount tokens. OpenShift is used as the example platform because it is a common environment where ServiceAccount token-based authentication is required.


## Install KServe + LocalModel(Kernel Cache)
```
echo "RUN: Install KServe + LocalModel"
DEPLOYMENT_MODE=Standard ENABLE_KSERVE=true ENABLE_LLMISVC=false ENABLE_LOCALMODEL=true hack/setup/infra/manage.kserve-kustomize.sh
```

## Build/push new kernelcache images and update inferenceservice-config

```
docs/samples/kernelcache/setup_script_for_ocp.sh
```

* Check kcng/kcn
```
kubectl get kcng,kcn -n kserve
```

## Test

* Deploy isvc
```
kubectl create ns kc-test
kubectl create -f ./docs/samples/kernelcache/vllm-servingruntime.yaml -n kc-test
kubectl create -f ./docs/samples/kernelcache/isvc.yaml -n kc-test
kubectl get pod
NAME                                           READY   STATUS    RESTARTS   AGE
opt-125m-no-cache-predictor-566d8dc4bf-lhh2b   1/2     Running   0          39s

opt-125m-no-cache-predictor-566d8dc4bf-lhh2b   2/2     Running   0          93s

```
The pod has two containers: vLLM and the MCV sidecar, which creates the OCI image used to capture the KernelCache.

* Check KCC/KC 
```
kubectl get kcc,kc,kcn -n kc-test
```
* Example output
```
NAME                                                                    PHASE      IMAGE                                                                                                                                                                AGE
kernelcachecapture.serving.kserve.io/opt-125m-no-cache-kcc-566d8dc4bf   Complete   image-registry.openshift-image-registry.svc:5000/jooho-test/kernel-cache-opt-125m-no-cache@sha256:095b7ef93fe67688a296842c83022e2a66edaec79ac9d1cb5986800963db287d   2m58s

NAME                                                                                      STATE   MOUNTTYPE   PODS-USING   AGE
kernelcache.serving.kserve.io/opt-125m-no-cache-kcc-566d8dc4bf-dad34c2cfef25053288c9303   Ready   oci                      88s

NAME                                                                           READY   PREPARING   ERROR   PODS-USING   AGE
kernelcachenode.serving.kserve.io/ip-10-0-115-143.us-west-2.compute.internal   1       0           0       0            13m
kernelcachenode.serving.kserve.io/ip-10-0-124-82.us-west-2.compute.internal    1       0           0       0            13m
```

* Check signing part for oci image
```
kubectl get kcc -oyaml
...
    signing:
      message: artifact signing completed
      mode: cert
      reason: SigningSucceeded
      signed: true
      signedAt: "2026-10-08T00:04:50Z"
      state: Succeeded
```

* Check verifying part for oci image
```
kubectl get kc -oyaml
    verification:
      message: artifact verification completed
      mode: cert
      reason: VerificationSucceeded
      state: Succeeded
      verified: true
      verifiedAt: "2026-10-08T00:04:50Z"
```

* Restart pod to see if it is using kernel cache 
```
kubectl delete pod --force --all -n kc-test

kubectl get pod 
NAME                                           READY   STATUS     RESTARTS   AGE
opt-125m-no-cache-predictor-566d8dc4bf-fg5xp   0/1     Init:1/2   0          3s

opt-125m-no-cache-predictor-566d8dc4bf-fg5xp   1/1     Running   0          60s
```
No sidecar was injected because the KernelCache was found. The workload became Ready in 60 seconds, compared with 93 seconds previously.


## Clean up

```
DEPLOYMENT_MODE=Standard ENABLE_KSERVE=true ENABLE_LLMISVC=false ENABLE_LOCALMODEL=true hack/setup/infra/manage.kserve-kustomize.sh --uninstall
```
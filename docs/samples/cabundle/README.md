# KServe with Self Signed Certificate Model Registry

If you are using a model registry with a self-signed certificate, you must either skip ssl verify or apply the appropriate cabundle to the storage-initializer to create a connection with the registry.
This document explains three methods that can be used in KServe.

- Configure CaBundle for storage-initializer
  - Global configuration
  - Using `storage-config` Secret

- Skip SSL Verification
  
## Configure CaBundle for storage-initializer  
### Global Configuration

KServe use `inferenceservice-config` ConfigMap for default configuration. If you want to add `cabundle` cert for every inferenceservice, you can set `caBundleConfigMapName` in the ConfigMap. Before updating the ConfigMap, you have to create a configmap for cabundle certificate in the namespace that KServe controller is running and the data key in the ConfigMap must be cabundle.crt. 

- Create a ConfigMap with the cabundle cert
  ~~~
  kubectl create configmap cabundle --from-file=/path/to/cabundle.crt

  kubectl get configmap cabundle -o yaml
  apiVersion: v1
  data:
    cabundle.crt: XXXXX
  kind: ConfigMap
  metadata:
    name: cabundle
    namespace: kserve
  ~~~
- Update `inferenceservice-config` ConfigMap 
  ~~~
    storageInitializer: |-
    {
        ...
        "caBundleConfigMapName": "cabundle",
        ...
    }
  ~~~
  
If you update this configuration after, please restart KServe controller pod.  

### Using storage-config Secret

If you want to apply the cabundle only to a specific inferenceservice, you can use a specific annotation on the `storage-config` Secret used by the inferenceservice.
In this case, you have to create the cabundle ConfigMap in the user namespace before you create the inferenceservice.


- Create a Secret with the cabundle cert
  ~~~
  kubectl create configmap local-cabundle --from-file=/path/to/cabundle.crt

  kubectl get configmap cabundle -o yaml
  apiVersion: v1
  data:
    cabundle.crt: XXXXX
  kind: ConfigMap
  metadata:
    name: local-cabundle
    namespace: kserve
  ~~~

- Add an annotation to `storage-config` Secret
  ~~~
  apiVersion: v1
  data:
    AWS_ACCESS_KEY_ID: VEhFQUNDRVNTS0VZ
    AWS_SECRET_ACCESS_KEY: VEhFUEFTU1dPUkQ=
  kind: Secret
  metadata:
    annotations:
      serving.kserve.io/s3-cabundle-configmap: local-cabundle
      ...
    name: storage-config
    namespace: kserve-demo
  type: Opaque
  ~~~


## Skip SSL Verification

For testing purposes or when there is no cabundle, you can easily create an SSL connection by disabling SSL verification.
This can also be used by adding an annotation in `secret-config` Secret.

- Add an annotation to `storage-config` Secret
  ~~~
  apiVersion: v1
  data:
    AWS_ACCESS_KEY_ID: VEhFQUNDRVNTS0VZ
    AWS_SECRET_ACCESS_KEY: VEhFUEFTU1dPUkQ=
  kind: Secret
  metadata:
    annotations:
         serving.kserve.io/s3-verifyssl: "0" # 0 is true, 1 is false
      ...
    name: storage-config
    namespace: kserve-demo
  type: Opaque
  ~~~

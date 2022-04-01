# Predict on a InferenceService with saved model on Ali-cloud OSS

## Using OSS

Kserve uses [aliyun-oss-python-sdk](https://github.com/aliyun/aliyun-oss-python-sdk) client to download artifacts.

### Create a K8s Secret

Store your OSS secrets as a k8s secret.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: osscreds
type: Opaque
data:
  OSS_ACCESS_KEY_ID: xxxx
  OSS_ACCESS_KEY_SECRET: xxxx
  OSS_ENDPOINT: xxxx
  OSS_REGION: xxxx
```

### Attach to Service Account

`KServe` gets the secrets from your service account, you need to add the above created or existing secret to your service account's secret list.
By default `Kserve` uses `default` service account, user can use own service account and overwrite on `InferenceService` CRD.

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: sa
secrets:
  - name: osscreds
```

Save the YAMLs and apply them to your cluster:

```bash
kubectl apply -f osscreds.yaml
```

## Create the InferenceService

Create the InferenceService with the oss `storageUri` and the service account with oss credential attached.

```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "mnist-oss"
spec:
  predictor:
    serviceAccountName: sa
    tensorflow:
      storageUri: "oss://<bucket_name>/mnist"
```

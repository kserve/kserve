
# Predict on a KFService with saved model on Azure
## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#deploy-kfserving).
2. Your cluster's Istio Ingress gateway must be network accessible.
3. Your cluster's Istio Egresss gateway must [allow accessing Azure Storage](https://knative.dev/docs/serving/outbound-network-access/)


## Create Azure Secret and attach to Service Account
Accessing azure storage requires the creation of a secret named "azcreds". 
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: azceds
type: Opaque
  AZ_CLIENT_ID: xxxxx
  AZ_CLIENT_SECRET: xxxxx
  AZ_SUBSCRIPTION_ID: xxxxx
  AZ_TENANT_ID: xxxxx
```

`KFServing` gets the secrets from your service account, you need to add the above created or existing secret to your service account's secret list. 
By default `KFServing` uses `default` service account, user can use own service account and overwrite on `KFService` CRD.

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: sa
secrets:
- name: azcreds
```

Apply the secret and service account
```bash
kubectl apply -f s3_secret.yaml
```

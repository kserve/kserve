
# Predict on a InferenceService with saved model on Azure
## Using Public Azure Blobs
By default, KServe uses anonymous client to download artifacts. To point to an Azure Blob, specify StorageUri to point to an Azure Blob Storage with the format:
```https://{$STORAGE_ACCOUNT_NAME}.blob.core.windows.net/{$CONTAINER}/{$PATH}```

e.g. https://kserve.blob.core.windows.net/triton/simple_string/

## Using Private Blobs
KServe supports authenticating using an Azure Service Principle.
### Create an authorized Azure Service Principle
* To create an Azure Service Principle follow the steps [here](https://docs.microsoft.com/en-us/cli/azure/create-an-azure-service-principal-azure-cli?view=azure-cli-latest).
* You may also use Azure Managed Identity follow the steps [here](https://docs.microsoft.com/en-us/azure/active-directory/managed-identities-azure-resources/how-manage-user-assigned-managed-identities?pivots=identity-mi-methods-azcli)
* Assign the SP or MI the `Storage Blob Data Owner` role on your blob (KServe needs this permission as it needs to list contents at the blob path to filter items to download).
* Details on assigning storage roles [here](https://docs.microsoft.com/en-us/azure/storage/common/storage-auth-aad).

### Create a K8s Secret
Store your Azure SP secrets as a k8s secret. 

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: azcreds
type: Opaque
data:
  AZURE_CLIENT_ID: xxxxx
  AZURE_CLIENT_SECRET: xxxxx
  AZURE_SUBSCRIPTION_ID: xxxxx
  AZURE_TENANT_ID: xxxxx
```
Note: 
* The azure secret KServe looks for can be configured by running `kubectl edit -n kserving-system inferenceservice-config`
* `AZURE_CLIENT_SECRET` is not required when using Managed Identity

### Attach to Service Account
`KServe` gets the secrets from your service account, you need to add the above created or existing secret to your service account's secret list. 
By default `KServe` uses `default` service account, user can use own service account and overwrite on `InferenceService` CRD.

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: sa
secrets:
- name: azcreds
```

Save the YAMLs and apply them to your cluster:
```bash
kubectl apply -f azcreds.yaml
```

Note: To use your model binary you must reference the folder where it's located with an ending ```/``` to denote it`s a folder.
```bash
https://accountname.blob.core.windows.net/container/models/iris/v1.1/
```

### Use Pod Identity

You may assign Managed Identity to `InferenceService` resource using Pod Identity.

* Set up Pod Identity follow the steps [here](https://azure.github.io/aad-pod-identity/docs/demo/standard_walkthrough/)
* Add the `aadpodidbinding` label to your service 

```yaml
---
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: kserve-simple-string
  labels:
    aadpodidbinding: piselector
spec:
  template:
    metadata:
  predictor:
    serviceAccountName: sa
    tensorflow:
      storageUri: "https://kserve.blob.core.windows.net/triton/simple_string/"
```


# Predict on a InferenceService with saved model on Azure
## Using Public Azure Blobs
By default, KFServing uses anonymous client to download artifacts. To point to an Azure Blob, specify StorageUri to point to an Azure Blob Storage with the format: 
```https://{$STORAGE_ACCOUNT_NAME}.blob.core.windows.net/{$CONTAINER}/{$PATH}```

e.g. https://kfserving.blob.core.windows.net/triton/simple_string/

## Using Private Blobs
KFServing supports authenticating using an Azure Service Principle.
### Create an authorized Azure Service Principle
* To create an Azure Service Principle follow the steps [here](https://docs.microsoft.com/en-us/cli/azure/create-an-azure-service-principal-azure-cli?view=azure-cli-latest).
* Assign the SP the `Storage Blob Data Owner` role on your blob (KFServing needs this permission as it needs to list contents at the blob path to filter items to download).
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
  AZ_CLIENT_ID: xxxxx
  AZ_CLIENT_SECRET: xxxxx
  AZ_SUBSCRIPTION_ID: xxxxx
  AZ_TENANT_ID: xxxxx
```
Note: The azure secret KFServing looks for can be configured by running `kubectl edit -n kfserving-system inferenceservice-config`

### Attach to Service Account
`KFServing` gets the secrets from your service account, you need to add the above created or existing secret to your service account's secret list. 
By default `KFServing` uses `default` service account, user can use own service account and overwrite on `InferenceService` CRD.

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
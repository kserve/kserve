# KFServing K8S constants
KFSERVING_GROUP = "serving.kubeflow.org"
KFSERVING_KIND = "KFService"
KFSERVING_PLURAL = "kfservices"
KFSERVING_VERSION = "v1alpha2"

# KFservice credentials common constants
KFSERVICE_CONFIG_MAP_NAME = 'kfservice-config'
KFSERVICE_SYSTEM_NAMESPACE = 'kfserving-system'
DEFAULT_SECRET_NAME = "kfserving-secret-"
DEFAULT_SA_NAME = "kfserving-service-credentials"

# S3 credentials constants
S3_ACCESS_KEY_ID_DEFAULT_NAME = "awsAccessKeyID"
S3_SECRET_ACCESS_KEY_DEFAULT_NAME = "awsSecretAccessKey"
S3_DEFAULT_CREDS_FILE = '~/.aws/credentials'

# GCS credentials constants
GCS_CREDS_FILE_DEFAULT_NAME = 'gcloud-application-credentials.json'
GCS_DEFAULT_CREDS_FILE = '~/.config/gcloud/application_default_credentials.json'

# Azure credentials constants
AZ_DEFAULT_CREDS_FILE = '~/.azure/azure_credentials.json'

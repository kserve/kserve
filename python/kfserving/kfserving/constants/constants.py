# KFServing K8S constants
KFSERVING_GROUP = "serving.kubeflow.org"
KFSERVING_KIND = "KFService"
KFSERVING_PLURAL = "kfservices"
KFSERVING_VERSION = "v1alpha1"

# KFservice constants
KFSERVICE_CONFIG_MAP_NAME = 'kfservice-config'
KFSERVICE_SYSTEM_NAMESPACE = 'kfserving-system'
DEFAULT_SECRET_NAME = "kfserving-secret-"
DEFAULT_SA_NAME = "kfserving-sa-"

# AWS credentials constants
AWS_ACCESS_KEY_ID = "AWS_ACCESS_KEY_ID"
AWS_SECRET_ACCESS_KEY = "AWS_SECRET_ACCESS_KEY"
AWS_ACCESS_KEY_ID_NAME = "awsAccessKeyID"
AWS_SECRET_ACCESS_KEY_NAME = "awsSecretAccessKey"
AWS_ENDPOINT_URL = "AWS_ENDPOINT_URL"
AWS_REGION = "AWS_REGION"
S3_ENDPOINT = "S3_ENDPOINT"
S3_USE_HTTPS = "S3_USE_HTTPS"
S3_VERIFY_SSL = "S3_VERIFY_SSL"

# GCP credentials constants
GCP_CREDS_ARG_NAME = 'GCP_CREDS_FILE'
GCP_CREDS_FILE_DEFAULT_NAME = 'gcloud-application-credentials.json'

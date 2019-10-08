# Copyright 2019 kubeflow.org.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# KFServing K8S constants
KFSERVING_GROUP = "serving.kubeflow.org"
KFSERVING_KIND = "InferenceService"
KFSERVING_PLURAL = "inferenceservices"
KFSERVING_VERSION = "v1alpha2"

# INFERENCESERVICE credentials common constants
INFERENCESERVICE_CONFIG_MAP_NAME = 'inferenceservice-config'
INFERENCESERVICE_SYSTEM_NAMESPACE = 'kfserving-system'
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

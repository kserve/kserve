# Copyright 2020 kubeflow.org.
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

import os

# KFServing K8S constants
KFSERVING_GROUP = 'serving.kubeflow.org'
KFSERVING_KIND = 'InferenceService'
KFSERVING_PLURAL = 'inferenceservices'
KFSERVING_KIND_TRAINEDMODEL = 'TrainedModel'
KFSERVING_PLURAL_TRAINEDMODEL = 'trainedmodels'
KFSERVING_VERSION = os.environ.get('KFSERVING_VERSION', 'v1alpha2')
KFSERVING_V1BETA1_VERSION = 'v1beta1'
KFSERVING_V1ALPHA2_VERSION = 'v1alpha2'
KFSERVING_V1ALPHA1_VERSION = "v1alpha1"
KFSERVING_API_VERSION = KFSERVING_GROUP + '/' + KFSERVING_VERSION
KFSERVING_V1BETA1 = KFSERVING_GROUP + '/' + KFSERVING_V1BETA1_VERSION
KFSERVING_V1ALPHA2 = KFSERVING_GROUP + '/' + KFSERVING_V1ALPHA2_VERSION
KFSERVING_V1ALPHA1 = KFSERVING_GROUP + '/' + KFSERVING_V1ALPHA1_VERSION

KFSERVING_LOGLEVEL = os.environ.get('KFSERVING_LOGLEVEL', 'INFO').upper()

# INFERENCESERVICE credentials common constants
INFERENCESERVICE_CONFIG_MAP_NAME = 'inferenceservice-config'
INFERENCESERVICE_SYSTEM_NAMESPACE = 'kfserving-system'
DEFAULT_SECRET_NAME = "kfserving-secret-"
DEFAULT_SA_NAME = "kfserving-service-credentials"

# S3 credentials constants
S3_ACCESS_KEY_ID_DEFAULT_NAME = "AWS_ACCESS_KEY_ID"
S3_SECRET_ACCESS_KEY_DEFAULT_NAME = "AWS_SECRET_ACCESS_KEY"
S3_DEFAULT_CREDS_FILE = '~/.aws/credentials'

# GCS credentials constants
GCS_CREDS_FILE_DEFAULT_NAME = 'gcloud-application-credentials.json'
GCS_DEFAULT_CREDS_FILE = '~/.config/gcloud/application_default_credentials.json'

# Azure credentials constants
AZ_DEFAULT_CREDS_FILE = '~/.azure/azure_credentials.json'

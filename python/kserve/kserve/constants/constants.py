# Copyright 2021 The KServe Authors.
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

# KServe K8S constants
KSERVE_GROUP = 'serving.kserve.io'
KSERVE_KIND = 'InferenceService'
KSERVE_PLURAL = 'inferenceservices'
KSERVE_KIND_TRAINEDMODEL = 'TrainedModel'
KSERVE_PLURAL_TRAINEDMODEL = 'trainedmodels'
KSERVE_V1BETA1_VERSION = 'v1beta1'
KSERVE_V1ALPHA1_VERSION = "v1alpha1"

KSERVE_V1BETA1 = KSERVE_GROUP + '/' + KSERVE_V1BETA1_VERSION
KSERVE_V1ALPHA1 = KSERVE_GROUP + '/' + KSERVE_V1ALPHA1_VERSION

KSERVE_LOGLEVEL = os.environ.get('KSERVE_LOGLEVEL', 'INFO').upper()

# INFERENCESERVICE credentials common constants
INFERENCESERVICE_CONFIG_MAP_NAME = 'inferenceservice-config'
INFERENCESERVICE_SYSTEM_NAMESPACE = 'kserve'
DEFAULT_SECRET_NAME = "kserve-secret-"
DEFAULT_SA_NAME = "kserve-service-credentials"

# S3 credentials constants
S3_ACCESS_KEY_ID_DEFAULT_NAME = "AWS_ACCESS_KEY_ID"
S3_SECRET_ACCESS_KEY_DEFAULT_NAME = "AWS_SECRET_ACCESS_KEY"
S3_DEFAULT_CREDS_FILE = '~/.aws/credentials'

# GCS credentials constants
GCS_CREDS_FILE_DEFAULT_NAME = 'gcloud-application-credentials.json'
GCS_DEFAULT_CREDS_FILE = '~/.config/gcloud/application_default_credentials.json'

# Azure credentials constants
AZ_DEFAULT_CREDS_FILE = '~/.azure/azure_credentials.json'

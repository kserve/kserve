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
from enum import Enum

# KServe K8S constants
KSERVE_GROUP = "serving.kserve.io"
KSERVE_KIND_INFERENCESERVICE = "InferenceService"
KSERVE_PLURAL_INFERENCESERVICE = "inferenceservices"
KSERVE_KIND_TRAINEDMODEL = "TrainedModel"
KSERVE_PLURAL_TRAINEDMODEL = "trainedmodels"
KSERVE_KIND_INFERENCEGRAPH = "InferenceGraph"
KSERVE_PLURAL_INFERENCEGRAPH = "inferencegraphs"
KSERVE_KIND_LOCALMODELNODEGROUP = "LocalModelNodeGroup"
KSERVE_PLURAL_LOCALMODELNODEGROUP = "localmodelnodegroups"
KSERVE_KIND_LOCALMODELCACHE = "LocalModelCache"
KSERVE_PLURAL_LOCALMODELCACHE = "localmodelcaches"
KSERVE_KIND_LOCALMODELNODE = "LocalModelNode"
KSERVE_PLURAL_LOCALMODELNODE = "localmodelnodes"
KSERVE_V1BETA1_VERSION = "v1beta1"
KSERVE_V1ALPHA1_VERSION = "v1alpha1"

KSERVE_V1BETA1 = KSERVE_GROUP + "/" + KSERVE_V1BETA1_VERSION
KSERVE_V1ALPHA1 = KSERVE_GROUP + "/" + KSERVE_V1ALPHA1_VERSION

KSERVE_LOGLEVEL = os.environ.get("KSERVE_LOGLEVEL", "INFO").upper()

# INFERENCESERVICE credentials common constants
INFERENCESERVICE_CONFIG_MAP_NAME = "inferenceservice-config"
INFERENCESERVICE_SYSTEM_NAMESPACE = "kserve"
DEFAULT_SECRET_NAME = "kserve-secret-"
DEFAULT_SA_NAME = "kserve-service-credentials"

# S3 credentials constants
S3_ACCESS_KEY_ID_DEFAULT_NAME = "AWS_ACCESS_KEY_ID"
S3_SECRET_ACCESS_KEY_DEFAULT_NAME = "AWS_SECRET_ACCESS_KEY"
S3_DEFAULT_CREDS_FILE = "~/.aws/credentials"

# GCS credentials constants
GCS_CREDS_FILE_DEFAULT_NAME = "gcloud-application-credentials.json"
GCS_DEFAULT_CREDS_FILE = "~/.config/gcloud/application_default_credentials.json"

# Azure credentials constants
AZ_DEFAULT_CREDS_FILE = "~/.azure/azure_credentials.json"

# Model Serve Constants
KSERVE_MODEL_SERVER_NAME = "kserve"

# GRPC content datatype mappings constants
GRPC_CONTENT_DATATYPE_MAPPINGS = {
    "BOOL": "bool_contents",
    "INT8": "int_contents",
    "INT16": "int_contents",
    "INT32": "int_contents",
    "INT64": "int64_contents",
    "UINT8": "uint_contents",
    "UINT16": "uint_contents",
    "UINT32": "uint_contents",
    "UINT64": "uint64_contents",
    "FP32": "fp32_contents",
    "FP64": "fp64_contents",
    "BYTES": "bytes_contents",
}
# K8S status key constants
OBSERVED_GENERATION = "observedGeneration"

# K8S metadata key constants
GENERATION = "generation"

EXPLAINER_BASE_URL_FORMAT = "{0}://{1}"


class PredictorProtocol(Enum):
    REST_V1 = "v1"
    REST_V2 = "v2"
    GRPC_V2 = "grpc-v2"


# LLM stats map key
LLM_STATS_KEY = "llm-stats"

# Default GRPC max message length
MAX_GRPC_MESSAGE_LENGTH = 8388608

V2_ROUTE_PREFIX = "/v2"
V1_ROUTE_PREFIX = "/v1"

DEFAULT_HTTP_PORT = 8080
DEFAULT_GRPC_PORT = 8081

# Header containing the json length in case of REST raw response.
INFERENCE_CONTENT_LENGTH_HEADER = "inference-header-content-length"

FASTAPI_APP_IMPORT_STRING = "kserve.model_server:app"


class ModelType(Enum):
    EXPLAINER = 1
    PREDICTOR = 2

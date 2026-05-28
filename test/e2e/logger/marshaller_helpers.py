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

import uuid
from typing import Optional

from kubernetes import client
from kubernetes.client import V1ResourceRequirements

from kserve import (
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1LoggerSpec,
    V1beta1PredictorSpec,
    V1beta1SKLearnSpec,
    V1beta1StorageSpec,
    constants,
)
from kserve.models.v1beta1_logger_storage_spec import V1beta1LoggerStorageSpec

from ..common.utils import KSERVE_TEST_NAMESPACE

LOGGER_S3_URL = "s3://logger-output/logs"


def create_logger_isvc(
    format: str,
    batch_size: int = 1,
    batch_interval: Optional[str] = None,
    store_path: Optional[str] = None,
) -> tuple[str, V1beta1InferenceService]:
    """Create an ISVC spec configured for marshaller e2e testing.

    Returns (service_name, isvc_object).
    """
    suffix = str(uuid.uuid4())[:6]
    service_name = f"sklearn-log-{format}-{suffix}"

    if store_path is None:
        store_path = f"{format}-{suffix}"

    logger_spec = V1beta1LoggerSpec(
        url=LOGGER_S3_URL,
        mode="all",
        storage=V1beta1LoggerStorageSpec(
            key="loggerS3",
            path=store_path,
            parameters={"format": format},
        ),
        batch_size=batch_size,
    )
    if batch_interval is not None:
        logger_spec.batch_interval = batch_interval

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        logger=logger_spec,
        sklearn=V1beta1SKLearnSpec(
            storage=V1beta1StorageSpec(
                key="localS3",
                path="sklearn",
                parameters={"bucket": "example-models"},
            ),
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    return service_name, isvc

# Copyright 2026 The KServe Authors.
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

from __future__ import annotations

import logging
import os
from contextlib import asynccontextmanager
from typing import AsyncIterator

from kubernetes import client

from kserve import (
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    constants,
)
from ..common.utils import (
    AUTOGLUON_ISVC_WAIT_TIMEOUT,
    KSERVE_TEST_NAMESPACE,
    predict_isvc,
)


def create_autogluon_isvc(
    service_name: str, predictor: V1beta1PredictorSpec
) -> V1beta1InferenceService:
    return V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )


def _kserve_client() -> KServeClient:
    return KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


def _log_predictor_pods(kserve_client: KServeClient, service_name: str) -> None:
    pods = kserve_client.core_api.list_namespaced_pod(
        KSERVE_TEST_NAMESPACE,
        label_selector=f"serving.kserve.io/inferenceservice={service_name}",
    )
    for pod in pods.items:
        logging.info(pod)


@asynccontextmanager
async def autogluon_isvc(
    service_name: str,
    predictor: V1beta1PredictorSpec,
    timeout_seconds: int = AUTOGLUON_ISVC_WAIT_TIMEOUT,
) -> AsyncIterator[KServeClient]:
    """Create an InferenceService, wait until ready, then delete it on exit."""
    kserve_client = _kserve_client()
    kserve_client.create(create_autogluon_isvc(service_name, predictor))
    try:
        try:
            kserve_client.wait_isvc_ready(
                service_name,
                namespace=KSERVE_TEST_NAMESPACE,
                timeout_seconds=timeout_seconds,
            )
        except RuntimeError as e:
            _log_predictor_pods(kserve_client, service_name)
            raise e
        yield kserve_client
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


async def deploy_and_predict(
    service_name: str,
    predictor: V1beta1PredictorSpec,
    rest_client,
    input_path: str,
    timeout_seconds: int = AUTOGLUON_ISVC_WAIT_TIMEOUT,
    network_layer: str = "istio",
):
    async with autogluon_isvc(service_name, predictor, timeout_seconds):
        return await predict_isvc(
            rest_client, service_name, input_path, network_layer=network_layer
        )

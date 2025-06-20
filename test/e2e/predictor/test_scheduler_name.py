# Copyright 2022 The KServe Authors.
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
from timeout_sampler import TimeoutExpiredError, TimeoutSampler
import pytest
import logging
from kubernetes import client

from kubernetes.client import V1ResourceRequirements
from kserve import KServeClient
from kserve import constants
from kserve import V1beta1PredictorSpec
from kserve import V1beta1InferenceServiceSpec
from kserve import V1beta1InferenceService
from kserve.models.v1beta1_sk_learn_spec import V1beta1SKLearnSpec

from ..common.utils import KSERVE_TEST_NAMESPACE

# Set up logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


def get_pods(service_name: str) -> list[client.V1Pod]:
    pods = kserve_client.core_api.list_namespaced_pod(
        KSERVE_TEST_NAMESPACE,
        label_selector=f"serving.kserve.io/inferenceservice={service_name}",
    )
    return pods.items


@pytest.mark.kserve_on_openshift
@pytest.mark.asyncio(scope="session")
async def test_scheduler_name(rest_v1_client):
    scheduler_name = "kserve-scheduler"
    service_name = "isvc-sklearn-scheduler"
    logger.info("Creating InferenceService %s", service_name)

    predictor = V1beta1PredictorSpec(
        scheduler_name=scheduler_name,  # This scheduler doesn't exist, but pods should still be created
        min_replicas=1,
        sklearn=V1beta1SKLearnSpec(
            storage_uri="gs://kfserving-examples/models/sklearn/1.0/model",
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
            annotations={
                "serving.kserve.io/autoscalerClass": "none"  # Adding autoscaler annotation
            },
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client.create(isvc)
    isvc = kserve_client.get(service_name, KSERVE_TEST_NAMESPACE)

    assert (
        isvc["spec"]["predictor"]["schedulerName"] == scheduler_name
    ), f"Expected scheduler name '{scheduler_name}', got {isvc['spec']['predictor'].get('schedulerName')}"

    try:
        for pods in TimeoutSampler(
            wait_timeout=30,
            sleep=2,
            func=lambda: get_pods(service_name),
        ):
            if len(pods) > 0:
                break

        pods = get_pods(service_name)
        for pod in pods:
            assert pod.spec.scheduler_name == scheduler_name, (
                f"Pod {pod.metadata.name} scheduler name {pod.spec.scheduler_name} "
                f"does not match expected value '{scheduler_name}'"
            )
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
    except TimeoutExpiredError as e:
        logger.error("Timeout waiting for pods to be created")
        raise e

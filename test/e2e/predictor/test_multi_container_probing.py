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
import pytest
import logging
from kubernetes import client

from kubernetes.client import (
    V1ResourceRequirements,
    V1Probe,
    V1HTTPGetAction,
    V1ContainerPort,
    V1EnvVar,
    V1Container,
    V1TCPSocketAction,
)
from kserve import KServeClient
from kserve import constants
from kserve import (
    V1beta1PredictorSpec,
    V1beta1SKLearnSpec,
    V1beta1InferenceServiceSpec,
    V1beta1InferenceService,
)
from timeout_sampler import TimeoutExpiredError, TimeoutSampler

from ..common.utils import KSERVE_TEST_NAMESPACE

# Set up logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


def get_deployment(k8s_client: client.AppsV1Api, service_name: str) -> client.V1Deployment:
    """Get the Kubernetes Deployment for RawDeployment mode."""
    return k8s_client.read_namespaced_deployment(
        name=service_name + "-predictor",
        namespace=KSERVE_TEST_NAMESPACE,
    )


@pytest.mark.kserve_on_openshift
@pytest.mark.asyncio(scope="session")
async def test_multi_container_probing(rest_v1_client):
    service_name = "isvc-sklearn-mcp"
    logger.info("Creating InferenceService %s", service_name)

    # Create the main predictor container
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        max_replicas=1,
        sklearn=V1beta1SKLearnSpec(
            storage_uri="gs://kfserving-examples/models/sklearn/1.0/model",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
            liveness_probe=V1Probe(
                http_get=V1HTTPGetAction(
                    path="/v1/models/" + service_name, port=8080, scheme="HTTP"
                ),
                initial_delay_seconds=30,
                period_seconds=10,
            ),
            readiness_probe=V1Probe(
                http_get=V1HTTPGetAction(
                    path="/v1/models/" + service_name, port=8080, scheme="HTTP"
                ),
                initial_delay_seconds=30,
                period_seconds=10,
            ),
        ),
        containers=[
            V1Container(
                name="kserve-agent",
                image="quay.io/opendatahub/kserve-agent:v0.14",
                ports=[V1ContainerPort(container_port=8080, protocol="TCP")],
                env=[
                    V1EnvVar(name="AGENT_TARGET_PORT", value="8080"),
                    V1EnvVar(name="AGENT_TARGET_HOST", value="localhost"),
                    V1EnvVar(
                        name="SERVING_READINESS_PROBE",
                        value='{"tcpSocket":{"port":8080},"initialDelaySeconds":60,"periodSeconds":10}',
                    ),
                ],
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "256Mi"},
                ),
                liveness_probe=V1Probe(
                    tcp_socket=V1TCPSocketAction(
                        port=8080,
                    ),
                    initial_delay_seconds=60,
                    period_seconds=10,
                ),
                readiness_probe=V1Probe(
                    tcp_socket=V1TCPSocketAction(
                        port=8080,
                    ),
                    initial_delay_seconds=60,
                    period_seconds=10,
                ),
            )
        ],
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations={
                "serving.kserve.io/autoscalerClass": "none",
                "serving.kserve.io/DeploymentMode": "RawDeployment",
            },
            labels={
                constants.KSERVE_LABEL_NETWORKING_VISIBILITY: constants.KSERVE_LABEL_NETWORKING_VISIBILITY_EXPOSED,
            },
        ),
        spec=V1beta1InferenceServiceSpec(
            predictor=predictor,
        ),
    )

    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, KSERVE_TEST_NAMESPACE)

    # Get the Kubernetes Deployment for RawDeployment mode
    k8s_client = client.AppsV1Api()
    try:
        for deployment in TimeoutSampler(
            wait_timeout=60,
            sleep=2,
            func=lambda: get_deployment(k8s_client, service_name),
        ):
            # Wait for Deployment to be ready
            if deployment.status.ready_replicas and deployment.status.ready_replicas > 0:
                break

        # Get latest deployment state after ready condition is met
        ready_deployment = get_deployment(k8s_client, service_name)
        containers = ready_deployment.spec.template.spec.containers

        # Find containers by name
        kserve_container = next(
            c for c in containers if c.name == "kserve-container"
        )
        kserve_agent = next(c for c in containers if c.name == "kserve-agent")

        # Verify kserve-container probes
        assert kserve_container.liveness_probe is not None
        assert kserve_container.readiness_probe is not None
        logger.info("kserve-container probes verified successfully")

        # Verify kserve-agent probes
        assert kserve_agent.liveness_probe is not None
        assert kserve_agent.readiness_probe is not None
        logger.info("kserve-agent probes verified successfully")

        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
    except TimeoutExpiredError as e:
        logger.error("Timeout waiting for deployment to be ready")
        raise e

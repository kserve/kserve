# Copyright 2024 The KServe Authors.
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

import asyncio
import json
import os
import time
from concurrent import futures
from typing import Union, List, Dict
from urllib.parse import urlparse

import portforward
import requests
from kubernetes import client as k8s_client
from orjson import orjson

from kserve import KServeClient, InferResponse, InferRequest
from kserve import constants
from kserve.inference_client import InferenceGRPCClient, InferenceRESTClient
from kserve.protocol.grpc import grpc_predict_v2_pb2 as pb
from kserve.logging import trace_logger as logger

KSERVE_NAMESPACE = "kserve"
KSERVE_TEST_NAMESPACE = "kserve-ci-e2e-test"
MODEL_CLASS_NAME = "modelClass"
INFERENCESERVICE_CONTAINER = "kserve-container"
TRANSFORMER_CONTAINER = "transformer-container"
STORAGE_URI_ENV = "STORAGE_URI"


def grpc_client(host):
    cluster_ip = get_cluster_ip()
    if ":" not in cluster_ip:
        cluster_ip = cluster_ip + ":80"
    logger.info("Cluster IP: %s", cluster_ip)
    logger.info("gRPC target host: %s", host)
    return InferenceGRPCClient(
        cluster_ip,
        verbose=True,
        channel_args=[
            ("grpc.ssl_target_name_override", host),
        ],
        timeout=120,
    )


async def predict_isvc(
    client: InferenceRESTClient,
    service_name,
    input: Union[str, InferRequest],
    version=constants.KSERVE_V1BETA1_VERSION,
    model_name=None,
    is_batch=False,
    is_raw: bool = False,
) -> Union[InferResponse, Dict, List[Union[Dict, InferResponse]]]:
    kfs_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    isvc = kfs_client.get(
        service_name,
        namespace=KSERVE_TEST_NAMESPACE,
        version=version,
    )
    scheme, cluster_ip, host, path = get_isvc_endpoint(isvc, is_raw)
    if model_name is None:
        model_name = service_name
    base_url = f"{scheme}://{cluster_ip}{path}"
    return await predict(
        client,
        base_url,
        host,
        input,
        model_name=model_name,
        is_batch=is_batch,
        is_graph=False,
    )


async def predict(
    client: InferenceRESTClient,
    url,
    host,
    input: Union[str, InferRequest],
    model_name=None,
    is_batch=False,
    is_graph=False,
) -> Union[InferResponse, Dict, List[Union[Dict, InferResponse]]]:
    if isinstance(input, str):
        with open(input) as json_file:
            data = json.load(json_file)
    else:
        data = input
    headers = {"Host": host, "Content-Type": "application/json"}
    if is_batch:
        with futures.ThreadPoolExecutor(max_workers=4) as executor:
            loop = asyncio.get_event_loop()
            future_list = [
                await loop.run_in_executor(
                    executor,
                    _predict,
                    client,
                    url,
                    input_data,
                    model_name,
                    headers,
                    is_graph,
                )
                for input_data in data
            ]
            result = await asyncio.gather(*future_list)
    else:
        result = await _predict(
            client,
            url,
            data,
            model_name=model_name,
            headers=headers,
            is_graph=is_graph,
        )
    logger.info("Got response %s", result)
    return result


async def _predict(
    client: InferenceRESTClient,
    url,
    input_data,
    model_name,
    headers=None,
    is_graph=False,
) -> Union[InferResponse, Dict]:
    logger.info("Sending Header = %s", headers)
    logger.info("base url = %s", url)
    # temporary sleep until this is fixed https://github.com/kserve/kserve/issues/604
    await asyncio.sleep(5)
    response = await client.infer(
        url,
        input_data,
        model_name=model_name,
        headers=headers,
        is_graph_endpoint=is_graph,
    )
    return response


async def predict_ig(
    client: InferenceRESTClient,
    ig_name,
    input_path,
    version=constants.KSERVE_V1ALPHA1_VERSION,
    is_raw: bool = False,
) -> Union[InferResponse, Dict]:
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    ig = kserve_client.get_inference_graph(
        ig_name,
        namespace=KSERVE_TEST_NAMESPACE,
        version=version,
    )
    scheme, cluster_ip, host, _ = get_isvc_endpoint(ig, is_raw)
    url = f"{scheme}://{cluster_ip}"
    return await predict(client, url, host, input_path, is_graph=True)


async def explain_art(client, service_name, input_path, is_raw: bool = False) -> Dict:
    res = await explain_response(client, service_name, input_path, is_raw)
    return res["explanations"]["adversarial_prediction"]


async def explain_response(client, service_name, input_path, is_raw: bool) -> Dict:
    kfs_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    isvc = kfs_client.get(
        service_name,
        namespace=KSERVE_TEST_NAMESPACE,
        version=constants.KSERVE_V1BETA1_VERSION,
    )
    scheme, cluster_ip, host, path = get_isvc_endpoint(isvc, is_raw)
    url = f"{scheme}://{cluster_ip}{path}"
    headers = {"Host": host}
    with open(input_path) as json_file:
        data = json.load(json_file)
        logger.info("Sending request data: %s", data)
        try:
            # temporary sleep until this is fixed https://github.com/kserve/kserve/issues/604
            await asyncio.sleep(5)
            response = await client.explain(
                url, model_name=service_name, data=data, headers=headers
            )
        except (RuntimeError, orjson.JSONDecodeError) as e:
            logger.info("Explain error -------")
            pods = kfs_client.core_api.list_namespaced_pod(
                KSERVE_TEST_NAMESPACE,
                label_selector="serving.kserve.io/inferenceservice={}".format(
                    service_name
                ),
            )
            for pod in pods.items:
                logger.info(pod)
                logger.info(
                    "%s\t%s\t%s"
                    % (pod.metadata.name, pod.status.phase, pod.status.pod_ip)
                )
                api_response = kfs_client.core_api.read_namespaced_pod_log(
                    pod.metadata.name,
                    KSERVE_TEST_NAMESPACE,
                    container="kserve-container",
                )
                logger.info(api_response)
            raise e
        return response


def get_cluster_ip(
    name="istio-ingressgateway", namespace="istio-system", labels: dict = None
):
    cluster_ip = os.environ.get("KSERVE_INGRESS_HOST_PORT")
    if cluster_ip is None:
        api_instance = k8s_client.CoreV1Api(k8s_client.ApiClient())
        if labels:
            label_selector = ",".join(
                [f"{key}={value}" for key, value in labels.items()]
            )
            services = api_instance.list_namespaced_service(
                namespace, label_selector=label_selector
            )
            if services.items:
                service = services.items[0]
            else:
                raise RuntimeError(f"No service found with labels: {labels}")
        else:
            service = api_instance.read_namespaced_service(name, namespace)

        if service.status.load_balancer.ingress is None:
            cluster_ip = service.spec.cluster_ip
        else:
            if service.status.load_balancer.ingress[0].hostname:
                cluster_ip = service.status.load_balancer.ingress[0].hostname
            else:
                cluster_ip = service.status.load_balancer.ingress[0].ip
    return cluster_ip


async def predict_grpc(
    service_name,
    payload,
    parameters=None,
    version=constants.KSERVE_V1BETA1_VERSION,
    model_name=None,
    is_raw: bool = False,
) -> InferResponse:
    kfs_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    isvc = kfs_client.get(
        service_name,
        namespace=KSERVE_TEST_NAMESPACE,
        version=version,
    )
    _, _, host, _ = get_isvc_endpoint(isvc, is_raw)

    if model_name is None:
        model_name = service_name
    client = grpc_client(host)

    response = await client.infer(
        InferRequest.from_grpc(
            pb.ModelInferRequest(
                model_name=model_name, inputs=payload, parameters=parameters
            )
        )
    )
    return response


async def predict_modelmesh(
    client, service_name, input_json, pod_name, model_name=None
) -> InferResponse:
    with open(input_json) as json_file:
        data = json.load(json_file)

    if model_name is None:
        model_name = service_name
    with portforward.forward("default", pod_name, 8008, 8008, waiting=5):
        headers = {"Content-Type": "application/json"}
        url = "http://localhost:8008"
        logger.info("Sending Header = %s", headers)
        logger.info("base url = %s", url)

        response = await client.infer(url, data, model_name=model_name, headers=headers)
        logger.info("Got response content %s", response)
        return response


def get_isvc_endpoint(isvc, is_raw: bool = False):
    scheme = urlparse(isvc["status"]["url"]).scheme
    host = urlparse(isvc["status"]["url"]).netloc
    path = urlparse(isvc["status"]["url"]).path
    if os.environ.get("CI_USE_ISVC_HOST") == "1":
        cluster_ip = host
    else:
        if is_raw:
            cluster_ip = get_cluster_ip(
                namespace="envoy-gateway-system",
                labels={
                    "gateway.envoyproxy.io/owning-gateway-name": "kserve-ingress-gateway"
                },
            )
        else:
            cluster_ip = get_cluster_ip()
    return scheme, cluster_ip, host, path


def generate(
    service_name,
    input_json,
    version=constants.KSERVE_V1BETA1_VERSION,
    chat_completions=True,
):
    with open(input_json) as json_file:
        data = json.load(json_file)

        kfs_client = KServeClient(
            config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
        )
        isvc = kfs_client.get(
            service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            version=version,
        )
        scheme, cluster_ip, host, path = get_isvc_endpoint(isvc)
        headers = {"Host": host, "Content-Type": "application/json"}

        if chat_completions:
            url = f"{scheme}://{cluster_ip}{path}/openai/v1/chat/completions"
        else:
            url = f"{scheme}://{cluster_ip}{path}/openai/v1/completions"
        logger.info("Sending Header = %s", headers)
        logger.info("Sending url = %s", url)
        logger.info("Sending request data: %s", data)
        # temporary sleep until this is fixed https://github.com/kserve/kserve/issues/604
        time.sleep(10)
        response = requests.post(url, json.dumps(data), headers=headers)
        logger.info(
            "Got response code %s, content %s", response.status_code, response.content
        )
        if response.status_code == 200:
            preds = json.loads(response.content.decode("utf-8"))
            return preds
        else:
            response.raise_for_status()


def is_model_ready(
    rest_client, service_name, model_name, version=constants.KSERVE_V1BETA1_VERSION
):
    kfs_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    isvc = kfs_client.get(
        service_name,
        namespace=KSERVE_TEST_NAMESPACE,
        version=version,
    )
    scheme, cluster_ip, host, path = get_isvc_endpoint(isvc)
    if model_name is None:
        model_name = service_name
    base_url = f"{scheme}://{cluster_ip}{path}"
    headers = {"Host": host}
    return rest_client.is_model_ready(base_url, model_name, headers=headers)
